package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-xcode/xcodebuild"
	cache "github.com/bitrise-io/go-xcode/xcodecache"
	"github.com/bitrise-io/go-xcode/xcpretty"
)

type timeInterval struct {
	start time.Time
	end   time.Time
}

func runCommandWithRetry(cmd *xcodebuild.CommandBuilder, useXcpretty bool, swiftPackagesPath string) (string, timeInterval, error) {
	var buildInterval timeInterval

	output, buildInterval, err := runCommand(cmd, useXcpretty)
	buildInterval.end = time.Now()
	if err != nil && swiftPackagesPath != "" && strings.Contains(output, cache.SwiftPackagesStateInvalid) {
		log.Warnf("Build failed, swift packages cache is in an invalid state, error: %s", err)
		log.RWarnf("xcode-build-for-test", "swift-packages-cache-invalid", nil, "swift packages cache is in an invalid state")
		if err := os.RemoveAll(swiftPackagesPath); err != nil {
			return output, buildInterval, fmt.Errorf("failed to remove invalid Swift package caches, error: %s", err)
		}
		return runCommand(cmd, useXcpretty)
	}
	return output, buildInterval, err
}

func prepareCommand(xcodeCmd *xcodebuild.CommandBuilder, useXcpretty bool, output *bytes.Buffer) (*command.Model, *xcpretty.CommandModel) {
	if useXcpretty {
		return nil, xcpretty.New(*xcodeCmd)
	}

	buildRootCmd := xcodeCmd.Command()
	buildRootCmd.SetStdout(io.MultiWriter(os.Stdout, output))
	buildRootCmd.SetStderr(io.MultiWriter(os.Stderr, output))
	return buildRootCmd, nil
}

func runCommand(buildCmd *xcodebuild.CommandBuilder, useXcpretty bool) (string, timeInterval, error) {
	var output bytes.Buffer
	xcodebuildCmd, xcprettyCmd := prepareCommand(buildCmd, useXcpretty, &output)
	var buildInterval timeInterval

	if xcprettyCmd != nil {
		log.Donef(" $ %s", xcprettyCmd.PrintableCmd())
		fmt.Println()

		buildInterval.start = time.Now()
		output, err := xcprettyCmd.Run()
		buildInterval.end = time.Now()
		return output, buildInterval, err
	}
	log.Donef("$ %s", xcodebuildCmd.PrintableCommandArgs())
	fmt.Println()

	buildInterval.start = time.Now()
	err := xcodebuildCmd.Run()
	buildInterval.end = time.Now()
	return output.String(), buildInterval, err
}
