package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bitrise-io/go-steputils/output"
	"github.com/bitrise-io/go-steputils/tools"
	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/bitrise-io/go-utils/colorstring"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/errorutil"
	"github.com/bitrise-io/go-utils/fileutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-io/go-utils/stringutil"
	v2command "github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/env"
	v2fileutil "github.com/bitrise-io/go-utils/v2/fileutil"
	v2log "github.com/bitrise-io/go-utils/v2/log"
	v2pathutil "github.com/bitrise-io/go-utils/v2/pathutil"
	"github.com/bitrise-io/go-xcode/utility"
	"github.com/bitrise-io/go-xcode/v2/codesign"
	"github.com/bitrise-io/go-xcode/v2/xcconfig"
	"github.com/bitrise-io/go-xcode/v2/xcpretty"
	"github.com/bitrise-io/go-xcode/xcodebuild"
	cache "github.com/bitrise-io/go-xcode/xcodecache"
	"github.com/bitrise-io/go-xcode/xcodeproject/xcworkspace"
	"github.com/kballard/go-shellquote"
)

const (
	xcodebuildLogPath = "BITRISE_XCODE_RAW_RESULT_TEXT_PATH"
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
	XCConfigContent        string
	XcodebuildOptions      []string
	XCPretty               bool
	CodesignManager        *codesign.Manager
	OutputDir              string
	XcodebuildMajorVersion int
	CacheLevel             string
	SwiftPackagesPath      string
}

type TestBuilder struct {
}

func NewTestBuilder() TestBuilder {
	return TestBuilder{}
}

func (b TestBuilder) ProcessConfig() (Config, error) {
	var input Input
	parser := stepconf.NewInputParser(env.NewRepository())
	if err := parser.Parse(&input); err != nil {
		return Config{}, err
	}
	logger := v2log.NewLogger()
	logger.EnableDebugLog(input.VerboseLog)
	log.SetEnableDebugLog(input.VerboseLog)

	stepconf.Print(input)
	fmt.Println()

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
	log.Infof("Xcode version: %s (%s)", xcodebuildVersion.Version, xcodebuildVersion.BuildVersion)

	var swiftPackagesPath string
	if xcodebuildVersion.MajorVersion >= 11 {
		var err error
		if swiftPackagesPath, err = cache.SwiftPackagesPath(absProjectPath); err != nil {
			return Config{}, fmt.Errorf("failed to get swift packages path: %w", err)
		}
	}

	var customOptions []string
	if input.XcodebuildOptions != "" {
		customOptions, err = shellquote.Split(input.XcodebuildOptions)
		if err != nil {
			return Config{}, fmt.Errorf("failed to parse additional options (%s): %w", input.XcodebuildOptions, err)
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
		}, xcodebuildVersion.MajorVersion, logger, factory)
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
		XCConfigContent:        input.XCConfigContent,
		XcodebuildOptions:      customOptions,
		XCPretty:               input.LogFormatter == "xcpretty",
		CodesignManager:        codesignManager,
		OutputDir:              absOutputDir,
		XcodebuildMajorVersion: int(xcodebuildVersion.MajorVersion),
		CacheLevel:             input.CacheLevel,
		SwiftPackagesPath:      swiftPackagesPath,
	}, nil
}

