package step

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bitrise-io/go-utils/v2/pathutil"
	"github.com/bitrise-io/go-xcode/xcodeproject/xcscheme"
	"github.com/bitrise-steplib/steps-xcode-build-for-test/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_GivenProject_WhenSkippingTests_ThenUpdatestTestPlan(t *testing.T) {
	// Given
	step, stepMocks := createStepAndMocks()

	projectPth := "BullsEye.xcworkspace"
	testPlan := "FullTests"
	testPlanPath := "path/to/FullTests.xctestplan"
	testPlanContent := []byte(`{
    "testTargets": [
        {
            "target": {
                "containerPath": "container:BullsEye.xcodeproj",
                "identifier": "13FB7D0B26773C570084066F",
                "name": "BullsEyeUITests"
            }
        }
    ]
}
`)
	tmpDir := "tmp"
	tmpTestPlanPath := filepath.Join(tmpDir, "FullTests.xctestplan")
	skipTesting := []string{"BullsEyeUITests/BullsEyeUITests/testExample"}
	// skippedTests field added to the original test plan content
	updatedTestPlan := []byte(`{
  "testTargets": [
    {
      "skippedTests": [
        "BullsEyeUITests\/testExample"
      ],
      "target": {
        "containerPath": "container:BullsEye.xcodeproj",
        "identifier": "13FB7D0B26773C570084066F",
        "name": "BullsEyeUITests"
      }
    }
  ]
}`)

	stepMocks.logger.On("Println").Return()
	stepMocks.logger.On("Infof", "Updating test plan (%s) to skip tests", testPlan).Return()
	stepMocks.logger.On("Printf", "Original test plan restored from backup: %s", testPlanPath).Return()
	stepMocks.logger.On("Printf", "%d skip testing item(s) added to test plan", 1).Return()

	// find and update test plan
	stepMocks.fileManager.On("FindFile", ".", testPlan+".xctestplan").Return(testPlanPath, nil)
	stepMocks.fileManager.On("ReadFile", testPlanPath).Return(testPlanContent, nil)
	stepMocks.fileManager.On("WriteFile", testPlanPath, updatedTestPlan, mock.Anything).Return(nil)
	// backup and restore test plan
	stepMocks.pathProvider.On("CreateTempDir", mock.Anything).Return(tmpDir, nil)
	stepMocks.fileManager.On("WriteFile", tmpTestPlanPath, testPlanContent, mock.Anything).Return(nil)
	stepMocks.fileManager.On("ReadFile", tmpTestPlanPath).Return(testPlanContent, nil)
	stepMocks.fileManager.On("WriteFile", testPlanPath, testPlanContent, mock.Anything).Return(nil)

	// When
	err := step.skipTesting(testPlan, projectPth, skipTesting)

	// Then
	require.NoError(t, err)
}

func Test_GivenIosProjectProducesOneXctestrun_WhenFindTestBundle_ThenReturnsTestBundle(t *testing.T) {
	// Given
	step, stepMocks := createStepAndMocks()

	project := "BullsEye.xcworkspace"
	scheme := "BullsEye"
	symRoot := "$HOME/Library/Developer/Xcode/DerivedData/BullsEye-exnjhblzvmjcydaiwoxkklkizqxc/Build/Products"

	stepMocks.logger.On("Printf", mock.Anything, mock.Anything).Return()
	stepMocks.logger.On("Donef", mock.Anything, mock.Anything).Return()
	stepMocks.pathChecker.On("IsPathExists", mock.Anything).Return(true, nil)
	stepMocks.fileManager.On("ReadDir", mock.Anything).Return([]os.DirEntry{
		createDirEntry("BullsEye_FullTests_iphonesimulator15.5-arm64.xctestrun"),
	}, nil)
	stepMocks.fileManager.On("ReadFile", mock.Anything).Return([]byte{}, nil)
	stepMocks.fileManager.On("WriteFile", mock.Anything, mock.Anything, mock.Anything).Return(nil)

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
	stepMocks.fileManager.On("ReadDir", mock.Anything).Return([]os.DirEntry{
		createDirEntry("BullsEye_UnitTests_iphonesimulator15.5-arm64.xctestrun"),
		createDirEntry("BullsEye_UITests_iphonesimulator15.5-arm64.xctestrun"),
		createDirEntry("BullsEye_FullTests_iphonesimulator15.5-arm64.xctestrun"),
	}, nil)
	stepMocks.fileManager.On("ReadFile", mock.Anything).Return([]byte{}, nil)
	stepMocks.fileManager.On("WriteFile", mock.Anything, mock.Anything, mock.Anything).Return(nil)
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
	pathModifier *pathutil.PathModifier
	pathProvider *mocks.PathProvider
	fileManager  *mocks.FileManager
}

func createStepAndMocks() (XcodebuildBuilder, testingMocks) {
	logger := new(mocks.Logger)
	xcodeproject := new(mocks.XcodeProject)
	pathChecker := new(mocks.PathChecker)
	fileManager := new(mocks.FileManager)
	pathModifier := pathutil.NewPathModifier()
	pathProvider := new(mocks.PathProvider)

	step := NewXcodebuildBuilder(logger, xcodeproject, pathChecker, pathModifier, pathProvider, fileManager)

	mocks := testingMocks{
		logger:       logger,
		xcodeproject: xcodeproject,
		pathChecker:  pathChecker,
		pathModifier: &pathModifier,
		pathProvider: pathProvider,
		fileManager:  fileManager,
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
