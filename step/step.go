package step

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitrise-io/go-steputils/output"
	"github.com/bitrise-io/go-steputils/tools"
	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/bitrise-io/go-utils/command"
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
	"github.com/bitrise-io/go-xcode/xcodeproject/xcworkspace"
	xcodebuild2 "github.com/bitrise-steplib/steps-xcode-build-for-test/xcodebuild"
	"github.com/kballard/go-shellquote"
)

const (
	testBundlePathEnvKey         = "BITRISE_TEST_BUNDLE_PATH"
	testBundleZipPathEnvKey      = "BITRISE_TEST_BUNDLE_ZIP_PATH"
	builtTargetBinariesDirEnvKey = "BITRISE_TEST_DIR_PATH"
	xctestrunPathEnvKey          = "BITRISE_XCTESTRUN_FILE_PATH"
	xcodebuildLogPathEnvKey      = "BITRISE_XCODE_RAW_RESULT_TEXT_PATH"
	xcodebuildLogBaseName        = "raw-xcodebuild-output.log"
)

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
	logger         v2log.Logger
	xcodebuild     xcodebuild2.Xcodebuild
	modtimeChecker ModtimeChecker
	pathChecker    v2pathutil.PathChecker
	filepathGlober FilepathGlober
}

func NewXcodebuildBuilder(logger v2log.Logger, xcodebuild xcodebuild2.Xcodebuild, modtimeChecker ModtimeChecker, pathChecker v2pathutil.PathChecker, filepathGlober FilepathGlober) XcodebuildBuilder {
	return XcodebuildBuilder{
		logger:         logger,
		xcodebuild:     xcodebuild,
		modtimeChecker: modtimeChecker,
		pathChecker:    pathChecker,
		filepathGlober: filepathGlober,
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

	factory := v2command.NewFactory(env.NewRepository())
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
	XcodebuildLog string
	BuiltTestDir  string
	XctestrunPths []string
	SYMRoot       string
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

	xcodeBuildCmd := xcodebuild.NewCommandBuilder(cfg.ProjectPath, xcworkspace.IsWorkspace(cfg.ProjectPath), "")
	xcodeBuildCmd.SetScheme(cfg.Scheme)
	xcodeBuildCmd.SetConfiguration(cfg.Configuration)
	xcodeBuildCmd.SetCustomBuildAction("build-for-testing")
	xcodeBuildCmd.SetDestination(cfg.Destination)

	var options []string
	// TODO: add setter method for specifying Test Plan on xcodeBuildCmd
	if cfg.TestPlan != "" {
		options = append(options, "-testPlan", cfg.TestPlan)
	}
	if len(cfg.XcodebuildOptions) > 0 {
		options = append(options, cfg.XcodebuildOptions...)
	}

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
	rawXcodebuildOut, buildInterval, err := runCommandWithRetry(xcodeBuildCmd, cfg.XCPretty, cfg.SwiftPackagesPath)
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
		ProjectPath:       cfg.ProjectPath,
		Scheme:            cfg.Scheme,
		Configuration:     cfg.Configuration,
		XcodebuildOptions: cfg.XcodebuildOptions,
		BuildInterval:     buildInterval,
	})
	if err != nil {
		return result, err
	}

	result.BuiltTestDir = testBundle.BuiltTestDir
	result.XctestrunPths = testBundle.XctestrunPths
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
			log.Warnf("%s", err)
		}
	}

	if opts.BuiltTestDir == "" {
		return nil
	}

	if err := b.exportTestBundle(opts.OutputDir, opts.BuiltTestDir, opts.XctestrunPths); err != nil {
		log.Warnf("%s", err)
	}

	return nil
}

func (b XcodebuildBuilder) exportXcodebuildLog(outputDir, xcodebuildLog string) error {
	xcodebuildLogPath := filepath.Join(outputDir, xcodebuildLogBaseName)
	if err := output.ExportOutputFileContent(xcodebuildLog, xcodebuildLogPath, xcodebuildLogPathEnvKey); err != nil {
		return fmt.Errorf("failed to export %s, error: %s", xcodebuildLogPathEnvKey, err)
	}
	b.logger.Donef("The xcodebuild command log file path is available in %s env: %s", xcodebuildLogPathEnvKey, xcodebuildLogPath)
	return nil
}

