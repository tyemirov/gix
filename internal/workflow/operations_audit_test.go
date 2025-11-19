package workflow_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/workflow"
)

const (
	auditReportTestFileNameConstant       = "audit_report.csv"
	auditReportWhitespacePaddingConstant  = " "
	auditReportExpectedHeaderLineConstant = "folder_name,final_github_repo,name_matches,remote_default_branch,local_branch,in_sync,remote_protocol,origin_matches_canonical"
)

func TestAuditReportOperationCreatesNestedOutput(testInstance *testing.T) {
	testCases := []struct {
		name                     string
		outputPathComponents     []string
		shouldTrimConfiguredPath bool
		expectedFileRelativePath string
	}{{
		name:                     "creates nested directories and writes header",
		outputPathComponents:     []string{"nested", "reports"},
		shouldTrimConfiguredPath: true,
		expectedFileRelativePath: auditReportTestFileNameConstant,
	}}

	for testCaseIndex := range testCases {
		currentTestCase := testCases[testCaseIndex]

		testInstance.Run(currentTestCase.name, func(testingInstance *testing.T) {
			testingInstance.Parallel()

			temporaryDirectory := testingInstance.TempDir()
			filePathComponents := append([]string{temporaryDirectory}, currentTestCase.outputPathComponents...)
			filePathComponents = append(filePathComponents, currentTestCase.expectedFileRelativePath)
			configuredOutputPath := filepath.Join(filePathComponents...)
			if currentTestCase.shouldTrimConfiguredPath {
				configuredOutputPath = auditReportWhitespacePaddingConstant + configuredOutputPath + auditReportWhitespacePaddingConstant
			}

			operation := &workflow.AuditReportOperation{OutputPath: configuredOutputPath, WriteToFile: true}
			operationEnvironment := &workflow.Environment{Output: &bytes.Buffer{}}
			operationState := &workflow.State{}

			executionError := operation.Execute(context.Background(), operationEnvironment, operationState)
			require.NoError(testingInstance, executionError)

			sanitizedOutputPath := strings.TrimSpace(configuredOutputPath)
			require.FileExists(testingInstance, sanitizedOutputPath)

			fileParentDirectory := filepath.Dir(sanitizedOutputPath)
			require.DirExists(testingInstance, fileParentDirectory)

			fileContents, fileReadError := os.ReadFile(sanitizedOutputPath)
			require.NoError(testingInstance, fileReadError)

			fileLines := strings.Split(strings.TrimSpace(string(fileContents)), "\n")
			require.NotEmpty(testingInstance, fileLines)
			require.Equal(testingInstance, auditReportExpectedHeaderLineConstant, fileLines[0])
		})
	}
}