// InstallDependencies ...
func (b TestBuilder) InstallDependencies(useXCPretty bool) error {
	if !useXCPretty {
		return nil
	}

	fmt.Println()
	log.Infof("Checking if output tool (xcpretty) is installed")
	formatter := xcpretty.NewXcpretty()

	installed, err := formatter.IsInstalled()
	if err != nil {
		return err
	}

	if !installed {
		log.Warnf("xcpretty is not installed")
		fmt.Println()
		log.Printf("Installing xcpretty")

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

	log.Printf("- xcpretty version: %s", xcprettyVersion.String())
	return nil
}

// RunOpts ...
type RunOpts struct {
	XCPretty          bool
	CodesignManager   *codesign.Manager
	SwiftPackagesPath string
	OutputDir         string

	ProjectPath       string
	Scheme            string
	Configuration     string
	Destination       string
	XCConfigContent   string
	XcodebuildOptions []string
}

// RunOut ...
type RunOut struct {
	BuildInterval timeInterval
	XcodebuildLog string
}

// Run ...
func (b TestBuilder) Run(opts RunOpts) (RunOut, error) {
	//
	// Manage code signing
	var authOptions *xcodebuild.AuthenticationParams = nil
	if opts.CodesignManager != nil {
		log.Infof("Preparing code signing assets (certificates, profiles)")

		xcodebuildAuthParams, err := opts.CodesignManager.PrepareCodesigning()
		if err != nil {
			return RunOut{}, fmt.Errorf("failed to prepare code signing assets: %s", err)
		}

		if xcodebuildAuthParams != nil {
			privateKey, err := xcodebuildAuthParams.WritePrivateKeyToFile()
			if err != nil {
				return RunOut{}, err
			}

			defer func() {
				if err := os.Remove(privateKey); err != nil {
					log.Warnf("failed to remove private key file: %s", err)
				}
			}()

			authOptions = &xcodebuild.AuthenticationParams{
				KeyID:     xcodebuildAuthParams.KeyID,
				IsssuerID: xcodebuildAuthParams.IssuerID,
				KeyPath:   privateKey,
			}
		}
	} else {
		log.Infof("Automatic code signing is disabled, skipped downloading code sign assets")
	}

	//
	// Build
	log.Infof("Build:")

	xcconfigWriter := xcconfig.NewWriter(v2pathutil.NewPathProvider(), v2fileutil.NewFileManager())
	xcconfigPath, err := xcconfigWriter.Write(opts.XCConfigContent)
	if err != nil {
		return RunOut{}, err
	}

	xcodeBuildCmd := xcodebuild.NewCommandBuilder(opts.ProjectPath, xcworkspace.IsWorkspace(opts.ProjectPath), "")
	xcodeBuildCmd.SetScheme(opts.Scheme)
	xcodeBuildCmd.SetConfiguration(opts.Configuration)
	xcodeBuildCmd.SetCustomBuildAction("build-for-testing")
	xcodeBuildCmd.SetDestination(opts.Destination)
	xcodeBuildCmd.SetCustomOptions(opts.XcodebuildOptions)
	xcodeBuildCmd.SetXCConfigPath(xcconfigPath)
	if authOptions != nil {
		xcodeBuildCmd.SetAuthentication(*authOptions)
	}

	// save the build time frame to find the build generated artifacts
	rawXcodebuildOut, buildInterval, err := runCommandWithRetry(xcodeBuildCmd, opts.XCPretty, opts.SwiftPackagesPath)
	if err != nil || !opts.XCPretty {
		printLastLinesOfXcodebuildTestLog(rawXcodebuildOut, err == nil)
	}

	return RunOut{
		BuildInterval: buildInterval,
		XcodebuildLog: rawXcodebuildOut,
	}, nil
}

// ExportOpts ...
type ExportOpts struct {
	OutputDir         string
	ProjectPath       string
	Scheme            string
	Configuration     string
	XcodebuildOptions []string
	BuildInterval     timeInterval
	CacheLevel        string
	XcodebuildTestLog string
}

// ExportOutput ...
func (b TestBuilder) ExportOutput(opts ExportOpts) error {
	log.Infof("Export:")

	if opts.XcodebuildTestLog != "" {
		if err := exportXcodebuildTestLog(opts.OutputDir, opts.XcodebuildTestLog); err != nil {
			return err
		}
	}

	buildSettingsCmd := xcodebuild.NewShowBuildSettingsCommand(opts.ProjectPath)
	buildSettingsCmd.SetScheme(opts.Scheme)
	buildSettingsCmd.SetConfiguration(opts.Configuration)
	buildSettingsCmd.SetCustomOptions(append([]string{"build-for-testing"}, opts.XcodebuildOptions...))

	fmt.Println()
	log.Donef("$ %s", buildSettingsCmd.PrintableCmd())

	buildSettings, err := buildSettingsCmd.RunAndReturnSettings()
	if err != nil {
		return fmt.Errorf("failed to read build settings: %s", err)
	}

	// The path at which all products will be placed when performing a build. Typically this path is not set per target, but is set per-project or per-user.
	symRoot, err := buildSettings.String("SYMROOT")
	if err != nil {
		return fmt.Errorf("failed to parse SYMROOT build setting: %s", err)
	}
	log.Printf("SYMROOT: %s", symRoot)

	configuration, err := buildSettings.String("CONFIGURATION")
	if err != nil {
		return fmt.Errorf("failed to parse CONFIGURATION build setting: %s", err)
	}
	log.Printf("CONFIGURATION: %s", configuration)

	// Without better solution the step collects every xctestrun files and filters them for the build time frame
	xctestrunPthPattern := filepath.Join(symRoot, fmt.Sprintf("%s*.xctestrun", opts.Scheme))
	xctestrunPths, err := filepath.Glob(xctestrunPthPattern)
	if err != nil {
		return fmt.Errorf("failed to search for xctestrun file using pattern: %s, error: %s", xctestrunPthPattern, err)
	}
	log.Printf("xctestrun paths: %s", strings.Join(xctestrunPths, ", "))

	if len(xctestrunPths) == 0 {
		return fmt.Errorf("no xctestrun file found with pattern: %s", xctestrunPthPattern)
	}

	var buildXCTestrunPths []string
	for _, xctestrunPth := range xctestrunPths {
		info, err := os.Stat(xctestrunPth)
		if err != nil {
			return fmt.Errorf("failed to check %s modtime: %s", xctestrunPth, err)
		}

		if !info.ModTime().Before(opts.BuildInterval.start) && !info.ModTime().After(opts.BuildInterval.end) {
			buildXCTestrunPths = append(buildXCTestrunPths, xctestrunPth)
		}
	}

	if len(buildXCTestrunPths) == 0 {
		return fmt.Errorf("no xctestrun file generated during the build")
	} else if len(buildXCTestrunPths) > 1 {
		return fmt.Errorf("multiple xctestrun file generated during the build:\n%s", strings.Join(buildXCTestrunPths, "\n- "))
	}

	xctestrunPth := buildXCTestrunPths[0]
	log.Printf("Built xctestrun path: %s", xctestrunPth)

	// Without better solution the step determines the build target based on the xctestrun file name
	// ios-simple-objc_iphonesimulator12.0-x86_64.xctestrun
	var builtForDestination string
	if strings.Contains(xctestrunPth, fmt.Sprintf("%s_iphonesimulator", opts.Scheme)) {
		builtForDestination = "iphonesimulator"
	} else {
		builtForDestination = "iphoneos"
	}

	builtTestDir := filepath.Join(symRoot, fmt.Sprintf("%s-%s", configuration, builtForDestination))
	if exist, err := pathutil.IsPathExists(builtTestDir); err != nil {
		return fmt.Errorf("failed to check if built test directory exists at: %s, error: %s", builtTestDir, err)
	} else if !exist {
		return fmt.Errorf("built test directory does not exist at: %s", builtTestDir)
	}
	log.Printf("Built test directory: %s", builtTestDir)

	outputTestBundleZipPath := filepath.Join(opts.OutputDir, "testbundle.zip")
	factory := v2command.NewFactory(env.NewRepository())
	zipCmd := factory.Create("zip", []string{"-r", outputTestBundleZipPath, filepath.Base(builtTestDir), filepath.Base(xctestrunPth)}, &v2command.Opts{
		Dir: symRoot,
	})
	if out, err := zipCmd.RunAndReturnTrimmedCombinedOutput(); err != nil {
		if errorutil.IsExitStatusError(err) {
			return fmt.Errorf("%s failed: %s", zipCmd.PrintableCommandArgs(), out)
		} else {
			return fmt.Errorf("%s failed: %s", zipCmd.PrintableCommandArgs(), err)
		}
	}
	log.Printf("Zipped test bundle: %s", outputTestBundleZipPath)

	outputXCTestrunPth := filepath.Join(opts.OutputDir, filepath.Base(xctestrunPth))
	if err := output.ExportOutputFile(xctestrunPth, outputXCTestrunPth, "BITRISE_XCTESTRUN_FILE_PATH"); err != nil {
		return fmt.Errorf("failed to export BITRISE_XCTESTRUN_FILE_PATH: %s", err)
	}
	log.Donef("The built xctestrun file is available in BITRISE_XCTESTRUN_FILE_PATH env: %s", outputXCTestrunPth)

	outputTestDirPath := filepath.Join(opts.OutputDir, filepath.Base(builtTestDir))
	if err := output.ExportOutputDir(builtTestDir, outputTestDirPath, "BITRISE_TEST_DIR_PATH"); err != nil {
		return fmt.Errorf("failed to export BITRISE_TEST_DIR_PATH: %s", err)
	}
	log.Donef("The built test directory is available in BITRISE_TEST_DIR_PATH env: %s", outputTestDirPath)

	if err := tools.ExportEnvironmentWithEnvman("BITRISE_TEST_BUNDLE_ZIP_PATH", outputTestBundleZipPath); err != nil {
		return fmt.Errorf("failed to export BITRISE_TEST_BUNDLE_ZIP_PATH: %s", err)
	}

	// Cache swift PM
	if opts.CacheLevel == "swift_packages" {
		if err := cache.CollectSwiftPackages(opts.ProjectPath); err != nil {
			log.Warnf("Failed to mark swift packages for caching, error: %s", err)
		}
	}

	log.Donef("The zipped test bundle is available in BITRISE_TEST_BUNDLE_ZIP_PATH env: %s", outputTestBundleZipPath)
	return nil
}

func printLastLinesOfXcodebuildTestLog(rawXcodebuildOutput string, isRunSuccess bool) {
	const lastLines = "\nLast lines of the build log:"
	if !isRunSuccess {
		log.Errorf(lastLines)
	} else {
		log.Infof(lastLines)
	}

	fmt.Println(stringutil.LastNLines(rawXcodebuildOutput, 20))

	if !isRunSuccess {
		log.Warnf("If you can't find the reason of the error in the log, please check the xcodebuild_test.log.")
	}

	log.Infof(colorstring.Magenta(`
The log file is stored in $BITRISE_DEPLOY_DIR, and its full path
is available in the $BITRISE_XCODEBUILD_TEST_LOG_PATH environment variable.
If you have the Deploy to Bitrise.io step (after this step),
that will attach the file to your build as an artifact!`))
}

func exportXcodebuildTestLog(deployDir, xcodebuildTestLog string) error {
	pth, err := saveRawOutputToLogFile(xcodebuildTestLog)
	if err != nil {
		log.Warnf("Failed to save the Raw Output, error: %s", err)
	}

	deployPth := filepath.Join(deployDir, "xcodebuild_test.log")
	if err := command.CopyFile(pth, deployPth); err != nil {
		return fmt.Errorf("failed to copy xcodebuild output log file from (%s) to (%s), error: %s", pth, deployPth, err)
	}

	if err := tools.ExportEnvironmentWithEnvman(xcodebuildLogPath, deployPth); err != nil {
		log.Warnf("Failed to export: %s, error: %s", xcodebuildLogPath, err)
	}

	return nil
}

func saveRawOutputToLogFile(rawXcodebuildOutput string) (string, error) {
	tmpDir, err := pathutil.NormalizedOSTempDirPath("xcodebuild-output")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir, error: %s", err)
	}
	logFileName := "raw-xcodebuild-output.log"
	logPth := filepath.Join(tmpDir, logFileName)
	if err := fileutil.WriteStringToFile(logPth, rawXcodebuildOutput); err != nil {
		return "", fmt.Errorf("failed to write xcodebuild output to file, error: %s", err)
	}

	return logPth, nil
}