func (b XcodebuildBuilder) exportTestBundle(outputDir, builtTestDir string, xctestrunPths []string) error {
	// BITRISE_TEST_DIR_PATH
	tmpDir, err := v2pathutil.NewPathProvider().CreateTempDir("test_bundle")
	if err != nil {
		return err
	}

	if err := command.CopyDir(builtTestDir, tmpDir, false); err != nil {
		return err
	}
	copiedTestDirDestination := filepath.Join(tmpDir, filepath.Base(builtTestDir))

	var copiedXctestrunPths []string
	for _, xctestrunPth := range xctestrunPths {
		xctestrunDestination := filepath.Join(tmpDir, filepath.Base(xctestrunPth))
		if err := command.CopyFile(xctestrunPth, xctestrunDestination); err != nil {
			return err
		}
		copiedXctestrunPths = append(copiedXctestrunPths, xctestrunDestination)
	}

	if err := tools.ExportEnvironmentWithEnvman(testBundlePathEnvKey, tmpDir); err != nil {
		return err
	}
	b.logger.Donef("The test bundle directory is available in %s env: %s", testBundlePathEnvKey, tmpDir)

	// BITRISE_TEST_BUNDLE_ZIP_PATH
	testBundleZipPth := filepath.Join(outputDir, "testbundle.zip")
	if err := output.ZipAndExportOutput([]string{tmpDir}, testBundleZipPth, testBundleZipPathEnvKey); err != nil {
		return err
	}
	b.logger.Donef("The zipped test bundle is available in %s env: %s", testBundleZipPathEnvKey, testBundleZipPth)

	// BITRISE_TEST_DIR_PATH
	if err := output.ExportOutputDir(copiedTestDirDestination, copiedTestDirDestination, builtTargetBinariesDirEnvKey); err != nil {
		return err
	}
	b.logger.Donef("The built target directory is available in %s env: %s", builtTargetBinariesDirEnvKey, copiedTestDirDestination)

	// BITRISE_XCTESTRUN_FILE_PATH
	// TODO: export the scheme's default Test Plan's xctestrun
	xctestrunPth := copiedXctestrunPths[0]
	if len(copiedXctestrunPths) > 0 {
		log.Warnf("Multiple xctesrun files generated, exporting the first (%s) under BITRISE_XCTESTRUN_FILE_PATH", xctestrunPth)
	}
	if err := output.ExportOutputFile(xctestrunPth, xctestrunPth, xctestrunPathEnvKey); err != nil {
		return err
	}
	b.logger.Donef("The built xctestrun file is available in %s env: %s", xctestrunPathEnvKey, xctestrunPth)

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
	ProjectPath       string
	Scheme            string
	Configuration     string
	XcodebuildOptions []string
	BuildInterval     timeInterval
}

type testBundle struct {
	BuiltTestDir  string
	XctestrunPths []string
	SYMRoot       string
}

