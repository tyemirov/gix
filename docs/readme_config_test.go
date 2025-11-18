package docs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/tyemirov/gix/internal/workflow"
)

const (
	documentationFileNameConstant       = "ARCHITECTURE.md"
	yamlFenceStartConstant              = "```yaml"
	yamlFenceEndConstant                = "```"
	configHeaderMarkerConstant          = "# config.yaml"
	architectureSnippetTestNameConstant = "architecture_workflow_configuration"
	architectureSnippetTemporaryPattern = "architecture-config-*.yaml"
	expectedOperationCount              = 8
	parentDirectoryReferenceConstant    = ".."
	missingHeaderMessageConstant        = "Architecture example missing config header marker"
	missingStartFenceMessageConstant    = "Architecture example missing yaml fence start"
	missingEndFenceMessageConstant      = "Architecture example missing yaml fence end"
	unexpectedOperationMessageTemplate  = "unexpected command %s"
	duplicateOperationMessageTemplate   = "duplicate command %s"
	defaultTempDirectoryRootConstant    = ""
)

var expectedCommandOperations = map[string]struct{}{
	"audit":                      {},
	"packages delete":            {},
	"prs delete":                 {},
	"remote update-to-canonical": {},
	"remote update-protocol":     {},
	"folder rename":              {},
	"workflow":                   {},
	"default":                    {},
}

type readmeApplicationConfiguration struct {
	Operations []readmeOperationConfiguration `yaml:"operations"`
}

type readmeOperationConfiguration struct {
	Command []string       `yaml:"command"`
	Options map[string]any `yaml:"with"`
}

func TestArchitectureWorkflowConfigurationParses(testInstance *testing.T) {
	workingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)

	documentationPath := filepath.Join(workingDirectory, parentDirectoryReferenceConstant, documentationFileNameConstant)
	contentBytes, readError := os.ReadFile(documentationPath)
	require.NoError(testInstance, readError)

	contentText := string(contentBytes)
	headerIndex := strings.Index(contentText, configHeaderMarkerConstant)
	require.NotEqual(testInstance, -1, headerIndex, missingHeaderMessageConstant)

	fenceStartIndex := strings.LastIndex(contentText[:headerIndex], yamlFenceStartConstant)
	require.NotEqual(testInstance, -1, fenceStartIndex, missingStartFenceMessageConstant)

	remainingText := contentText[headerIndex:]
	fenceEndRelativeIndex := strings.Index(remainingText, yamlFenceEndConstant)
	require.NotEqual(testInstance, -1, fenceEndRelativeIndex, missingEndFenceMessageConstant)
	fenceEndIndex := headerIndex + fenceEndRelativeIndex

	snippetContent := strings.TrimSpace(contentText[fenceStartIndex+len(yamlFenceStartConstant) : fenceEndIndex])

	testCases := []struct {
		name          string
		configuration string
	}{
		{
			name:          architectureSnippetTestNameConstant,
			configuration: snippetContent,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		testInstance.Run(testCase.name, func(subtest *testing.T) {
			tempFile, tempFileError := os.CreateTemp(defaultTempDirectoryRootConstant, architectureSnippetTemporaryPattern)
			require.NoError(subtest, tempFileError)
			subtest.Cleanup(func() {
				require.NoError(subtest, os.Remove(tempFile.Name()))
			})

			_, writeError := tempFile.WriteString(testCase.configuration)
			require.NoError(subtest, writeError)
			require.NoError(subtest, tempFile.Close())

			_, workflowError := workflow.LoadConfiguration(tempFile.Name())
			require.NoError(subtest, workflowError)

			var applicationConfiguration readmeApplicationConfiguration
			unmarshalError := yaml.Unmarshal([]byte(testCase.configuration), &applicationConfiguration)
			require.NoError(subtest, unmarshalError)

			require.Len(subtest, applicationConfiguration.Operations, expectedOperationCount)

			seenOperations := make(map[string]struct{}, len(applicationConfiguration.Operations))
			for _, operationConfig := range applicationConfiguration.Operations {
				normalizedName := workflow.CommandPathKey(operationConfig.Command)
				require.NotEmpty(subtest, normalizedName)
				_, expected := expectedCommandOperations[normalizedName]
				require.Truef(subtest, expected, unexpectedOperationMessageTemplate, normalizedName)

				_, duplicate := seenOperations[normalizedName]
				require.Falsef(subtest, duplicate, duplicateOperationMessageTemplate, normalizedName)
				seenOperations[normalizedName] = struct{}{}
			}
		})
	}
}
