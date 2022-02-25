package main

import (
	"fmt"

	"github.com/bitrise-io/go-utils/colorstring"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/stringutil"
)

func printLastLinesOfXcodebuildTestLog(rawXcodebuildOutput string, isRunSuccess bool) {
	const lastLines = "\nLast lines of the build log:"
	if !isRunSuccess {
		log.Errorf(lastLines)
	} else {
		log.Infof(lastLines)
	}

	fmt.Println(stringutil.LastNLines(rawXcodebuildOutput, 20))

	if !isRunSuccess {
		log.Warnf("If you can't find the reason of the error in the log, please check the %s.", xcodebuildLogBaseName)
	}

	fmt.Println(colorstring.Magentaf(`
The log file is stored in the output directory, and its full path
is available in the $%s environment variable.
Use Deploy to Bitrise.io step (after this step),
to attach the file to your build as an artifact!`, xcodebuildLogPathEnvKey))
}
