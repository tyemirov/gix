package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/web"
)

func TestWebWorkflowPrimitiveCatalogIncludesAllBuiltInActions(t *testing.T) {
	application := NewApplication()
	catalog := application.newWebWorkflowPrimitiveCatalogLoader()(context.Background())

	require.Empty(t, catalog.Error)
	require.Equal(t, []string{
		webWorkflowPrimitiveCanonicalRemoteConstant,
		webWorkflowPrimitiveProtocolConversionConstant,
		webWorkflowPrimitiveRenameFolderConstant,
		webWorkflowPrimitiveDefaultBranchConstant,
		webWorkflowPrimitiveReleaseTagConstant,
		webWorkflowPrimitiveReleaseRetagConstant,
		webWorkflowPrimitiveAuditReportConstant,
		webWorkflowPrimitiveHistoryPurgeConstant,
		webWorkflowPrimitiveFileReplaceConstant,
		webWorkflowPrimitiveNamespaceRewriteConstant,
	}, workflowPrimitiveCatalogIdentifiers(catalog.Primitives))
}

func TestWebWorkflowPrimitiveDefinitionsBuildOptions(t *testing.T) {
	application := NewApplication()
	definitions := workflowPrimitiveDefinitionIndex(application.webWorkflowPrimitiveDefinitions())

	testCases := []struct {
		name            string
		primitiveID     string
		parameters      map[string]any
		expectedOptions map[string]any
		expectedError   string
	}{
		{
			name:        "protocol conversion validates select values",
			primitiveID: webWorkflowPrimitiveProtocolConversionConstant,
			parameters: map[string]any{
				webWorkflowPrimitiveParameterFromConstant: webWorkflowPrimitiveProtocolHTTPSConstant,
				webWorkflowPrimitiveParameterToConstant:   webWorkflowPrimitiveProtocolSSHConstant,
			},
			expectedOptions: map[string]any{
				webWorkflowPrimitiveParameterFromConstant: webWorkflowPrimitiveProtocolHTTPSConstant,
				webWorkflowPrimitiveParameterToConstant:   webWorkflowPrimitiveProtocolSSHConstant,
			},
		},
		{
			name:        "history purge parses multiline paths and boolean toggles",
			primitiveID: webWorkflowPrimitiveHistoryPurgeConstant,
			parameters: map[string]any{
				webWorkflowPrimitiveParameterPathsConstant:       "vendor\nnode_modules\n",
				webWorkflowPrimitiveParameterRemoteConstant:      "origin",
				webWorkflowPrimitiveParameterPushConstant:        false,
				webWorkflowPrimitiveParameterRestoreConstant:     true,
				webWorkflowPrimitiveParameterPushMissingConstant: true,
			},
			expectedOptions: map[string]any{
				webWorkflowPrimitiveParameterPathsConstant:       []string{"vendor", "node_modules"},
				webWorkflowPrimitiveParameterRemoteConstant:      "origin",
				webWorkflowPrimitiveParameterPushConstant:        false,
				webWorkflowPrimitiveParameterRestoreConstant:     true,
				webWorkflowPrimitiveParameterPushMissingConstant: true,
			},
		},
		{
			name:        "retag parses csv mappings",
			primitiveID: webWorkflowPrimitiveReleaseRetagConstant,
			parameters: map[string]any{
				webWorkflowPrimitiveParameterRemoteConstant:   "origin",
				webWorkflowPrimitiveParameterMappingsConstant: "v1.2.3,main,Retag main\nv1.2.4,release\n",
			},
			expectedOptions: map[string]any{
				webWorkflowPrimitiveParameterRemoteConstant: "origin",
				webWorkflowPrimitiveParameterMappingsConstant: []map[string]any{
					{
						webWorkflowPrimitiveParameterTagConstant:     "v1.2.3",
						webWorkflowPrimitiveParameterTargetConstant:  "main",
						webWorkflowPrimitiveParameterMessageConstant: "Retag main",
					},
					{
						webWorkflowPrimitiveParameterTagConstant:    "v1.2.4",
						webWorkflowPrimitiveParameterTargetConstant: "release",
					},
				},
			},
		},
		{
			name:        "namespace rewrite keeps optional values",
			primitiveID: webWorkflowPrimitiveNamespaceRewriteConstant,
			parameters: map[string]any{
				webWorkflowPrimitiveParameterOldConstant:           "github.com/acme/old",
				webWorkflowPrimitiveParameterNewConstant:           "github.com/acme/new",
				webWorkflowPrimitiveParameterBranchPrefixConstant:  "rewrite/namespace",
				webWorkflowPrimitiveParameterCommitMessageConstant: "Rewrite namespace",
				webWorkflowPrimitiveParameterPushConstant:          false,
				webWorkflowPrimitiveParameterRemoteConstant:        "origin",
			},
			expectedOptions: map[string]any{
				webWorkflowPrimitiveParameterOldConstant:           "github.com/acme/old",
				webWorkflowPrimitiveParameterNewConstant:           "github.com/acme/new",
				webWorkflowPrimitiveParameterBranchPrefixConstant:  "rewrite/namespace",
				webWorkflowPrimitiveParameterCommitMessageConstant: "Rewrite namespace",
				webWorkflowPrimitiveParameterPushConstant:          false,
				webWorkflowPrimitiveParameterRemoteConstant:        "origin",
			},
		},
		{
			name:        "file replace requires find value",
			primitiveID: webWorkflowPrimitiveFileReplaceConstant,
			parameters: map[string]any{
				webWorkflowPrimitiveParameterPatternsConstant: "README.md",
			},
			expectedError: `workflow primitive repo.files.replace requires "find"`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			definition, exists := definitions[testCase.primitiveID]
			require.True(t, exists)

			options, optionsError := definition.buildOptions(testCase.parameters)
			if len(testCase.expectedError) > 0 {
				require.EqualError(t, optionsError, testCase.expectedError)
				return
			}

			require.NoError(t, optionsError)
			require.Equal(t, testCase.expectedOptions, options)
		})
	}
}

