package step

import (
	"path/filepath"
	"testing"

	"github.com/bitrise-io/go-xcode/xcodeproject/serialized"
	"github.com/bitrise-steplib/steps-xcode-build-for-test/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_GivenIosProjectWithTestPlan_WhenFindTestBundle_ThenReturnsTestBundle(t *testing.T) {
	// Given
	step, stepMocks := createStepAndMocks()

	project := "BullsEye.xcworkspace"
	scheme := "BullsEye"
	configuration := "Debug"
	symroot := "$HOME/Library/Developer/Xcode/DerivedData/BullsEye-exnjhblzvmjcydaiwoxkklkizqxc/Build/Products"
	var options []string

	stepMocks.xcodebuild.
		On("ShowBuildSettings", project, scheme, configuration, "build-for-testing", options).
		Return(serialized.Object{
			"SYMROOT":       symroot,
			"CONFIGURATION": configuration,
		}, nil)

	pattern := xctestrunPathPattern(symroot, scheme)
	xctestrunPth := filepath.Join(symroot, "BullsEye_FullTests_iphonesimulator15.5-arm64-x86_64.xctestrun")
	stepMocks.filepathGlobber.On("Glob", pattern).Return([]string{xctestrunPth}, nil)

	stepMocks.modtimeChecker.On("ModifiedInTimeFrame", xctestrunPth, mock.Anything, mock.Anything).Return(true, nil)

	builtTestDir := builtTestDirPath(xctestrunPth, symroot, scheme, configuration)
	stepMocks.pathChecker.On("IsPathExists", builtTestDir).Return(true, nil)

	stepMocks.logger.On("Printf", mock.Anything, mock.Anything).Return()

	// When
	bundle, err := step.findTestBundle(findTestBundleOpts{
		ProjectPath:       project,
		Scheme:            scheme,
		Configuration:     configuration,
		XcodebuildOptions: nil,
		BuildInterval:     timeInterval{},
	})

	// Then
	require.NoError(t, err)
	require.Equal(t, builtTestDir, filepath.Join(symroot, "Debug-iphonesimulator"))
	require.Equal(t, xctestrunPth, xctestrunPth)
	require.Equal(t, symroot, bundle.SYMRoot)
}

type testingMocks struct {
	logger          *mocks.Logger
	xcodebuild      *mocks.Xcodebuild
	modtimeChecker  *mocks.ModtimeChecker
	pathChecker     *mocks.PathChecker
	filepathGlobber *mocks.FilepathGlober
}

func createStepAndMocks() (XcodebuildBuilder, testingMocks) {
	logger := new(mocks.Logger)
	xcodebuild := new(mocks.Xcodebuild)
	modtimeChecker := new(mocks.ModtimeChecker)
	pathChecker := new(mocks.PathChecker)
	filepathGlobber := new(mocks.FilepathGlober)

	step := NewXcodebuildBuilder(logger, xcodebuild, modtimeChecker, pathChecker, filepathGlobber)

	mocks := testingMocks{
		logger:          logger,
		xcodebuild:      xcodebuild,
		modtimeChecker:  modtimeChecker,
		pathChecker:     pathChecker,
		filepathGlobber: filepathGlobber,
	}

	return step, mocks
}
