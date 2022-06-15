package step

import (
	"os"
	"path/filepath"
	"testing"

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
	symRoot := "$HOME/Library/Developer/Xcode/DerivedData/BullsEye-exnjhblzvmjcydaiwoxkklkizqxc/Build/Products"

	stepMocks.logger.On("Printf", mock.Anything, mock.Anything).Return()
	stepMocks.logger.On("Donef", mock.Anything, mock.Anything).Return()
	stepMocks.pathChecker.On("IsPathExists", mock.Anything).Return(true, nil)
	stepMocks.dirReader.On("ReadDir", mock.Anything).Return([]os.DirEntry{
		createDirEntry(filepath.Join(symRoot, "BullsEye_FullTests_iphonesimulator15.5-arm64.xctestrun")),
	}, nil)

	// When
	bundle, err := step.findTestBundle(findTestBundleOpts{
		SYMRoot:     symRoot,
		ProjectPath: project,
		Scheme:      scheme,
	})

	// Then
	require.NoError(t, err)
	require.Equal(t, filepath.Join(symRoot, "BullsEye_FullTests_iphonesimulator15.5-arm64.xctestrun"), bundle.DefaultXctestrunPth)
	require.Equal(t, []string{filepath.Join(symRoot, "BullsEye_FullTests_iphonesimulator15.5-arm64.xctestrun")}, bundle.XctestrunPths)
	require.Equal(t, symRoot, bundle.SYMRoot)
}

func Test_GivenIosProjectProducesMultipleXctestrun_WhenFindTestBundle_ThenReturnsTestBundle(t *testing.T) {
	// Given
	step, stepMocks := createStepAndMocks()

	project := "BullsEye.xcworkspace"
	scheme := "BullsEye"
	symRoot := "$HOME/Library/Developer/Xcode/DerivedData/BullsEye-exnjhblzvmjcydaiwoxkklkizqxc/Build/Products"

	stepMocks.logger.On("Printf", mock.Anything, mock.Anything).Return()
	stepMocks.logger.On("Donef", mock.Anything, mock.Anything).Return()
	stepMocks.logger.On("Donef", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	stepMocks.pathChecker.On("IsPathExists", mock.Anything).Return(true, nil)
	stepMocks.dirReader.On("ReadDir", mock.Anything).Return([]os.DirEntry{
		createDirEntry(filepath.Join(symRoot, "BullsEye_UnitTests_iphonesimulator15.5-arm64.xctestrun")),
		createDirEntry(filepath.Join(symRoot, "BullsEye_UITests_iphonesimulator15.5-arm64.xctestrun")),
		createDirEntry(filepath.Join(symRoot, "BullsEye_FullTests_iphonesimulator15.5-arm64.xctestrun")),
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
		SYMRoot:     symRoot,
		ProjectPath: project,
		Scheme:      scheme,
	})

	// Then
	require.NoError(t, err)
	require.Equal(t, filepath.Join(symRoot, "BullsEye_FullTests_iphonesimulator15.5-arm64.xctestrun"), bundle.DefaultXctestrunPth)
	require.Equal(t, []string{
		filepath.Join(symRoot, "BullsEye_UnitTests_iphonesimulator15.5-arm64.xctestrun"),
		filepath.Join(symRoot, "BullsEye_UITests_iphonesimulator15.5-arm64.xctestrun"),
		filepath.Join(symRoot, "BullsEye_FullTests_iphonesimulator15.5-arm64.xctestrun"),
	}, bundle.XctestrunPths)
	require.Equal(t, symRoot, bundle.SYMRoot)
}

type testingMocks struct {
	logger       *mocks.Logger
	xcodeproject *mocks.XcodeProject
	pathChecker  *mocks.PathChecker
	pathProvider *mocks.PathProvider
	dirReader    *mocks.DirReader
}

func createStepAndMocks() (XcodebuildBuilder, testingMocks) {
	logger := new(mocks.Logger)
	xcodeproject := new(mocks.XcodeProject)
	pathChecker := new(mocks.PathChecker)
	pathProvider := new(mocks.PathProvider)
	dirReader := new(mocks.DirReader)

	step := NewXcodebuildBuilder(logger, xcodeproject, pathChecker, pathProvider, dirReader)

	mocks := testingMocks{
		logger:       logger,
		xcodeproject: xcodeproject,
		pathChecker:  pathChecker,
		pathProvider: pathProvider,
		dirReader:    dirReader,
	}

	return step, mocks
}

// simpleDirEntry implements os.DirEntry interface
type simpleDirEntry struct {
	name string
}

func (e simpleDirEntry) Name() string {
	return e.name
}

func (e simpleDirEntry) IsDir() bool {
	return false
}

func (e simpleDirEntry) Type() os.FileMode {
	return os.ModeDevice
}

func (e simpleDirEntry) Info() (os.FileInfo, error) {
	return nil, nil
}

func createDirEntry(pth string) os.DirEntry {
	return simpleDirEntry{name: pth}
}