func TestWebWorkflowPrimitiveExecutorAppliesFileReplace(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))

	application := NewApplication()
	response := application.newWebWorkflowPrimitiveExecutor()(context.Background(), web.WorkflowPrimitiveApplyRequest{
		Actions: []web.WorkflowPrimitiveQueuedAction{
			{
				ID:             "wf-001",
				RepositoryPath: repositoryPath,
				PrimitiveID:    webWorkflowPrimitiveFileReplaceConstant,
				Parameters: map[string]any{
					webWorkflowPrimitiveParameterPatternsConstant: "README.md",
					webWorkflowPrimitiveParameterFindConstant:     "initial",
					webWorkflowPrimitiveParameterReplaceConstant:  "updated",
				},
			},
		},
	})

	require.Empty(t, response.Error)
	require.Len(t, response.Results, 1)
	require.Equal(t, webAuditChangeStatusSucceededConstant, response.Results[0].Status)
	require.Contains(t, response.Results[0].Stdout, "REPLACE-APPLY")
	require.Empty(t, response.Results[0].Stderr)

	updatedContent, readError := os.ReadFile(filepath.Join(repositoryPath, "README.md"))
	require.NoError(t, readError)
	require.Equal(t, "updated\n", string(updatedContent))
}

func workflowPrimitiveCatalogIdentifiers(primitives []web.WorkflowPrimitiveDescriptor) []string {
	identifiers := make([]string, 0, len(primitives))
	for _, primitive := range primitives {
		identifiers = append(identifiers, primitive.ID)
	}
	return identifiers
}

func workflowPrimitiveDefinitionIndex(definitions []webWorkflowPrimitiveDefinition) map[string]webWorkflowPrimitiveDefinition {
	index := make(map[string]webWorkflowPrimitiveDefinition, len(definitions))
	for _, definition := range definitions {
		index[definition.descriptor.ID] = definition
	}
	return index
}
