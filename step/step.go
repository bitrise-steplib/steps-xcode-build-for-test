package step

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bitrise-io/go-steputils/output"
	"github.com/bitrise-io/go-steputils/tools"
	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-io/go-utils/sliceutil"
	v2command "github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/fileutil"
	v2log "github.com/bitrise-io/go-utils/v2/log"
	v2pathutil "github.com/bitrise-io/go-utils/v2/pathutil"
	"github.com/bitrise-io/go-xcode/utility"
	"github.com/bitrise-io/go-xcode/v2/codesign"
	"github.com/bitrise-io/go-xcode/v2/xcconfig"
	"github.com/bitrise-io/go-xcode/v2/xcpretty"
	"github.com/bitrise-io/go-xcode/xcodebuild"
	cache "github.com/bitrise-io/go-xcode/xcodecache"
	"github.com/bitrise-steplib/steps-xcode-build-for-test/xcodeproject"
	"github.com/kballard/go-shellquote"
)

const (
	testBundlePathEnvKey    = "BITRISE_TEST_BUNDLE_PATH"
	testBundleZipPathEnvKey = "BITRISE_TEST_BUNDLE_ZIP_PATH"
	xctestrunPathEnvKey     = "BITRISE_XCTESTRUN_FILE_PATH"
	xcodebuildLogPathEnvKey = "BITRISE_XCODE_RAW_RESULT_TEXT_PATH"
	xcodebuildLogBaseName   = "raw-xcodebuild-output.log"
)

const xctestrunExt = ".xctestrun"

const (
	codeSignSourceOff     = "off"
	codeSignSourceAPIKey  = "api-key"
	codeSignSourceAppleID = "apple-id"
)

type Input struct {
	ProjectPath   string `env:"project_path,required"`
	Scheme        string `env:"scheme,required"`
	Configuration string `env:"configuration"`
	Destination   string `env:"destination,required"`
	TestPlan      string `env:"test_plan"`
	// xcodebuild configuration
	XCConfigContent   string `env:"xcconfig_content"`
	XcodebuildOptions string `env:"xcodebuild_options"`
	// xcodebuild log formatting
	LogFormatter string `env:"log_formatter,opt[xcpretty,xcodebuild]"`
	// Automatic code signing
	CodeSigningAuthSource     string          `env:"automatic_code_signing,opt[off,api-key,apple-id]"`
	RegisterTestDevices       bool            `env:"register_test_devices,opt[yes,no]"`
	MinDaysProfileValid       int             `env:"min_profile_validity,required"`
	TeamID                    string          `env:"apple_team_id"`
	CertificateURLList        string          `env:"certificate_url_list"`
	CertificatePassphraseList stepconf.Secret `env:"passphrase_list"`
	KeychainPath              string          `env:"keychain_path"`
	KeychainPassword          stepconf.Secret `env:"keychain_password"`
	BuildURL                  string          `env:"BITRISE_BUILD_URL"`
	BuildAPIToken             stepconf.Secret `env:"BITRISE_BUILD_API_TOKEN"`
	// Step output configuration
	OutputDir string `env:"output_dir,required"`
	// Caching
	CacheLevel string `env:"cache_level,opt[none,swift_packages]"`
	// Debugging
	VerboseLog bool `env:"verbose_log,opt[yes,no]"`
}

type Config struct {
	ProjectPath            string
	Scheme                 string
	Configuration          string
	Destination            string
	TestPlan               string
	XCConfig               string
	XcodebuildOptions      []string
	XCPretty               bool
	CodesignManager        *codesign.Manager
	OutputDir              string
	XcodebuildMajorVersion int
	CacheLevel             string
	SwiftPackagesPath      string
}

type XcodebuildBuilder struct {
	logger       v2log.Logger
	xcodeproject xcodeproject.XcodeProject
	pathChecker  v2pathutil.PathChecker
	pathProvider v2pathutil.PathProvider
	fileManager  FileManager
}

