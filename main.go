package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bitrise-io/go-steputils/output"
	"github.com/bitrise-io/go-steputils/stepconf"
	"github.com/bitrise-io/go-steputils/tools"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/env"
	"github.com/bitrise-io/go-utils/errorutil"
	"github.com/bitrise-io/go-utils/fileutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-io/go-utils/stringutil"
	"github.com/bitrise-io/go-xcode/codesign"
	"github.com/bitrise-io/go-xcode/utility"
	"github.com/bitrise-io/go-xcode/xcconfig"
	"github.com/bitrise-io/go-xcode/xcodebuild"
	cache "github.com/bitrise-io/go-xcode/xcodecache"
	"github.com/bitrise-io/go-xcode/xcodeproject/xcworkspace"
	"github.com/bitrise-io/go-xcode/xcpretty"
	"github.com/kballard/go-shellquote"
)

const (
	xcodebuildLogPath = "BITRISE_XCODE_RAW_RESULT_TEXT_PATH"

	// Code Signing Authentication Source
	codeSignSourceOff     = "off"
	codeSignSourceAPIKey  = "api-key"
	codeSignSourceAppleID = "apple-id"
)

// Config ...
type Config struct {
	ProjectPath   string `env:"project_path,required"`
	Scheme        string `env:"scheme,required"`
	Configuration string `env:"configuration"`
	Destination   string `env:"destination,required"`

	XCConfigContent   string `env:"xcconfig_content"`
	XcodebuildOptions string `env:"xcodebuild_options"`

	LogFormatter string `env:"log_formatter,opt[xcpretty,xcodebuild]"`

	OutputDir string `env:"output_dir,required"`

	CacheLevel string `env:"cache_level,opt[none,swift_packages]"`

	VerboseLog bool `env:"verbose_log,opt[yes,no]"`

	CodeSigningAuthSource     string          `env:"automatic_code_signing,opt[off,api-key,apple-id]"`
	CertificateURLList        string          `env:"certificate_url_list"`
	CertificatePassphraseList stepconf.Secret `env:"passphrase_list"`
	KeychainPath              string          `env:"keychain_path"`
	KeychainPassword          stepconf.Secret `env:"keychain_password"`
	RegisterTestDevices       bool            `env:"register_test_devices,opt[yes,no]"`
	MinDaysProfileValid       int             `env:"min_profile_validity,required"`
	TeamID                    string          `env:"apple_team_id"`
	BuildURL                  string          `env:"BITRISE_BUILD_URL"`
	BuildAPIToken             stepconf.Secret `env:"BITRISE_BUILD_API_TOKEN"`
}

