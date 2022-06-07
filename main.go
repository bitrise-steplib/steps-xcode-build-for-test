package main

import (
	"os"

	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-steplib/steps-xcode-build-for-test/step"
)

func main() {
	os.Exit(run())
}

func run() int {
	exitCode := 0

	logger := log.NewLogger()
	xcodebuildBuild := createXcodebuildBuild(logger)

	cfg, err := xcodebuildBuild.ProcessConfig()
	if err != nil {
		logger.Errorf("Process config: %s", err)

		exitCode = 1
		return exitCode
	}

	if err := xcodebuildBuild.InstallDependencies(cfg.XCPretty); err != nil {
		logger.Warnf("Install dependencies: %s", err)
		logger.Printf("Switching to xcodebuild for output tool")
		cfg.XCPretty = false
	}

	result, err := xcodebuildBuild.Run(cfg)
	if err != nil {
		logger.Errorf("Run: %s", err)
		exitCode = 1
	}

	if err := xcodebuildBuild.ExportOutputs(step.ExportOpts{
		OutputDir: cfg.OutputDir,
		RunOut:    result,
	}); err != nil {
		logger.Errorf("Export outputs: %s", err)
		exitCode = 1
	}

	return exitCode
}

func createXcodebuildBuild(logger log.Logger) step.XcodebuildBuild {
	return step.NewXcodebuildBuild(logger)
}
