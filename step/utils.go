package step

import (
	"io/fs"
	"os"
	"strings"

	"github.com/bitrise-io/go-utils/colorstring"
	"github.com/bitrise-io/go-utils/stringutil"
	"github.com/bitrise-io/go-utils/v2/log"
)

type FileManager interface {
	ReadFile(pth string) ([]byte, error)
	WriteFile(filename string, data []byte, perm fs.FileMode) error
	ReadDir(name string) ([]os.DirEntry, error)
}

type fileManager struct {
}

func NewFileManager() FileManager {
	return fileManager{}
}

func (m fileManager) ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

func (m fileManager) WriteFile(filename string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(filename, data, perm)
}

func (m fileManager) ReadDir(name string) ([]os.DirEntry, error) {
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

func findBuildSetting(options []string, key string) string {
	for _, option := range options {
		split := strings.Split(option, "=")
		if len(split) < 2 {
			continue
		}
		k := split[0]
		v := strings.Join(split[1:], "=")

		if k == key {
			return v
		}
	}

	return ""
}
