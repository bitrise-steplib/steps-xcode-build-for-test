package step

import (
	"path/filepath"
	"testing"

	"github.com/bitrise-io/go-xcode/xcodeproject/serialized"
	"github.com/bitrise-io/go-xcode/xcodeproject/xcscheme"
	"github.com/bitrise-steplib/steps-xcode-build-for-test/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_GivenIosProjectProducesOneXctestrun_WhenFindTestBundle_ThenReturnsTestBundle(t *testing.T) {
	// Given
	step, stepMocks := createStepAndMocks()

	project := "BullsEye.xcworkspace"
	scheme := "BullsEye"
	configuration := "Debug"
	symroot := "$HOME/Library/Developer/Xcode/DerivedData/BullsEye-exnjhblzvmjcydaiwoxkklkizqxc/Build/Products"
	var options []string
	xctestrunPths := []string{filepath.Join(symroot, "BullsEye_FullTests_iphonesimulator15.5-arm64.xctestrun")}

	stepMocks.filepathGlobber.On("Glob", mock.Anything).Return(xctestrunPths, nil)
	stepMocks.modtimeChecker.On("ModifiedInTimeFrame", mock.Anything, mock.Anything, mock.Anything).Return(true, nil)
	stepMocks.pathChecker.On("IsPathExists", mock.Anything).Return(true, nil)
	stepMocks.logger.On("Printf", mock.Anything, mock.Anything).Return()
	stepMocks.logger.On("Donef", mock.Anything, mock.Anything).Return()
	stepMocks.xcodebuild.
		On("ShowBuildSettings", project, scheme, configuration, "build-for-testing", options).
		Return(serialized.Object{
			"SYMROOT":       symroot,
			"CONFIGURATION": configuration,
		}, nil)

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
	require.Equal(t, bundle.BuiltTestDir, filepath.Join(symroot, "Debug-iphonesimulator"))
	require.Equal(t, bundle.DefaultXctestrunPth, filepath.Join(symroot, "BullsEye_FullTests_iphonesimulator15.5-arm64.xctestrun"))
	require.Equal(t, bundle.XctestrunPths, []string{filepath.Join(symroot, "BullsEye_FullTests_iphonesimulator15.5-arm64.xctestrun")})
	require.Equal(t, symroot, bundle.SYMRoot)
}

func Test_GivenIosProjectProducesMultipleXctestrun_WhenFindTestBundle_ThenReturnsTestBundle(t *testing.T) {
	// Given
	step, stepMocks := createStepAndMocks()

	project := "BullsEye.xcworkspace"
	scheme := "BullsEye"
	configuration := "Debug"
	symroot := "$HOME/Library/Developer/Xcode/DerivedData/BullsEye-exnjhblzvmjcydaiwoxkklkizqxc/Build/Products"
	var options []string
	xctestrunPths := []string{
		filepath.Join(symroot, "BullsEye_UnitTests_iphonesimulator15.5-arm64.xctestrun"),
		filepath.Join(symroot, "BullsEye_UITests_iphonesimulator15.5-arm64.xctestrun"),
		filepath.Join(symroot, "BullsEye_FullTests_iphonesimulator15.5-arm64.xctestrun"),
	}

	stepMocks.filepathGlobber.On("Glob", mock.Anything).Return(xctestrunPths, nil)
	stepMocks.modtimeChecker.On("ModifiedInTimeFrame", mock.Anything, mock.Anything, mock.Anything).Return(true, nil)
	stepMocks.pathChecker.On("IsPathExists", mock.Anything).Return(true, nil)
	stepMocks.logger.On("Printf", mock.Anything, mock.Anything).Return()
	stepMocks.logger.On("Donef", mock.Anything, mock.Anything).Return()
	stepMocks.logger.On("Donef", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	stepMocks.xcodebuild.
		On("ShowBuildSettings", project, scheme, configuration, "build-for-testing", options).
		Return(serialized.Object{
			"SYMROOT":       symroot,
			"CONFIGURATION": configuration,
		}, nil)
	stepMocks.xcodeproject.On("Scheme", project, scheme).Return(&xcscheme.Scheme{
		TestAction: xcscheme.TestAction{
			TestPlans: &xcscheme.TestPlans{
				TestPlanReferences: []xcscheme.TestPlanReference{
					{Reference: "container:UnitTests.xctestplan"},
					{Reference: "container:UITests_.xctestplan"},
					{Reference: "container:FullTests.xctestplan", Default: "YES"},
				},
			},
		},
	}, nil)

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
	require.Equal(t, bundle.BuiltTestDir, filepath.Join(symroot, "Debug-iphonesimulator"))
	require.Equal(t, bundle.DefaultXctestrunPth, filepath.Join(symroot, "BullsEye_FullTests_iphonesimulator15.5-arm64.xctestrun"))
	require.Equal(t, bundle.XctestrunPths, []string{
		filepath.Join(symroot, "BullsEye_UnitTests_iphonesimulator15.5-arm64.xctestrun"),
		filepath.Join(symroot, "BullsEye_UITests_iphonesimulator15.5-arm64.xctestrun"),
		filepath.Join(symroot, "BullsEye_FullTests_iphonesimulator15.5-arm64.xctestrun"),
	})
	require.Equal(t, symroot, bundle.SYMRoot)
}

type testingMocks struct {
	logger          *mocks.Logger
	xcodebuild      *mocks.Xcodebuild
	xcodeproject    *mocks.XcodeProject
	modtimeChecker  *mocks.ModtimeChecker
	pathChecker     *mocks.PathChecker
	filepathGlobber *mocks.FilepathGlober
}

func createStepAndMocks() (XcodebuildBuilder, testingMocks) {
	logger := new(mocks.Logger)
	xcodebuild := new(mocks.Xcodebuild)
	xcodeproject := new(mocks.XcodeProject)
	modtimeChecker := new(mocks.ModtimeChecker)
	pathChecker := new(mocks.PathChecker)
	filepathGlobber := new(mocks.FilepathGlober)

	step := NewXcodebuildBuilder(logger, xcodebuild, xcodeproject, modtimeChecker, pathChecker, filepathGlobber)

	mocks := testingMocks{
		logger:          logger,
		xcodebuild:      xcodebuild,
		xcodeproject:    xcodeproject,
		modtimeChecker:  modtimeChecker,
		pathChecker:     pathChecker,
		filepathGlobber: filepathGlobber,
	}

	return step, mocks
}
