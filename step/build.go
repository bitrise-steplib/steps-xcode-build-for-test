package step

import (
	"fmt"
	"os"
	"strings"

	"github.com/bitrise-io/go-utils/colorstring"
	"github.com/bitrise-io/go-utils/stringutil"
	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-io/go-xcode/v2/xcodecommand"
	"github.com/bitrise-io/go-xcode/xcodebuild"
	cache "github.com/bitrise-io/go-xcode/xcodecache"
)

func runCommandWithRetry(xcodeCommandRunner xcodecommand.Runner, logFormatter string, cmd *xcodebuild.CommandBuilder, swiftPackagesPath string, logger log.Logger) (string, error) {
	output, err := runCommand(xcodeCommandRunner, logFormatter, cmd, logger)
	if err != nil && swiftPackagesPath != "" && strings.Contains(output, cache.SwiftPackagesStateInvalid) {
		logger.Warnf("Build failed, swift packages cache is in an invalid state, error: %s", err)
		if err := os.RemoveAll(swiftPackagesPath); err != nil {
			return output, fmt.Errorf("failed to remove invalid Swift package caches, error: %s", err)
		}
		return runCommand(xcodeCommandRunner, logFormatter, cmd, logger)
	}
	return output, err
}

func runCommand(xcodeCommandRunner xcodecommand.Runner, logFormatter string, cmd *xcodebuild.CommandBuilder, logger log.Logger) (string, error) {
	output, err := xcodeCommandRunner.Run("", cmd.CommandArgs(), []string{})
	if logFormatter == XcodebuildTool || err != nil {
		printLastLinesOfXcodebuildLog(logger, string(output.RawOut), err == nil)
	}

	return string(output.RawOut), err
}

func printLastLinesOfXcodebuildLog(logger log.Logger, xcodebuildLog string, isXcodebuildSuccess bool) {
	const lastLinesMsg = "\nLast lines of the Xcode log:"
	if isXcodebuildSuccess {
		logger.Infof(lastLinesMsg)
	} else {
		logger.Infof(colorstring.Red(lastLinesMsg))
	}

	logger.Printf("%s", stringutil.LastNLines(xcodebuildLog, 20))
	logger.Println()

	if !isXcodebuildSuccess {
		logger.Warnf("If you can't find the reason of the error in the log, please check the artifact %s.", xcodebuildLogBaseName)
	}

	logger.Infof(colorstring.Magenta(fmt.Sprintf(`
The log file is stored in $BITRISE_DEPLOY_DIR, and its full path
is available in the $%s environment variable.

Deploy to Bitrise.io Step can attach the file to your build as an artifact.`, xcodebuildLogPathEnvKey)))
}
