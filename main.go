package main

import (
	"os"

	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-io/go-utils/v2/pathutil"
	"github.com/bitrise-steplib/steps-xcode-build-for-test/step"
	"github.com/bitrise-steplib/steps-xcode-build-for-test/xcodeproject"
)

func main() {
	os.Exit(run())
}

func run() int {
	exitCode := 0

	logger := log.NewLogger()
	xcodebuildBuilder := createXcodebuildBuilder(logger)

	cfg, err := xcodebuildBuilder.ProcessConfig()
	if err != nil {
		logger.Errorf("Process config: %s", err)

		exitCode = 1
		return exitCode
	}

	if err := xcodebuildBuilder.InstallDependencies(cfg.XCPretty); err != nil {
		logger.Warnf("Install dependencies: %s", err)
		logger.Printf("Switching to xcodebuild for output tool")
		cfg.XCPretty = false
	}

	result, err := xcodebuildBuilder.Run(cfg)
	if err != nil {
		logger.Errorf("Run: %s", err)
		exitCode = 1
	}

	if err := xcodebuildBuilder.ExportOutputs(step.ExportOpts{
		OutputDir:        cfg.OutputDir,
		CompressionLevel: cfg.CompressionLevel,
		RunOut:           result,
	}); err != nil {
		logger.Errorf("Export outputs: %s", err)
		exitCode = 1
	}

	return exitCode
}

func createXcodebuildBuilder(logger log.Logger) step.XcodebuildBuilder {
	xcproject := xcodeproject.NewXcodeProject()
	pathChecker := pathutil.NewPathChecker()
	pathProvider := pathutil.NewPathProvider()
	fileManager := step.NewFileManager()

	return step.NewXcodebuildBuilder(logger, xcproject, pathChecker, pathProvider, fileManager)
}
