package step

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bitrise-io/go-utils/colorstring"
	"github.com/bitrise-io/go-utils/stringutil"
	"github.com/bitrise-io/go-utils/v2/log"
)

type ModtimeChecker interface {
	ModifiedInTimeFrame(pth string, start, end time.Time) (bool, error)
}

type modtimeChecker struct {
	logger log.Logger
}

func NewModtimeChecker(logger log.Logger) ModtimeChecker {
	return modtimeChecker{
		logger: logger,
	}
}

func (c modtimeChecker) ModifiedInTimeFrame(pth string, start, end time.Time) (bool, error) {
	info, err := os.Stat(pth)
	if err != nil {
		return false, fmt.Errorf("failed to check %s modtime: %w", pth, err)
	}
	if !info.ModTime().Before(start) && !info.ModTime().After(end) {
		return true, nil
	}

	c.logger.Printf("xctestrun: %s was created at %s, which is outside of the window %s - %s ", pth, info.ModTime(), start, end)
	return false, nil
}

type FilepathGlober interface {
	Glob(pattern string) (matches []string, err error)
}

type filepathGlober struct {
}

func NewFilepathGlober() FilepathGlober {
	return filepathGlober{}
}

func (g filepathGlober) Glob(pattern string) (matches []string, err error) {
	return filepath.Glob(pattern)
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
