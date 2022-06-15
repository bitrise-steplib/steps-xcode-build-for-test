package step

import (
	"os"

	"github.com/bitrise-io/go-utils/colorstring"
	"github.com/bitrise-io/go-utils/stringutil"
	"github.com/bitrise-io/go-utils/v2/log"
)

type DirReader interface {
	ReadDir(name string) ([]os.DirEntry, error)
}

type dirReader struct {
}

func NewDirReader() DirReader {
	return dirReader{}
}

func (r dirReader) ReadDir(name string) ([]os.DirEntry, error) {
	return os.ReadDir(name)
}

func printLastLinesOfXcodebuildTestLog(rawXcodebuildOutput string, isRunSuccess bool, logger log.Logger) {
	const lastLines = "\nLast lines of the build log:"
	if !isRunSuccess {
		logger.Errorf(lastLines)
	} else {
		logger.Infof(lastLines)
	}

	logger.Printf(stringutil.LastNLines(rawXcodebuildOutput, 20) + "\n")

	if !isRunSuccess {
		logger.Warnf("If you can't find the reason of the error in the log, please check the %s.", xcodebuildLogBaseName)
	}

	logger.Printf(colorstring.Magentaf(`
The log file is stored in the output directory, and its full path
is available in the $%s environment variable.
Use Deploy to Bitrise.io step (after this step),
to attach the file to your build as an artifact!`, xcodebuildLogPathEnvKey) + "\n")
}
