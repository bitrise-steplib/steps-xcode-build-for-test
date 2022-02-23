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
		log.Warnf("Failed to install deps: %s", err)
		log.Printf("Switching to xcodebuild for output tool")
		cfg.XCPretty = false
	}

	runOpts := RunOpts{
		XCPretty:          cfg.XCPretty,
		CodesignManager:   cfg.CodesignManager,
		SwiftPackagesPath: cfg.SwiftPackagesPath,
		OutputDir:         cfg.OutputDir,
		ProjectPath:       cfg.ProjectPath,
		Scheme:            cfg.Scheme,
		Configuration:     cfg.Configuration,
		Destination:       cfg.Destination,
		XCConfigContent:   cfg.XCConfigContent,
		XcodebuildOptions: cfg.XcodebuildOptions,
	}
	runOut, runErr := s.Run(runOpts)

	exportOpts := ExportOpts{
		OutputDir:         cfg.OutputDir,
		ProjectPath:       cfg.ProjectPath,
		Scheme:            cfg.Scheme,
		Configuration:     cfg.Configuration,
		XcodebuildOptions: cfg.XcodebuildOptions,
		CacheLevel:        cfg.CacheLevel,
		BuildInterval:     runOut.BuildInterval,
		XcodebuildTestLog: runOut.XcodebuildLog,
	}
	exportErr := s.ExportOutput(exportOpts)

	if runErr != nil {
		log.Errorf(runErr.Error())
	}
	if exportErr != nil {
		log.Errorf(exportErr.Error())
	}
	if runErr != nil || exportErr != nil {
		return 1
	}

	return 0
}

func createStep() TestBuilder {
	return NewTestBuilder()
}
