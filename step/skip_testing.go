package step

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// TestPathString is used to avoid JSON marshalling double escape "\" ("\\/").
type TestPathString string

func (cs TestPathString) MarshalJSON() ([]byte, error) {
	escaped := `"` + string(cs) + `"`
	return []byte(escaped), nil
}

func (b XcodebuildBuilder) findTestPlan(testPlan, projectPath string) (string, error) {
	return b.fileManager.FindFile(filepath.Dir(projectPath), testPlan+".xctestplan")
}

func (b XcodebuildBuilder) backupTestPlan(testPlanPath string) (string, error) {
	tmpDir, err := b.pathProvider.CreateTempDir("backupTestPlans")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir for backup: %w", err)
	}

	backupTestPlanPath := filepath.Join(tmpDir, filepath.Base(testPlanPath))

	testPlanContent, err := b.fileManager.ReadFile(testPlanPath)
	if err != nil {
		return "", fmt.Errorf("failed to read original test plan: %w", err)
	}

	if err := b.fileManager.WriteFile(backupTestPlanPath, testPlanContent, 0644); err != nil {
		return "", fmt.Errorf("failed to write backup test plan: %w", err)
	}

	return backupTestPlanPath, nil
}

func (b XcodebuildBuilder) addSkippedTestsToTestPlanFile(testPlanPath string, skipTesting []string) error {
	skipTestingInTargets, err := b.parseSkipTestingFormat(skipTesting)
	if err != nil {
		return fmt.Errorf("failed to parse skip testing format: %w", err)
	}

	testPlan, err := b.readAndParseTestPlan(testPlanPath)
	if err != nil {
		return fmt.Errorf("failed to read and parse test plan: %w", err)
	}

	updatedTestPlan, err := b.addSkippedTestsToTestPlan(testPlan, skipTestingInTargets)
	if err != nil {
		return fmt.Errorf("failed to add skipped tests to test plan: %w", err)
	}

	if err := b.writeTestPlan(testPlanPath, updatedTestPlan); err != nil {
		return fmt.Errorf("failed to write updated test plan: %w", err)
	}

	return nil
}

func (b XcodebuildBuilder) restoreTestPlan(backupTestPlanPath, originalTestPlanPath string) error {
	backupTestPlanContent, err := b.fileManager.ReadFile(backupTestPlanPath)
	if err != nil {
		return fmt.Errorf("failed to read backup test plan: %w", err)
	}

	if err := b.fileManager.WriteFile(originalTestPlanPath, backupTestPlanContent, 0644); err != nil {
		return fmt.Errorf("failed to restore original test plan: %w", err)
	}

	b.logger.Printf("Original test plan restored from backup: %s", originalTestPlanPath)
	return nil
}

func (b XcodebuildBuilder) writeTestPlan(testPlanPath string, updatedTestPlan map[string]interface{}) error {
	updatedTestPlanContent, err := json.MarshalIndent(updatedTestPlan, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal updated test plan to JSON: %w", err)
	}

	if err := b.fileManager.WriteFile(testPlanPath, updatedTestPlanContent, 0644); err != nil {
		return fmt.Errorf("failed to write updated test plan to file: %w", err)
	}

	return nil
}

func (b XcodebuildBuilder) parseSkipTestingFormat(skipTesting []string) (map[string][]TestPathString, error) {
	// skipTesting item format: TestTarget[/TestClass[/TestMethod]], for example: MyAppTests/MyAppTests/testExample
	// skipTestingInTargets stores "TestClass/TestMethod" per TestTarget to be skipped.
	// The test plan JSON file expects the "skippedTests" array items to have "/" escaped as "\/", TestPathString is used
	// to avoid JSON marshalling double escape "\" ("\\/").
	skipTestingInTargets := map[string][]TestPathString{}
	for _, skipTest := range skipTesting {
		skipTestSplit := strings.Split(skipTest, "/")
		if len(skipTestSplit) == 1 {
			return nil, fmt.Errorf("not yet supported skip testing format: %s", skipTest)
		}
		if len(skipTestSplit) != 2 && len(skipTestSplit) != 3 {
			return nil, fmt.Errorf("invalid skip testing format: %s", skipTest)
		}

		skipTest = strings.Join(skipTestSplit[1:], "/")
		skipTest = strings.ReplaceAll(skipTest, "/", `\/`)
		testTarget := skipTestSplit[0]
		testTargetSkipTesting := skipTestingInTargets[testTarget]
		testTargetSkipTesting = append(testTargetSkipTesting, TestPathString(skipTest))
		skipTestingInTargets[testTarget] = testTargetSkipTesting
	}
	return skipTestingInTargets, nil
}

func (b XcodebuildBuilder) readAndParseTestPlan(testPlanPath string) (map[string]interface{}, error) {
	testPlanContent, err := b.fileManager.ReadFile(testPlanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read test plan file: %w", err)
	}

	var testPlan map[string]interface{}
	if err := json.Unmarshal(testPlanContent, &testPlan); err != nil {
		return nil, fmt.Errorf("failed to unmarshal test plan JSON: %w", err)
	}

	return testPlan, nil
}

func (b XcodebuildBuilder) addSkippedTestsToTestPlan(testPlan map[string]interface{}, skipTestingInTargets map[string][]TestPathString) (map[string]interface{}, error) {
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

		var skippedTests []TestPathString
		skippedTestsRaw, ok := target["skippedTests"]
		if ok {
			skippedTestsList, ok := skippedTestsRaw.([]string)
			if !ok {
				return nil, fmt.Errorf("invalid skippedTests format in test target")
			}

			for _, skippedTestsListItem := range skippedTestsList {
				skippedTests = append(skippedTests, TestPathString(skippedTestsListItem))
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