func main() {
	//
	// Config
	var cfg Config
	parser := stepconf.NewInputParser(env.NewRepository())
	if err := parser.Parse(&cfg); err != nil {
		failf("Issue with input: %s", err)
	}
	logger := log.NewLogger()
	logger.EnableDebugLog(cfg.VerboseLog)
	log.SetEnableDebugLog(cfg.VerboseLog)

	stepconf.Print(cfg)
	fmt.Println()

	absProjectPath, err := filepath.Abs(cfg.ProjectPath)
	if err != nil {
		failf("Failed to expand ProjectPath (%s), error: %s", cfg.ProjectPath, err)
	}

	// abs out dir pth
	absOutputDir, err := pathutil.AbsPath(cfg.OutputDir)
	if err != nil {
		failf("Failed to expand OutputDir (%s), error: %s", cfg.OutputDir, err)
	}

	if exist, err := pathutil.IsPathExists(absOutputDir); err != nil {
		failf("Failed to check if OutputDir exist, error: %s", err)
	} else if !exist {
		if err := os.MkdirAll(absOutputDir, 0777); err != nil {
			failf("Failed to create OutputDir (%s), error: %s", absOutputDir, err)
		}
	}

	// Output files
	rawXcodebuildOutputLogPath := filepath.Join(absOutputDir, "raw-xcodebuild-output.log")

	//
	// Ensure xcpretty
	// only if output tool is set to xcpretty
	if cfg.LogFormatter == "xcpretty" {
		log.Infof("Output tool check:")

		var xcpretty = xcpretty.NewXcpretty()
		// check if already installed
		if installed, err := xcpretty.IsInstalled(); err != nil {
			log.Warnf(" Failed to check if xcpretty is installed, error: %s", err)
			cfg.LogFormatter = "xcodebuild"
		} else if !installed {
			log.Warnf(` xcpretty is not installed`)
			log.Printf(" Installing...")

			// install if not installed
			if cmds, err := xcpretty.Install(); err != nil {
				log.Warnf(" Failed to install xcpretty, error: %s", err)
				cfg.LogFormatter = "xcodebuild"
			} else {
				for _, cmd := range cmds {
					log.Donef(" $ %s", cmd.PrintableCommandArgs())
					if err := cmd.Run(); err != nil {
						log.Warnf(" Failed to install xcpretty, error: %s", err)
						cfg.LogFormatter = "xcodebuild"
						break
					}
				}
			}
		} else {
			// already installed
			log.Donef(` xcpretty is installed`)
		}
		// warn user if we needed to switch back from xcpretty
		if cfg.LogFormatter != "xcpretty" {
			log.Warnf(" Switching output tool to xcodebuild")
		}
		fmt.Println()
	}

	// Detect Xcode major version
	factory := command.NewFactory(env.NewRepository())
	xcodebuildVersion, err := utility.GetXcodeVersion(factory)
	if err != nil {
		failf("Failed to determin xcode version, error: %s", err)
	}
	log.Infof("Xcode version: %s (%s)", xcodebuildVersion.Version, xcodebuildVersion.BuildVersion)

	var swiftPackagesPath string
	if xcodebuildVersion.MajorVersion >= 11 {
		var err error
		if swiftPackagesPath, err = cache.SwiftPackagesPath(absProjectPath); err != nil {
			failf("Failed to get Swift Packages path, error: %s", err)
		}
	}

	var codesignManager *codesign.Manager = nil
	if cfg.CodeSigningAuthSource != codeSignSourceOff {
		codesignMgr, err := createCodesignManager(cfg, xcodebuildVersion.MajorVersion, logger, factory)
		if err != nil {
			failf("%s", err)
		}
		codesignManager = &codesignMgr
	}

	// manage code signing
	var authOptions *xcodebuild.AuthenticationParams = nil
	if codesignManager != nil {
		log.Infof("Preparing code signing assets (certificates, profiles)")

		xcodebuildAuthParams, err := codesignManager.PrepareCodesigning()
		if err != nil {
			failf("Failed to prepare code signing assets: %s", err)
		}

		if xcodebuildAuthParams != nil {
			privateKey, err := xcodebuildAuthParams.WritePrivateKeyToFile()
			if err != nil {
				failf("%s", err)
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

	var customOptions []string
	// parse custom flags
	if cfg.XcodebuildOptions != "" {
		customOptions, err = shellquote.Split(cfg.XcodebuildOptions)
		if err != nil {
			failf("Failed to shell split XcodebuildOptions (%s), error: %s", cfg.XcodebuildOptions)
		}
	}

	xcconfigWriter := xcconfig.NewWriter(pathutil.NewPathProvider(), fileutil.NewFileManager())
	xcconfigPath, err := xcconfigWriter.Write(cfg.XCConfigContent)
	if err != nil {
		failf(err.Error())
	}

	xcodeBuildCmd := xcodebuild.NewCommandBuilder(absProjectPath, xcworkspace.IsWorkspace(absProjectPath), "", factory)
	xcodeBuildCmd.SetScheme(cfg.Scheme)
	xcodeBuildCmd.SetConfiguration(cfg.Configuration)
	xcodeBuildCmd.SetCustomBuildAction("build-for-testing")
	xcodeBuildCmd.SetDestination(cfg.Destination)
	xcodeBuildCmd.SetCustomOptions(customOptions)
	xcodeBuildCmd.SetXCConfigPath(xcconfigPath)
	if authOptions != nil {
		xcodeBuildCmd.SetAuthentication(*authOptions)
	}

	// save the build time frame to find the build generated artifacts
	rawXcodebuildOut, buildInterval, xcodebuildErr := runCommandWithRetry(xcodeBuildCmd, cfg.LogFormatter == "xcpretty", swiftPackagesPath)

	if err := output.ExportOutputFileContent(rawXcodebuildOut, rawXcodebuildOutputLogPath, xcodebuildLogPath); err != nil {
		log.Warnf("Failed to export %s, error: %s", xcodebuildLogPath, err)
	}
	log.Donef("The xcodebuild command log file path is available in BITRISE_XCODE_RAW_RESULT_TEXT_PATH env: %s", rawXcodebuildOutputLogPath)

	if xcodebuildErr != nil {
		if cfg.LogFormatter == "xcpretty" {
			log.Errorf("\nLast lines of the Xcode's build log:")
			fmt.Println(stringutil.LastNLines(rawXcodebuildOut, 10))
		}
		failf("Build failed, error: %s", xcodebuildErr)
	}
	fmt.Println()

	//
	// Export
	log.Infof("Export:")

	buildSettingsCmd := xcodebuild.NewShowBuildSettingsCommand(absProjectPath, factory)
	buildSettingsCmd.SetScheme(cfg.Scheme)
	buildSettingsCmd.SetConfiguration(cfg.Configuration)
	buildSettingsCmd.SetCustomOptions(append([]string{"build-for-testing"}, customOptions...))

	fmt.Println()
	log.Donef("$ %s", buildSettingsCmd.PrintableCmd())

	buildSettings, err := buildSettingsCmd.RunAndReturnSettings()
	if err != nil {
		failf("failed to read build settings: %s", err)
	}

	// The path at which all products will be placed when performing a build. Typically this path is not set per target, but is set per-project or per-user.
	symRoot, err := buildSettings.String("SYMROOT")
	if err != nil {
		failf("Failed to parse SYMROOT build setting: %s", err)
	}
	log.Printf("SYMROOT: %s", symRoot)

	configuration, err := buildSettings.String("CONFIGURATION")
	if err != nil {
		failf("Failed to parse CONFIGURATION build setting: %s", err)
	}
	log.Printf("CONFIGURATION: %s", configuration)

	// Without better solution the step collects every xctestrun files and filters them for the build time frame
	xctestrunPthPattern := filepath.Join(symRoot, fmt.Sprintf("%s*.xctestrun", cfg.Scheme))
	xctestrunPths, err := filepath.Glob(xctestrunPthPattern)
	if err != nil {
		failf("Failed to search for xctestrun file using pattern: %s, error: %s", xctestrunPthPattern, err)
	}
	log.Printf("xctestrun paths: %s", strings.Join(xctestrunPths, ", "))

	if len(xctestrunPths) == 0 {
		failf("No xctestrun file found with pattern: %s", xctestrunPthPattern)
	}

	var buildXCTestrunPths []string
	for _, xctestrunPth := range xctestrunPths {
		info, err := os.Stat(xctestrunPth)
		if err != nil {
			failf("Failed to check %s modtime: %s", xctestrunPth, err)
		}

		if !info.ModTime().Before(buildInterval.start) && !info.ModTime().After(buildInterval.end) {
			buildXCTestrunPths = append(buildXCTestrunPths, xctestrunPth)
		}
	}

	if len(buildXCTestrunPths) == 0 {
		failf("No xctestrun file generated during the build")
	} else if len(buildXCTestrunPths) > 1 {
		failf("Multiple xctestrun file generated during the build:\n%s", strings.Join(buildXCTestrunPths, "\n- "))
	}

	xctestrunPth := buildXCTestrunPths[0]
	log.Printf("Built xctestrun path: %s", xctestrunPth)

	// Without better solution the step determines the build target based on the xctestrun file name
	// ios-simple-objc_iphonesimulator12.0-x86_64.xctestrun
	var builtForDestination string
	if strings.Contains(xctestrunPth, fmt.Sprintf("%s_iphonesimulator", cfg.Scheme)) {
		builtForDestination = "iphonesimulator"
	} else {
		builtForDestination = "iphoneos"
	}

	builtTestDir := filepath.Join(symRoot, fmt.Sprintf("%s-%s", configuration, builtForDestination))
	if exist, err := pathutil.IsPathExists(builtTestDir); err != nil {
		failf("Failed to check if built test directory exists at: %s, error: %s", builtTestDir, err)
	} else if !exist {
		failf("built test directory does not exist at: %s", builtTestDir)
	}
	log.Printf("Built test directory: %s", builtTestDir)

	outputTestBundleZipPath := filepath.Join(absOutputDir, "testbundle.zip")
	zipCmd := factory.Create("zip", []string{"-r", outputTestBundleZipPath, filepath.Base(builtTestDir), filepath.Base(xctestrunPth)}, &command.Opts{
		Dir: symRoot,
	})
	if out, err := zipCmd.RunAndReturnTrimmedCombinedOutput(); err != nil {
		if errorutil.IsExitStatusError(err) {
			failf("%s failed: %s", zipCmd.PrintableCommandArgs(), out)
		} else {
			failf("%s failed: %s", zipCmd.PrintableCommandArgs(), err)
		}
	}
	log.Printf("Zipped test bundle: %s", outputTestBundleZipPath)

	outputXCTestrunPth := filepath.Join(absOutputDir, filepath.Base(xctestrunPth))
	if err := output.ExportOutputFile(xctestrunPth, outputXCTestrunPth, "BITRISE_XCTESTRUN_FILE_PATH"); err != nil {
		failf("Failed to export BITRISE_XCTESTRUN_FILE_PATH: %s", err)
	}
	log.Donef("The built xctestrun file is available in BITRISE_XCTESTRUN_FILE_PATH env: %s", outputXCTestrunPth)

	outputTestDirPath := filepath.Join(absOutputDir, filepath.Base(builtTestDir))
	if err := output.ExportOutputDir(builtTestDir, outputTestDirPath, "BITRISE_TEST_DIR_PATH"); err != nil {
		failf("Failed to export BITRISE_TEST_DIR_PATH: %s", err)
	}
	log.Donef("The built test directory is available in BITRISE_TEST_DIR_PATH env: %s", outputTestDirPath)

	if err := tools.ExportEnvironmentWithEnvman("BITRISE_TEST_BUNDLE_ZIP_PATH", outputTestBundleZipPath); err != nil {
		failf("Failed to export BITRISE_TEST_BUNDLE_ZIP_PATH: %s", err)
	}

	// Cache swift PM
	if xcodebuildVersion.MajorVersion >= 11 && cfg.CacheLevel == "swift_packages" {
		if err := cache.CollectSwiftPackages(absProjectPath); err != nil {
			log.Warnf("Failed to mark swift packages for caching, error: %s", err)
		}
	}

	log.Donef("The zipped test bundle is available in BITRISE_TEST_BUNDLE_ZIP_PATH env: %s", outputTestBundleZipPath)
}

func failf(format string, v ...interface{}) {
	log.Errorf(format, v...)
	os.Exit(1)
}