func NewXcodebuildBuilder(logger v2log.Logger, xcodeproject xcodeproject.XcodeProject, pathChecker v2pathutil.PathChecker, pathProvider v2pathutil.PathProvider, fileManager FileManager) XcodebuildBuilder {
	return XcodebuildBuilder{
		logger:       logger,
		xcodeproject: xcodeproject,
		pathChecker:  pathChecker,
		pathProvider: pathProvider,
		fileManager:  fileManager,
	}
}

func (b XcodebuildBuilder) ProcessConfig() (Config, error) {
	var input Input
	parser := stepconf.NewInputParser(env.NewRepository())
	if err := parser.Parse(&input); err != nil {
		return Config{}, err
	}

	b.logger.EnableDebugLog(input.VerboseLog)
	log.SetEnableDebugLog(input.VerboseLog)

	stepconf.Print(input)
	b.logger.Println()

	absProjectPath, err := filepath.Abs(input.ProjectPath)
	if err != nil {
		return Config{}, fmt.Errorf("failed to expand project path (%s): %w", input.ProjectPath, err)
	}

	absOutputDir, err := pathutil.AbsPath(input.OutputDir)
	if err != nil {
		return Config{}, fmt.Errorf("failed to expand output dir (%s): %w", input.OutputDir, err)
	}

	if exist, err := pathutil.IsPathExists(absOutputDir); err != nil {
		return Config{}, fmt.Errorf("failed to check if output dir exist: %w", err)
	} else if !exist {
		if err := os.MkdirAll(absOutputDir, 0777); err != nil {
			return Config{}, fmt.Errorf("failed to create output dir (%s): %w", absOutputDir, err)
		}
	}

	xcodebuildVersion, err := utility.GetXcodeVersion()
	if err != nil {
		return Config{}, fmt.Errorf("failed to get xcode version: %w", err)
	}
	b.logger.Infof("Xcode version: %s (%s)", xcodebuildVersion.Version, xcodebuildVersion.BuildVersion)

	var swiftPackagesPath string
	if xcodebuildVersion.MajorVersion >= 11 {
		var err error
		if swiftPackagesPath, err = cache.SwiftPackagesPath(absProjectPath); err != nil {
			return Config{}, fmt.Errorf("failed to get swift packages path: %w", err)
		}
	}

	if strings.TrimSpace(input.XCConfigContent) == "" {
		input.XCConfigContent = ""
	}

	var customOptions []string
	if input.XcodebuildOptions != "" {
		customOptions, err = shellquote.Split(input.XcodebuildOptions)
		if err != nil {
			return Config{}, fmt.Errorf("provided additional options (%s) are not valid CLI arguments: %w", input.XcodebuildOptions, err)
		}

		if sliceutil.IsStringInSlice("-xcconfig", customOptions) && input.XCConfigContent != "" {
			return Config{}, fmt.Errorf("`-xcconfig` option found in 'Additional options for the xcodebuild command' input, please clear 'Build settings (xcconfig)' input as only one can be set")
		}
	}

	var codesignManager *codesign.Manager
	if input.CodeSigningAuthSource != codeSignSourceOff {
		factory := v2command.NewFactory(env.NewRepository())

		codesignMgr, err := createCodesignManager(CodesignManagerOpts{
			ProjectPath:               absProjectPath,
			Scheme:                    input.Scheme,
			Configuration:             input.Configuration,
			CodeSigningAuthSource:     input.CodeSigningAuthSource,
			RegisterTestDevices:       input.RegisterTestDevices,
			MinDaysProfileValid:       input.MinDaysProfileValid,
			TeamID:                    input.TeamID,
			CertificateURLList:        input.CertificateURLList,
			CertificatePassphraseList: input.CertificatePassphraseList,
			KeychainPath:              input.KeychainPath,
			KeychainPassword:          input.KeychainPassword,
			BuildURL:                  input.BuildURL,
			BuildAPIToken:             input.BuildAPIToken,
			VerboseLog:                input.VerboseLog,
		}, xcodebuildVersion.MajorVersion, b.logger, factory)
		if err != nil {
			return Config{}, err
		}
		codesignManager = &codesignMgr
	}

	return Config{
		ProjectPath:            absProjectPath,
		Scheme:                 input.Scheme,
		Configuration:          input.Configuration,
		Destination:            input.Destination,
		TestPlan:               input.TestPlan,
		XCConfig:               input.XCConfigContent,
		XcodebuildOptions:      customOptions,
		XCPretty:               input.LogFormatter == "xcpretty",
		CodesignManager:        codesignManager,
		OutputDir:              absOutputDir,
		XcodebuildMajorVersion: int(xcodebuildVersion.MajorVersion),
		CacheLevel:             input.CacheLevel,
		SwiftPackagesPath:      swiftPackagesPath,
	}, nil
}