func (b XcodebuildBuilder) findTestBundle(opts findTestBundleOpts) (testBundle, error) {
	buildSettings, err := b.xcodebuild.ShowBuildSettings(opts.ProjectPath, opts.Scheme, opts.Configuration, "build-for-testing", opts.XcodebuildOptions)
	if err != nil {
		return testBundle{}, fmt.Errorf("failed to read build settings: %w", err)
	}

	// The path at which all products will be placed when performing a build. Typically, this path is not set per target, but is set per-project or per-user.
	symRoot, err := buildSettings.String("SYMROOT")
	if err != nil {
		return testBundle{}, fmt.Errorf("failed to get SYMROOT build setting: %w", err)
	}
	b.logger.Printf("SYMROOT: %s", symRoot)

	configuration, err := buildSettings.String("CONFIGURATION")
	if err != nil {
		return testBundle{}, fmt.Errorf("failed to get CONFIGURATION build setting: %w", err)
	}
	b.logger.Printf("CONFIGURATION: %s", configuration)

	// Without better solution the step collects every xctestrun files and filters them for the build time frame
	xctestrunPths, err := b.findBuiltXCTestrunFiles(symRoot, opts.Scheme, opts.BuildInterval.start, opts.BuildInterval.end)
	if err != nil {
		return testBundle{}, fmt.Errorf("failed to find built xctestrun file(s): %w", err)
	}

	if len(xctestrunPths) == 0 {
		return testBundle{}, fmt.Errorf("no xctestrun file generated during the build")
	}

	b.logger.Donef("xctestrun file(s) generated during the build:\n- %s", strings.Join(xctestrunPths, "\n- "))

	builtTestDir, err := b.builtTestDirPath(xctestrunPths, symRoot, opts.Configuration)
	if err != nil {
		return testBundle{}, fmt.Errorf("failed to find built test directory: %w", err)
	}

	b.logger.Donef("Built test directory: %s", builtTestDir)

	return testBundle{
		BuiltTestDir:  builtTestDir,
		XctestrunPths: xctestrunPths,
		SYMRoot:       symRoot,
	}, nil
}

func (b XcodebuildBuilder) findBuiltXCTestrunFiles(symRoot, scheme string, buildStart, buildEnd time.Time) ([]string, error) {
	xctestrunPthPattern := xctestrunPathPattern(symRoot, scheme)
	xctestrunPths, err := b.filepathGlober.Glob(xctestrunPthPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to search for xctestrun file using pattern (%s): %w", xctestrunPthPattern, err)
	}
	//b.logger.Printf("xctestrun paths: %s", strings.Join(xctestrunPths, ", "))

	//if len(xctestrunPths) == 0 {
	//	return testBundle{}, fmt.Errorf("no xctestrun file found with pattern: %s", xctestrunPthPattern)
	//}

	var builtXCTestrunPths []string
	for _, xctestrunPth := range xctestrunPths {
		if modified, err := b.modtimeChecker.ModifiedInTimeFrame(xctestrunPth, buildStart, buildEnd); err != nil {
			return nil, err
		} else if modified {
			builtXCTestrunPths = append(builtXCTestrunPths, xctestrunPth)
		}
	}

	return builtXCTestrunPths, nil
}

func (b XcodebuildBuilder) builtTestDirPath(xctestrunPths []string, symRoot, configuration string) (string, error) {
	// Without better solution the step determines the build target based on the xctestrun file name.
	// xctestrun file name layout (without Test Plans): <scheme>_<destination>.xctestrun
	// 	example: ios-simple-objc_iphonesimulator12.0-x86_64.xctestrun
	//
	// xctestrun file name layout with Test Plans: <scheme>_<test_plan>_<destination>.xctestrun
	//	example: BullsEye_FullTests_iphonesimulator15.5-arm64-x86_64.xctestrun
	var builtForDestination string
	for _, xctestrunPth := range xctestrunPths {
		var destination string
		if strings.Contains(xctestrunPth, "_iphonesimulator") {
			destination = "iphonesimulator"
		} else {
			destination = "iphoneos"
		}

		if builtForDestination == "" {
			builtForDestination = destination
		} else if builtForDestination != destination {
			return "", fmt.Errorf("xctestrun files with different destinations")
		}
	}

	builtTestDir := filepath.Join(symRoot, fmt.Sprintf("%s-%s", configuration, builtForDestination))
	if exist, err := b.pathChecker.IsPathExists(builtTestDir); err != nil {
		return "", fmt.Errorf("failed to check if built test directory exists (%s): %w", builtTestDir, err)
	} else if !exist {
		return "", fmt.Errorf("built test directory does not exist at: %s", builtTestDir)
	}

	return builtTestDir, nil
}

func xctestrunPathPattern(symRoot, scheme string) string {
	return filepath.Join(symRoot, fmt.Sprintf("%s*.xctestrun", scheme))
}
