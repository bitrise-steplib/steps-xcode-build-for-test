package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/errorutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-io/go-utils/stringutil"
	"github.com/bitrise-io/steps-xcode-archive/utils"
	"github.com/bitrise-tools/go-steputils/output"
	"github.com/bitrise-tools/go-steputils/stepconf"
	"github.com/bitrise-tools/go-steputils/tools"
	"github.com/bitrise-tools/go-xcode/xcodebuild"
	"github.com/bitrise-tools/go-xcode/xcpretty"
	"github.com/bitrise-tools/xcode-project/serialized"
	"github.com/bitrise-tools/xcode-project/xcworkspace"
	shellquote "github.com/kballard/go-shellquote"
)

const bitriseXcodeRawResultTextEnvKey = "BITRISE_XCODE_RAW_RESULT_TEXT_PATH"

// Config ...
type Config struct {
	ProjectPath   string `env:"project_path,required"`
	Scheme        string `env:"scheme,required"`
	Configuration string `env:"configuration"`
	Destination   string `env:"destination,required"`

	XcodebuildOptions string `env:"xcodebuild_options"`
	OutputDir         string `env:"output_dir,required"`
	OutputTool        string `env:"output_tool,opt[xcpretty,xcodebuild]"`
	VerboseLog        bool   `env:"verbose_log,required"`
}

func main() {
	//
	// Config
	var cfg Config
	cfg.OutputTool = "xcpretty"
	if err := stepconf.Parse(&cfg); err != nil {
		failf("Issue with input: %s", err)
	}
	log.SetEnableDebugLog(cfg.VerboseLog)

	stepconf.Print(cfg)
	fmt.Println()

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
	if cfg.OutputTool == "xcpretty" {
		log.Infof("Output tool check:")

		// check if already installed
		if installed, err := xcpretty.IsInstalled(); err != nil {
			log.Warnf(" Failed to check if xcpretty is installed, error: %s", err)
			cfg.OutputTool = "xcodebuild"
		} else if !installed {
			log.Warnf(` xcpretty is not installed`)
			log.Printf(" Installing...")

			// install if not installed
			if cmds, err := xcpretty.Install(); err != nil {
				log.Warnf(" Failed to install xcpretty, error: %s", err)
				cfg.OutputTool = "xcodebuild"
			} else {
				for _, cmd := range cmds {
					log.Donef(" $ %s", cmd.PrintableCommandArgs())
					if err := cmd.Run(); err != nil {
						log.Warnf(" Failed to install xcpretty, error: %s", err)
						cfg.OutputTool = "xcodebuild"
						break
					}
				}
			}
		} else {
			// already installed
			log.Donef(` xcpretty is installed`)
		}
		// warn user if we needed to switch back from xcpretty
		if cfg.OutputTool != "xcpretty" {
			log.Warnf(" Switching output tool to xcodebuild")
		}
		fmt.Println()
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

	xcodeBuildCmd := xcodebuild.NewCommandBuilder(cfg.ProjectPath, xcworkspace.IsWorkspace(cfg.ProjectPath), "")
	xcodeBuildCmd.SetScheme(cfg.Scheme)
	xcodeBuildCmd.SetConfiguration(cfg.Configuration)
	xcodeBuildCmd.SetCustomBuildAction("build-for-testing")
	xcodeBuildCmd.SetDestination(cfg.Destination)
	xcodeBuildCmd.SetCustomOptions(customOptions)

	// save the build time frame to find the build generated artifacts
	var buildStartTime time.Time
	var buildEndTime time.Time

	if cfg.OutputTool == "xcpretty" {
		xcprettyCmd := xcpretty.New(xcodeBuildCmd)

		log.Donef(" $ %s", xcprettyCmd.PrintableCmd())
		fmt.Println()

		buildStartTime = time.Now()
		rawXcodebuildOut, err := xcprettyCmd.Run()
		buildEndTime = time.Now()

		if err != nil {
			log.Errorf("\nLast lines of the Xcode's build log:")
			fmt.Println(stringutil.LastNLines(rawXcodebuildOut, 10))

			if err := utils.ExportOutputFileContent(rawXcodebuildOut, rawXcodebuildOutputLogPath, bitriseXcodeRawResultTextEnvKey); err != nil {
				log.Warnf("Failed to export %s, error: %s", bitriseXcodeRawResultTextEnvKey, err)
			} else {
				log.Warnf(`You can find the last couple of lines of Xcode's build log above, but the full log is also available in the %s
The log file is stored in $BITRISE_DEPLOY_DIR, and its full path is available in the $%s environment variable
(value: %s)`, filepath.Base(rawXcodebuildOutputLogPath), bitriseXcodeRawResultTextEnvKey, rawXcodebuildOutputLogPath)
			}

			failf("Build failed, error: %s", err)
		}
	} else {
		log.Donef(" $ %s", xcodeBuildCmd.PrintableCmd())
		fmt.Println()

		buildRootCmd := xcodeBuildCmd.Command()
		buildRootCmd.SetStdout(os.Stdout)
		buildRootCmd.SetStderr(os.Stderr)

		buildStartTime = time.Now()
		err := buildRootCmd.Run()
		buildEndTime = time.Now()

		if err != nil {
			failf("Build failed, error: %s", err)
		}
	}

	fmt.Println()

	//
	// Export
	log.Infof("Export:")

	args := []string{"xcodebuild", "-showBuildSettings"}
	{
		if xcworkspace.IsWorkspace(cfg.ProjectPath) {
			args = append(args, "-workspace", cfg.ProjectPath)
		} else {
			args = append(args, "-project", cfg.ProjectPath)
		}

		args = append(args, "-scheme", cfg.Scheme)
		if cfg.Configuration != "" {
			args = append(args, "-configuration", cfg.Configuration)
		}

		args = append(args, "build-for-testing")
		args = append(args, customOptions...)
	}

	cmd := command.New(args[0], args[1:]...)
	fmt.Println()
	log.Donef("$ %s", cmd.PrintableCommandArgs())
	fmt.Println()
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

	configuration, err := buildSettings.String("CONFIGURATION")
	if err != nil {
		failf("Failed to parse CONFIGURATION build setting: %s", err)
	}

	// Without better solution the step collects every xctestrun files and filters them for the build time frame
	xctestrunPthPattern := filepath.Join(symRoot, fmt.Sprintf("%s*.xctestrun", cfg.Scheme))
	xctestrunPths, err := filepath.Glob(xctestrunPthPattern)
	if err != nil {
		failf("Failed to search for xctestrun file using pattern: %s, error: %s", xctestrunPthPattern, err)
	}

	if len(xctestrunPths) == 0 {
		failf("No xctestrun file found with pattern: %s", xctestrunPthPattern)
	}

	var buildXCTestrunPths []string
	for _, xctestrunPth := range xctestrunPths {
		info, err := os.Stat(xctestrunPth)
		if err != nil {
			failf("Failed to check %s modtime: %s", xctestrunPth, err)
		}

		if !info.ModTime().Before(buildStartTime) && !info.ModTime().After(buildEndTime) {
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
	zipCmd := command.New("zip", "-r", outputTestBundleZipPath, filepath.Base(builtTestDir), filepath.Base(xctestrunPth)).SetDir(symRoot)
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
