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
	"github.com/bitrise-io/go-xcode/utility"
	"github.com/bitrise-io/go-xcode/xcodebuild"
	cache "github.com/bitrise-io/go-xcode/xcodecache"
	"github.com/bitrise-io/go-xcode/xcodeproject/serialized"
	"github.com/bitrise-io/go-xcode/xcodeproject/xcworkspace"
	"github.com/bitrise-io/go-xcode/xcpretty"
	"github.com/kballard/go-shellquote"
)

const bitriseXcodeRawResultTextEnvKey = "BITRISE_XCODE_RAW_RESULT_TEXT_PATH"

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
}

func main() {
	//
	// Config
	var cfg Config
	parser := stepconf.NewInputParser(env.NewRepository())
	if err := parser.Parse(&cfg); err != nil {
		failf("Issue with input: %s", err)
	}
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

	var xcconfigPath string
	if cfg.XCConfigContent != "" {
		dir, err := pathutil.NewPathProvider().CreateTempDir("")
		if err != nil {
			failf("Unable to create temp dir for writing XCConfig: %s", err)
		}
		xcconfigPath = filepath.Join(dir, "temp.xcconfig")

		if err = fileutil.NewFileManager().Write(xcconfigPath, cfg.XCConfigContent, 0644); err != nil {
			failf("unable to write XCConfig content into file: %s", err, err)
		}
	}

	xcodeBuildCmd := xcodebuild.NewCommandBuilder(absProjectPath, xcworkspace.IsWorkspace(absProjectPath), "", factory)
	xcodeBuildCmd.SetScheme(cfg.Scheme)
	xcodeBuildCmd.SetConfiguration(cfg.Configuration)
	xcodeBuildCmd.SetCustomBuildAction("build-for-testing")
	xcodeBuildCmd.SetDestination(cfg.Destination)
	xcodeBuildCmd.SetCustomOptions(customOptions)
	xcodeBuildCmd.SetXCConfigPath(xcconfigPath)

	// save the build time frame to find the build generated artifacts
	rawXcodebuildOut, buildInterval, xcodebuildErr := runCommandWithRetry(xcodeBuildCmd, cfg.LogFormatter == "xcpretty", swiftPackagesPath)

	if err := output.ExportOutputFileContent(rawXcodebuildOut, rawXcodebuildOutputLogPath, bitriseXcodeRawResultTextEnvKey); err != nil {
		log.Warnf("Failed to export %s, error: %s", bitriseXcodeRawResultTextEnvKey, err)
	} else {
		log.Warnf(`You can find the last couple of lines of Xcode's build log above, but the full log is also available in the %s
The log file is stored in $BITRISE_DEPLOY_DIR, and its full path is available in the $%s environment variable
(value: %s)`, filepath.Base(rawXcodebuildOutputLogPath), bitriseXcodeRawResultTextEnvKey, rawXcodebuildOutputLogPath)
	}

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

	args := []string{"xcodebuild", "-showBuildSettings"}
	{
		if xcworkspace.IsWorkspace(absProjectPath) {
			args = append(args, "-workspace", absProjectPath)
		} else {
			args = append(args, "-project", absProjectPath)
		}

		args = append(args, "-scheme", cfg.Scheme)
		if cfg.Configuration != "" {
			args = append(args, "-configuration", cfg.Configuration)
		}

		args = append(args, "build-for-testing")
		args = append(args, customOptions...)
	}

	cmd := factory.Create(args[0], args[1:], nil)
	fmt.Println()
	log.Donef("$ %s", cmd.PrintableCommandArgs())
	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		failf("%s failed, error: %s", cmd.PrintableCommandArgs(), err)
	}

	buildSettings, err := parseShowBuildSettingsOutput(out)
	if err != nil {
		failf("Failed to parse build settings, error: %s", err)
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

func parseShowBuildSettingsOutput(out string) (serialized.Object, error) {
	settings := serialized.Object{}

	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		split := strings.Split(line, " = ")

		if len(split) < 2 {
			continue
		}

		key := strings.TrimSpace(split[0])
		value := strings.TrimSpace(strings.Join(split[1:], " = "))

		settings[key] = value
	}

	return settings, nil
}