func (b XcodebuildBuilder) InstallDependencies(useXCPretty bool) error {
	if !useXCPretty {
		return nil
	}

	b.logger.Println()
	b.logger.Infof("Checking if output tool (xcpretty) is installed")
	formatter := xcpretty.NewXcpretty(b.logger)

	installed, err := formatter.IsInstalled()
	if err != nil {
		return err
	}

	if !installed {
		b.logger.Warnf("xcpretty is not installed")
		b.logger.Println()
		b.logger.Printf("Installing xcpretty")

		cmdModelSlice, err := formatter.Install()
		if err != nil {
			return fmt.Errorf("failed to create xcpretty install commands: %w", err)
		}

		for _, cmd := range cmdModelSlice {
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to run xcpretty install command (%s): %w", cmd.PrintableCommandArgs(), err)
			}
		}
	}

	xcprettyVersion, err := formatter.Version()
	if err != nil {
		return fmt.Errorf("failed to get xcpretty version: %w", err)
	}

	b.logger.Printf("- xcpretty version: %s", xcprettyVersion.String())
	return nil
}

type RunOut struct {
	XcodebuildLog       string
	XctestrunPths       []string
	DefaultXctestrunPth string
	SYMRoot             string
}

func (b XcodebuildBuilder) Run(cfg Config) (RunOut, error) {
	// Automatic code signing
	authOptions, err := b.automaticCodeSigning(cfg.CodesignManager)
	if err != nil {
		return RunOut{}, err
	}
	defer func() {
		if authOptions != nil && authOptions.KeyPath != "" {
			if err := os.Remove(authOptions.KeyPath); err != nil {
				b.logger.Warnf("failed to remove private key file: %s", err)
			}
		}
	}()

	// Build for testing
	b.logger.Println()
	b.logger.Infof("Running xcodebuild")

	xcodeBuildCmd := xcodebuild.NewCommandBuilder(cfg.ProjectPath, "build-for-testing")
	xcodeBuildCmd.SetScheme(cfg.Scheme)
	xcodeBuildCmd.SetConfiguration(cfg.Configuration)
	xcodeBuildCmd.SetDestination(cfg.Destination)
	xcodeBuildCmd.SetTestPlan(cfg.TestPlan)

	options := cfg.XcodebuildOptions
	symRoot, err := b.pathProvider.CreateTempDir("test_bundle")
	if err != nil {
		return RunOut{}, err
	}
	// TODO: make sure we set SYMROOT only once
	options = append(options, fmt.Sprintf("SYMROOT=%s", symRoot))
	xcodeBuildCmd.SetCustomOptions(options)

	if cfg.XCConfig != "" {
		xcconfigWriter := xcconfig.NewWriter(v2pathutil.NewPathProvider(), fileutil.NewFileManager(), v2pathutil.NewPathChecker(), v2pathutil.NewPathModifier())
		xcconfigPath, err := xcconfigWriter.Write(cfg.XCConfig)
		if err != nil {
			return RunOut{}, err
		}
		xcodeBuildCmd.SetXCConfigPath(xcconfigPath)
	}

	if authOptions != nil {
		xcodeBuildCmd.SetAuthentication(*authOptions)
	}

	result := RunOut{}
	rawXcodebuildOut, err := runCommandWithRetry(xcodeBuildCmd, cfg.XCPretty, cfg.SwiftPackagesPath)
	// TODO: if output_tool == xcodebuild, the build log is printed to stdout + last couple of lines printed again
	if err != nil || !cfg.XCPretty {
		printLastLinesOfXcodebuildTestLog(rawXcodebuildOut, err == nil, b.logger)
	}

	result.XcodebuildLog = rawXcodebuildOut

	if err != nil {
		return result, err
	}

	// Cache swift packages
	if cfg.CacheLevel == "swift_packages" {
		if err := cache.CollectSwiftPackages(cfg.ProjectPath); err != nil {
			b.logger.Warnf("Failed to mark swift packages for caching: %s", err)
		}
	}

	// Find outputs
	b.logger.Println()
	b.logger.Infof("Searching for outputs")
	testBundle, err := b.findTestBundle(findTestBundleOpts{
		SYMRoot:     symRoot,
		ProjectPath: cfg.ProjectPath,
		Scheme:      cfg.Scheme,
	})
	if err != nil {
		return result, err
	}

	result.XctestrunPths = testBundle.XctestrunPths
	result.DefaultXctestrunPth = testBundle.DefaultXctestrunPth
	result.SYMRoot = testBundle.SYMRoot

	return result, nil
}

