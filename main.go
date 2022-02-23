package main

import (
	"os"

	"github.com/bitrise-io/go-utils/log"
)

func main() {
	os.Exit(run())
}

func run() int {
	s := createStep()
	cfg, err := s.ProcessConfig()
	if err != nil {
		log.Errorf("Process config: %s", err)
		return 1
	}

	if err := s.InstallDependencies(cfg.XCPretty); err != nil {
		log.Warnf("Install dependencies: %s", err)
		log.Printf("Switching to xcodebuild for output tool")
		cfg.XCPretty = false
	}

	runOut, runErr := s.Run(cfg)

	exportOpts := ExportOpts{
		OutputDir:         cfg.OutputDir,
		XcodebuildTestLog: runOut.XcodebuildLog,
		BuiltTestDir:      runOut.BuiltTestDir,
		XctestrunPth:      runOut.XctestrunPth,
		SYMRoot:           runOut.SYMRoot,
	}
	exportErr := s.ExportOutput(exportOpts)

	if runErr != nil {
		log.Errorf("Run: %s", runErr)
	}
	if exportErr != nil {
		log.Errorf("Export outputs: %s", exportErr)
	}
	if runErr != nil || exportErr != nil {
		return 1
	}

	return 0
}

func createStep() TestBuilder {
	return NewTestBuilder()
}
