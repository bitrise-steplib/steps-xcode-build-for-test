package main

import (
	"os"

	v2log "github.com/bitrise-io/go-utils/v2/log"
)

func main() {
	os.Exit(run())
}

func run() int {
	logger := v2log.NewLogger()

	s := createStep(logger)
	cfg, err := s.ProcessConfig()
	if err != nil {
		logger.Errorf("Process config: %s", err)
		return 1
	}

	if err := s.InstallDependencies(cfg.XCPretty); err != nil {
		logger.Warnf("Install dependencies: %s", err)
		logger.Printf("Switching to xcodebuild for output tool")
		cfg.XCPretty = false
	}

	runOut, runErr := s.Run(cfg)
	exportErr := s.ExportOutput(ExportOpts{
		OutputDir: cfg.OutputDir,
		RunOut:    runOut,
	})

	if runErr != nil {
		logger.Errorf("Run: %s", runErr)
	}
	if exportErr != nil {
		logger.Errorf("Export outputs: %s", exportErr)
	}
	if runErr != nil || exportErr != nil {
		return 1
	}

	return 0
}

func createStep(logger v2log.Logger) TestBuilder {
	return NewTestBuilder(logger)
}