type ExportOpts struct {
	RunOut
	OutputDir string
}

func (b XcodebuildBuilder) ExportOutputs(opts ExportOpts) error {
	b.logger.Println()
	b.logger.Infof("Export outputs")

	if opts.XcodebuildLog != "" {
		if err := b.exportXcodebuildLog(opts.OutputDir, opts.XcodebuildLog); err != nil {
			b.logger.Warnf("%s", err)
		}
	}

	if len(opts.XctestrunPths) == 0 {
		return nil
	}

	if err := b.exportTestBundle(opts.OutputDir, opts.SYMRoot, opts.XctestrunPths, opts.DefaultXctestrunPth); err != nil {
		b.logger.Warnf("%s", err)
	}

	return nil
}

func (b XcodebuildBuilder) automaticCodeSigning(codesignManager *codesign.Manager) (*xcodebuild.AuthenticationParams, error) {
	b.logger.Println()

	if codesignManager == nil {
		b.logger.Infof("Automatic code signing is disabled, skipped downloading code sign assets")
		return nil, nil
	}

	b.logger.Infof("Preparing code signing assets (certificates, profiles)")

	xcodebuildAuthParams, err := codesignManager.PrepareCodesigning()
	if err != nil {
		return nil, fmt.Errorf("failed to prepare code signing assets: %w", err)
	}

	if xcodebuildAuthParams != nil {
		privateKey, err := xcodebuildAuthParams.WritePrivateKeyToFile()
		if err != nil {
			return nil, err
		}

		return &xcodebuild.AuthenticationParams{
			KeyID:     xcodebuildAuthParams.KeyID,
			IsssuerID: xcodebuildAuthParams.IssuerID,
			KeyPath:   privateKey,
		}, nil
	}

	return nil, nil
}

type findTestBundleOpts struct {
	SYMRoot     string
	ProjectPath string
	Scheme      string
}

type testBundle struct {
	XctestrunPths       []string
	DefaultXctestrunPth string
	SYMRoot             string
}

