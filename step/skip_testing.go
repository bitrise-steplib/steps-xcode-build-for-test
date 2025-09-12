package step

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func (b XcodebuildBuilder) skipTesting(testPlan, projectPath string, skipTesting []string) error {
	testPlanPath, err := b.findTestPlan(testPlan, projectPath)
	if err != nil {
		return fmt.Errorf("could not find test plan %s: %w", testPlan, err)
	}
	if testPlanPath == "" {
		return fmt.Errorf("test plan %s not found in project directory", testPlan)
	}

	b.logger.Printf("Found test plan at: %s", testPlanPath)

	updatedTestPlan, err := b.addSkippedTestsToTestPlan(testPlanPath, skipTesting)
	if err != nil {
		return fmt.Errorf("failed to add skipped tests to test plan: %w", err)
	}

	if err := b.writeTestPlan(testPlanPath, updatedTestPlan); err != nil {
		return fmt.Errorf("failed to write updated test plan: %w", err)
	}

	return nil
}

func (b XcodebuildBuilder) findTestPlan(testPlan, projectPath string) (string, error) {
	var testPlanPath string
	projectRootDir := filepath.Dir(projectPath)
	if err := filepath.WalkDir(projectRootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		if filepath.Base(path) == testPlan+".xctestplan" {
			testPlanPath = path
			return filepath.SkipAll
		}

		return nil
	}); err != nil {
		return "", err
	}

	return testPlanPath, nil
}

type CustomString string

func (cs CustomString) MarshalJSON() ([]byte, error) {
	escaped := `"` + string(cs) + `"`
	return []byte(escaped), nil
}

func (b XcodebuildBuilder) addSkippedTestsToTestPlan(testPlanPath string, skipTesting []string) (map[string]interface{}, error) {
	// TestTarget[/TestClass[/TestMethod]]: MyAppTests/MyAppTests/testExample
	skipTestingInTargets := map[string][]CustomString{}
	for _, skipTest := range skipTesting {
		skipTestSplit := strings.Split(skipTest, "/")
		if len(skipTestSplit) == 1 {
			return nil, fmt.Errorf("not yet supported skip testing format: %s", testPlanPath)
		}
		if len(skipTestSplit) != 2 && len(skipTestSplit) != 3 {
			return nil, fmt.Errorf("invalid skip testing format: %s", testPlanPath)
		}

		skipTest = strings.Join(skipTestSplit[1:], "/")
		//skipTestListItem := strings.Replace(skipTest, "/", `\/`, -1)
		skipTest = strings.ReplaceAll(skipTest, "/", `\/`)
		testTarget := skipTestSplit[0]
		testTargetSkipTesting := skipTestingInTargets[testTarget]
		testTargetSkipTesting = append(testTargetSkipTesting, CustomString(skipTest))
		skipTestingInTargets[testTarget] = testTargetSkipTesting
	}

	testPlanContent, err := os.ReadFile(testPlanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read test plan file: %w", err)
	}

	var testPlan map[string]interface{}
	if err := json.Unmarshal(testPlanContent, &testPlan); err != nil {
		return nil, fmt.Errorf("failed to unmarshal test plan JSON: %w", err)
	}

	testTargetsRaw, ok := testPlan["testTargets"]
	if !ok {
		return nil, fmt.Errorf("testTargets not found in test plan")
	}
	testTargetList, ok := testTargetsRaw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("testTargets not found or invalid in test plan")
	}

	for idx, testTargetItemRaw := range testTargetList {
		testTargetItem, ok := testTargetItemRaw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid test target format in test plan")
		}
		targetRaw, ok := testTargetItem["target"]
		if !ok {
			return nil, fmt.Errorf("target not found in test target")
		}
		target, ok := targetRaw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid target format in test target")
		}

		targetNameRaw, ok := target["name"]
		if !ok {
			return nil, fmt.Errorf("name not found in test target")
		}

		targetName, ok := targetNameRaw.(string)
		if !ok {
			return nil, fmt.Errorf("invalid name format in test target")
		}

		skipTestingToAdd, ok := skipTestingInTargets[targetName]
		if !ok {
			continue
		}

		var skippedTests []CustomString
		skippedTestsRaw, ok := target["skippedTests"]
		if ok {
			skippedTestsList, ok := skippedTestsRaw.([]string)
			if !ok {
				return nil, fmt.Errorf("invalid skippedTests format in test target")
			}

			for _, skippedTestsListItem := range skippedTestsList {
				skippedTests = append(skippedTests, CustomString(skippedTestsListItem))
			}
		}

		skippedTests = append(skippedTests, skipTestingToAdd...)
		testTargetItem["skippedTests"] = skippedTests
		testTargetItem["target"] = target
		testTargetList[idx] = testTargetItem
		testPlan["testTargets"] = testTargetList
	}

	return testPlan, nil
}

func (b XcodebuildBuilder) writeTestPlan(testPlanPath string, updatedTestPlan map[string]interface{}) error {
	updatedTestPlanContent, err := json.MarshalIndent(updatedTestPlan, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal updated test plan to JSON: %w", err)
	}

	if err := b.fileManager.WriteFile(testPlanPath, updatedTestPlanContent, 0644); err != nil {
		return fmt.Errorf("failed to write updated test plan to file: %w", err)
	}

	b.logger.Printf("Updated test plan written to: %s", testPlanPath)
	return nil
}
