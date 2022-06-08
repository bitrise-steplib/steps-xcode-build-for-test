package xcodebuild

import (
	xcodebuildCmdFactory "github.com/bitrise-io/go-xcode/xcodebuild"
	"github.com/bitrise-io/go-xcode/xcodeproject/serialized"
)

type Xcodebuild interface {
	ShowBuildSettings(projectPath, scheme, configuration, action string, options []string) (serialized.Object, error)
}

type xcodebuild struct {
}

func NewXcodebuild() Xcodebuild {
	return xcodebuild{}
}

func (b xcodebuild) ShowBuildSettings(projectPath, scheme, configuration, action string, options []string) (serialized.Object, error) {
	buildSettingsCmd := xcodebuildCmdFactory.NewShowBuildSettingsCommand(projectPath)
	buildSettingsCmd.SetScheme(scheme)
	buildSettingsCmd.SetConfiguration(configuration)
	buildSettingsCmd.SetCustomOptions(append([]string{action}, options...))

	return buildSettingsCmd.RunAndReturnSettings()
}