// findTestBundle searches for the built target, associated tests and xctestrun file(s) in the build root (SYMROOT).
// Example file structure of the build root:
// ├── BullsEye_EventuallyFailingInMemoryTests_iphonesimulator15.5-arm64.xctestrun
// ├── BullsEye_EventuallyFailingTests_iphonesimulator15.5-arm64.xctestrun
// ├── BullsEye_EventuallySucceedingTests_iphonesimulator15.5-arm64.xctestrun
// ├── BullsEye_FailingTests_iphonesimulator15.5-arm64.xctestrun
// ├── BullsEye_FullTests_iphonesimulator15.5-arm64.xctestrun
// ├── BullsEye_ParallelUITests_iphonesimulator15.5-arm64.xctestrun
// ├── BullsEye_UITests_iphonesimulator15.5-arm64.xctestrun
// ├── BullsEye_UnitTests_iphonesimulator15.5-arm64.xctestrun
// └── Debug-iphonesimulator
//     ├── BullsEye.app
//     ├── BullsEye.swiftmodule
//     ├── BullsEyeFailingTests.swiftmodule
//     ├── BullsEyeSlowTests.swiftmodule
//     ├── BullsEyeTests.swiftmodule
//     ├── BullsEyeUITests-Runner.app
//     └── BullsEyeUITests.swiftmodule
func (b XcodebuildBuilder) findTestBundle(opts findTestBundleOpts) (testBundle, error) {
	b.logger.Printf("SYMROOT: %s", opts.SYMRoot)

	entries, err := b.fileManager.ReadDir(opts.SYMRoot)
	if err != nil {
		return testBundle{}, fmt.Errorf("failed to list SYMROOT entries: %w", err)
	}

	var xctestrunPths []string
	for _, entry := range entries {
		ext := filepath.Ext(entry.Name())
		if ext == xctestrunExt {
			absXctestrunPth := filepath.Join(opts.SYMRoot, entry.Name())
			if err := b.fixTestRoot(absXctestrunPth); err != nil {
				return testBundle{}, fmt.Errorf("failed to apply TESTROOT fix on %s: %s", absXctestrunPth, err)
			}
			xctestrunPths = append(xctestrunPths, absXctestrunPth)
		}
	}

	if len(xctestrunPths) == 0 {
		return testBundle{}, fmt.Errorf("no xctestrun file generated during the build")
	}

	b.logger.Donef("xctestrun file(s) generated during the build:\n- %s", strings.Join(xctestrunPths, "\n- "))

	// find default xctestrun file
	var defaultXctestrunPth string
	if len(xctestrunPths) > 1 {
		scheme, err := b.xcodeproject.Scheme(opts.ProjectPath, opts.Scheme)
		if err != nil {
			return testBundle{}, err
		}

		var defaultTestPlanName string
		if testPlan := scheme.DefaultTestPlan(); testPlan != nil {
			defaultTestPlanName = testPlan.Name()
		}

		if defaultTestPlanName != "" {
			for _, xctestrunPth := range xctestrunPths {
				// xctestrun file name layout with Test Plans: <scheme>_<test_plan>_<destination>.xctestrun
				if strings.Contains(filepath.Base(xctestrunPth), fmt.Sprintf("_%s_", defaultTestPlanName)) {
					b.logger.Donef("default xctestrun based on %s scheme's default test plan (%s): %s", scheme.Name, defaultTestPlanName, xctestrunPth)
					defaultXctestrunPth = xctestrunPth
				}
			}
		}
	}
	if defaultXctestrunPth == "" {
		defaultXctestrunPth = xctestrunPths[0]
	}

	return testBundle{
		XctestrunPths:       xctestrunPths,
		DefaultXctestrunPth: defaultXctestrunPth,
		SYMRoot:             opts.SYMRoot,
	}, nil
}

