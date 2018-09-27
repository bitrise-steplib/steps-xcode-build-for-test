package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-io/go-utils/stringutil"
	"github.com/bitrise-io/steps-xcode-archive/utils"
	"github.com/bitrise-tools/go-steputils/stepconf"
	"github.com/bitrise-tools/go-xcode/xcodebuild"
	"github.com/bitrise-tools/go-xcode/xcpretty"
	"github.com/bitrise-tools/xcode-project/serialized"
	"github.com/bitrise-tools/xcode-project/xcodeproj"
	"github.com/bitrise-tools/xcode-project/xcscheme"
	"github.com/bitrise-tools/xcode-project/xcworkspace"
	shellquote "github.com/kballard/go-shellquote"
)

const bitriseXcodeRawResultTextEnvKey = "BITRISE_XCODE_RAW_RESULT_TEXT_PATH"

// Config ...
type Config struct {
	ProjectPath       string `env:"project_path,required"`
	Scheme            string `env:"scheme,required"`
	Configuration     string `env:"configuration,required"`
	XcodebuildOptions string `env:"xcodebuild_options"`
	OutputDir         string `env:"output_dir,required"`
	IsCleanBuild      bool   `env:"is_clean_build,opt[yes,no]"`
	OutputTool        string `env:"output_tool,opt[xcpretty,xcodebuild]"`
	VerboseLog        bool   `env:"verbose_log,required"`
}

