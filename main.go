package main

import (
	"fmt"
	"os"

	"github.com/bitrise-io/go-steputils/v2/ruby"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/errorutil"
	"github.com/bitrise-io/go-utils/v2/fileutil"
	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-io/go-utils/v2/pathutil"
	"github.com/bitrise-io/go-xcode/v2/xcodecommand"
	"github.com/bitrise-io/go-xcode/v2/xcodeversion"
	"github.com/bitrise-steplib/steps-xcode-build-for-test/step"
)

func main() {
	os.Exit(run())
}
func run() int {
	logger := log.NewLogger()
	configParser := createConfigParser(logger)
	config, err := configParser.ProcessConfig()
	if err != nil {
		logger.Errorf("%s", errorutil.FormattedError(fmt.Errorf("failed to process Step inputs: %w", err)))
		return 1
	}

	builder, err := createXcodebuildBuilder(logger, config.LogFormatter)
	if err != nil {
		logger.Errorf("%s", errorutil.FormattedError(fmt.Errorf("failed to process Step inputs: %w", err)))
		return 1
	}

	builder.EnsureDependencies()

	exitCode := 0
	result, err := builder.Run(config)
	if err != nil {
		logger.Errorf("%s", errorutil.FormattedError(fmt.Errorf("failed to execute Step main logic: %w", err)))
		exitCode = 1
		// don't return as step outputs needs to be exported even in case of failure (for example the xcodebuild logs)
	}

	exportOpts := createExportOptions(config, result)
	if err := builder.ExportOutputs(exportOpts); err != nil {
		logger.Errorf("%s", errorutil.FormattedError(fmt.Errorf("failed to export Step outputs: %w", err)))
		return 1
	}

	return exitCode
}

func createConfigParser(logger log.Logger) step.ConfigParser {
	return step.NewConfigParser(logger)
}

func createXcodebuildBuilder(logger log.Logger, logFormatter string) (step.XcodebuildBuilder, error) {
	envRepository := env.NewRepository()
	pathProvider := pathutil.NewPathProvider()
	pathChecker := pathutil.NewPathChecker()
	pathModifier := pathutil.NewPathModifier()
	fileManager := fileutil.NewFileManager()
	cmdFactory := command.NewFactory(envRepository)
	xcodeVersionReader := xcodeversion.NewXcodeVersionProvider(cmdFactory)
	xcodeCommandRunner := xcodecommand.Runner(nil)

	switch logFormatter {
	case step.XcodebuildTool:
		xcodeCommandRunner = xcodecommand.NewRawCommandRunner(logger, cmdFactory)
	case step.XcbeautifyTool:
		xcodeCommandRunner = xcodecommand.NewXcbeautifyRunner(logger, cmdFactory)
	case step.XcprettyTool:
		commandLocator := env.NewCommandLocator()
		rubyComamndFactory, err := ruby.NewCommandFactory(cmdFactory, commandLocator)
		if err != nil {
			return step.XcodebuildBuilder{}, fmt.Errorf("failed to install xcpretty: %s", err)
		}
		rubyEnv := ruby.NewEnvironment(rubyComamndFactory, commandLocator, logger)

		xcodeCommandRunner = xcodecommand.NewXcprettyCommandRunner(logger, cmdFactory, pathChecker, fileManager, rubyComamndFactory, rubyEnv)
	default:
		panic(fmt.Sprintf("Unknown log formatter: %s", logFormatter))
	}

	return step.NewXcodebuildBuilder(
		xcodeCommandRunner,
		logFormatter,
		xcodeVersionReader,
		pathProvider,
		pathChecker,
		pathModifier,
		step.NewFileManager(),
		logger,
		cmdFactory,
	), nil
}

func createExportOptions(config step.Config, result step.RunOut) step.ExportOpts {
	return step.ExportOpts{
		RunOut:           result,
		OutputDir:        config.OutputDir,
		CompressionLevel: config.CompressionLevel,
	}
}