// fixTestRoot replaces "/private__TESTROOT__" with "__TESTROOT__" to achieve and xctestrun file,
// that works well with Firebase TestLab.
//
// The "/private" suffix gets added to DependentProductPaths, TestHostPath and UITargetAppPath within the xctestrun file,
// when setting a custom SYMROOT build setting on the `xcodebuild build-for-testing` command.
//
// Setting custom SYMROOT introduced to ease finding the Step generated outputs:
// - xctestrun file(s)
// - built targets and tests dir (for example: Debug-iphonesimulator)
//
// If we find an issue with this workaround, probably we need to get back to the old solution:
// - do not modify the output dir on the xcodebuild command -> output will be placed into the DerivedData dir
// - find all the existing xctestrun files in Xcode's DerivedData dir
// - filter them for the build's timeframe (so that only the current step generated outputs are considered)
// - find the built targets and tests dir based on the configuration and destination inputs
func (b XcodebuildBuilder) fixTestRoot(xctestrunPth string) error {
	c, err := b.fileManager.ReadFile(xctestrunPth)
	if err != nil {
		return err
	}

	newC := bytes.Replace(c, []byte("/private__TESTROOT__"), []byte("__TESTROOT__"), -1)

	return b.fileManager.WriteFile(xctestrunPth, newC, 0666)
}

func (b XcodebuildBuilder) exportXcodebuildLog(outputDir, xcodebuildLog string) error {
	xcodebuildLogPath := filepath.Join(outputDir, xcodebuildLogBaseName)
	if err := output.ExportOutputFileContent(xcodebuildLog, xcodebuildLogPath, xcodebuildLogPathEnvKey); err != nil {
		return fmt.Errorf("failed to export %s, error: %w", xcodebuildLogPathEnvKey, err)
	}
	b.logger.Donef("The xcodebuild command log file path is available in %s env: %s", xcodebuildLogPathEnvKey, xcodebuildLogPath)
	return nil
}

func (b XcodebuildBuilder) exportTestBundle(outputDir, symroot string, xctestrunPths []string, defaultXctestrunPth string) error {
	// BITRISE_TEST_BUNDLE_PATH
	if err := tools.ExportEnvironmentWithEnvman(testBundlePathEnvKey, symroot); err != nil {
		return err
	}
	b.logger.Donef("The test bundle directory is available in %s env: %s", testBundlePathEnvKey, symroot)

	// BITRISE_TEST_BUNDLE_ZIP_PATH
	testBundleZipPth := filepath.Join(outputDir, "testbundle.zip")

	entries, err := b.fileManager.ReadDir(symroot)
	if err != nil {
		return fmt.Errorf("failed to list SYMROOT entries: %w", err)
	}

	var builtTestsDir string
	for _, entry := range entries {
		if entry.IsDir() {
			if builtTestsDir != "" {
				return fmt.Errorf("multiple built test dir found in build output dir")
			}
			builtTestsDir = entry.Name()
		}
	}

	args := []string{"-r", testBundleZipPth}
	args = append(args, builtTestsDir)
	for _, xctestrunPth := range xctestrunPths {
		args = append(args, filepath.Base(xctestrunPth))
	}

	factory := v2command.NewFactory(env.NewRepository())
	zipCmd := factory.Create("zip", args, &v2command.Opts{
		Dir: symroot,
	})
	if out, err := zipCmd.RunAndReturnTrimmedCombinedOutput(); err != nil {
		var exerr *exec.ExitError
		if errors.As(err, &exerr) {
			return fmt.Errorf("%s failed: %s", zipCmd.PrintableCommandArgs(), out)
		}
		return fmt.Errorf("%s failed: %w", zipCmd.PrintableCommandArgs(), err)
	}
	if err := output.ExportOutputFile(testBundleZipPth, testBundleZipPth, testBundleZipPathEnvKey); err != nil {
		return err
	}
	b.logger.Donef("The zipped test bundle is available in %s env: %s", testBundleZipPathEnvKey, testBundleZipPth)

	// BITRISE_XCTESTRUN_FILE_PATH
	if len(xctestrunPths) > 1 {
		b.logger.Warnf("Multiple xctestrun files generated, exporting %s under BITRISE_XCTESTRUN_FILE_PATH", defaultXctestrunPth)
	}
	if err := output.ExportOutputFile(defaultXctestrunPth, defaultXctestrunPth, xctestrunPathEnvKey); err != nil {
		return err
	}
	b.logger.Donef("The built xctestrun file is available in %s env: %s", xctestrunPathEnvKey, defaultXctestrunPth)

	return nil
}