func failf(format string, v ...interface{}) {
	log.Errorf(format, v...)
	os.Exit(1)
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

	// ABS out dir pth
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
			log.Printf(" Switching output tool to xcodebuild")
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
	xcodeBuildCmd.SetCustomOptions(customOptions)

	// set clean build
	if cfg.IsCleanBuild {
		xcodeBuildCmd.SetCustomBuildAction("clean")
	}

	if cfg.OutputTool == "xcpretty" {
		xcprettyCmd := xcpretty.New(xcodeBuildCmd)

		log.Donef(" $ %s", xcprettyCmd.PrintableCmd())
		fmt.Println()

		if rawXcodebuildOut, err := xcprettyCmd.Run(); err != nil {
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

		if err := buildRootCmd.Run(); err != nil {
			failf("Build failed, error: %s", err)
		}
	}

	fmt.Println()

	//
	// Export

	log.Infof("Export:")

	var buildSettings serialized.Object
	if xcworkspace.IsWorkspace(cfg.ProjectPath) {
		workspace, err := xcworkspace.Open(cfg.ProjectPath)
		if err != nil {
			failf("Failed to open xcworkspace (%s), error: %s", cfg.ProjectPath, err)
		}

		buildSettings, err = workspace.SchemeBuildSettings(cfg.Scheme, cfg.Configuration, customOptions...)
		if err != nil {
			failf("failed to parse workspace (%s) build settings, error: %s", cfg.ProjectPath, err)
		}
	} else {
		project, err := xcodeproj.Open(cfg.ProjectPath)
		if err != nil {
			failf("")
		}

		target, err := findUITestTarget(project, cfg.Scheme)
		if err != nil {
			failf("")
		}

		fmt.Println(target)

		buildSettings, err = project.TargetBuildSettings(target.Name, cfg.Configuration, customOptions...)
		if err != nil {
			failf("failed to parse project (%s) build settings, error: %s", cfg.ProjectPath, err)
		}
	}

	symRoot, err := buildSettings.String("SYMROOT")
	if err != nil {
		failf("")
	}
	projectName, err := buildSettings.String("PROJECT_NAME")
	if err != nil {
		failf("")
	}
	sdkVersion, err := buildSettings.String("SDK_VERSION")
	if err != nil {
		failf("")
	}

	zipCmd := command.New("zip", "-r", "/tmp/testarchive.zip", fmt.Sprintf("%s-iphoneos", cfg.Configuration), fmt.Sprintf("%s_iphoneos%s-arm64.xctestrun", projectName, sdkVersion))
	zipCmd.SetDir(symRoot)
	zipCmd.Run()

	log.Donef(" $ %s", zipCmd.PrintableCommandArgs())
}

func findUITestTarget(proj xcodeproj.XcodeProj, scheme string) (xcodeproj.Target, error) {
	sch, ok := proj.Scheme(scheme)
	if !ok {
		return xcodeproj.Target{}, fmt.Errorf("failed to find scheme (%s) in project", scheme)
	}

	for _, testable := range sch.TestAction.Testables {
		if testable.Skipped == "NO" {
			for _, t := range proj.Proj.Targets {
				if t.ID == testable.BuildableReference.BlueprintIdentifier && t.ProductType == "com.apple.product-type.bundle.ui-testing" {
					return t, nil
				}
			}
		}
	}

	return xcodeproj.Target{}, fmt.Errorf("failed to find UITest target for scheme (%s)", scheme)
}

// buildTargetDirForScheme returns the TARGET_BUILD_DIR for the provided scheme
func buildTargetDirForScheme(proj xcodeproj.XcodeProj, scheme, configuration string, customOptions ...string) (string, error) {
	// Fetch project's main target from .xcodeproject
	var buildSettings serialized.Object

	mainTarget, err := findUITestTarget(proj, scheme)
	if err != nil {
		return "", fmt.Errorf("failed to fetch project's targets, error: %s", err)
	}

	buildSettings, err = proj.TargetBuildSettings(mainTarget.Name, configuration, customOptions...)
	if err != nil {
		return "", fmt.Errorf("failed to parse project (%s) build settings, error: %s", proj.Path, err)
	}

	schemeBuildDir, err := buildSettings.String("TARGET_BUILD_DIR")
	if err != nil {
		return "", fmt.Errorf("failed to parse build settings, error: %s", err)
	}

	return schemeBuildDir, nil
}

func openProject(pth, schemeName, configurationName string) (xcodeproj.XcodeProj, xcscheme.Scheme, error) {
	var scheme xcscheme.Scheme
	var schemeContainerDir string

	if xcodeproj.IsXcodeProj(pth) {
		project, err := xcodeproj.Open(pth)
		if err != nil {
			return xcodeproj.XcodeProj{}, xcscheme.Scheme{}, err
		}

		var ok bool
		scheme, ok = project.Scheme(schemeName)
		if !ok {
			return xcodeproj.XcodeProj{}, xcscheme.Scheme{}, fmt.Errorf("no scheme found with name: %s in project: %s", schemeName, pth)
		}
		schemeContainerDir = filepath.Dir(pth)
	} else if xcworkspace.IsWorkspace(pth) {
		workspace, err := xcworkspace.Open(pth)
		if err != nil {
			return xcodeproj.XcodeProj{}, xcscheme.Scheme{}, err
		}

		var ok bool
		var containerProject string
		scheme, containerProject, ok = workspace.Scheme(schemeName)
		if !ok {
			return xcodeproj.XcodeProj{}, xcscheme.Scheme{}, fmt.Errorf("no scheme found with name: %s in workspace: %s", schemeName, pth)
		}
		schemeContainerDir = filepath.Dir(containerProject)
	} else {
		return xcodeproj.XcodeProj{}, xcscheme.Scheme{}, fmt.Errorf("unknown project extension: %s", filepath.Ext(pth))
	}

	if configurationName == "" {
		configurationName = scheme.ArchiveAction.BuildConfiguration
	}

	if configurationName == "" {
		return xcodeproj.XcodeProj{}, xcscheme.Scheme{}, fmt.Errorf("no configuration provided nor default defined for the scheme's (%s) archive action", schemeName)
	}

	var archiveEntry xcscheme.BuildActionEntry
	for _, entry := range scheme.BuildAction.BuildActionEntries {
		if entry.BuildForArchiving != "YES" {
			continue
		}
		archiveEntry = entry
		break
	}

	if archiveEntry.BuildableReference.BlueprintIdentifier == "" {
		return xcodeproj.XcodeProj{}, xcscheme.Scheme{}, fmt.Errorf("archivable entry not found")
	}

	projectPth, err := archiveEntry.BuildableReference.ReferencedContainerAbsPath(schemeContainerDir)
	if err != nil {
		return xcodeproj.XcodeProj{}, xcscheme.Scheme{}, err
	}

	project, err := xcodeproj.Open(projectPth)
	if err != nil {
		return xcodeproj.XcodeProj{}, xcscheme.Scheme{}, err
	}

	return project, scheme, nil
}
