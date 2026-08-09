package migrate_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	migrate "github.com/tyemirov/gix/internal/migrate"
)

const (
	testRepositoryRelativeWorkflowsConstant = ".github/workflows"
	testWorkflowFileNameConstant            = "ci.yml"
	testInlineWorkflowContentConstant       = "on: { push: { branches: [main] } }\n"
	testListWorkflowContentConstant         = "on:\n  push:\n    branches:\n      - main\n"
	testNoRewriteWorkflowContentConstant    = "name: build\n"
	testCommentWorkflowContentConstant      = "# run on main branch\n"
)

func TestWorkflowRewriterScenarios(testInstance *testing.T) {
	testCases := []struct {
		name            string
		initialContent  string
		expectedContent string
		expectRewrite   bool
		expectMentions  bool
	}{
		{
			name:            "inline_branches",
			initialContent:  testInlineWorkflowContentConstant,
			expectedContent: "on: { push: { branches: [master] } }\n",
			expectRewrite:   true,
			expectMentions:  false,
		},
		{
			name:            "list_item",
			initialContent:  testListWorkflowContentConstant,
			expectedContent: "on:\n  push:\n    branches:\n      - master\n",
			expectRewrite:   true,
			expectMentions:  false,
		},
		{
			name:            "no_rewrite_needed",
			initialContent:  testNoRewriteWorkflowContentConstant,
			expectedContent: testNoRewriteWorkflowContentConstant,
			expectRewrite:   false,
			expectMentions:  false,
		},
		{
			name:            "leftover_mentions",
			initialContent:  testCommentWorkflowContentConstant,
			expectedContent: testCommentWorkflowContentConstant,
			expectRewrite:   false,
			expectMentions:  true,
		},
	}

	for index, testCase := range testCases {
		testInstance.Run(buildWorkflowSubtestName(index, testCase.name), func(testInstance *testing.T) {
			repositoryDirectory := testInstance.TempDir()
			workflowsDirectory := filepath.Join(repositoryDirectory, testRepositoryRelativeWorkflowsConstant)
			creationError := os.MkdirAll(workflowsDirectory, 0o755)
			require.NoError(testInstance, creationError)

			workflowPath := filepath.Join(workflowsDirectory, testWorkflowFileNameConstant)
			writeError := os.WriteFile(workflowPath, []byte(testCase.initialContent), 0o644)
			require.NoError(testInstance, writeError)

			rewriter := migrate.NewWorkflowRewriter(zap.NewNop())
			outcome, rewriteError := rewriter.Rewrite(context.Background(), migrate.WorkflowRewriteConfig{
				RepositoryPath:     repositoryDirectory,
				WorkflowsDirectory: testRepositoryRelativeWorkflowsConstant,
				SourceBranch:       migrate.BranchMain,
				TargetBranch:       migrate.BranchMaster,
			})
			require.NoError(testInstance, rewriteError)

			fileBytes, readError := os.ReadFile(workflowPath)
			require.NoError(testInstance, readError)
			actualContent := strings.TrimSpace(string(fileBytes))
			expectedContent := strings.TrimSpace(testCase.expectedContent)
			require.Equal(testInstance, expectedContent, actualContent)

			if testCase.expectRewrite {
				require.Len(testInstance, outcome.UpdatedFiles, 1)
			} else {
				require.Len(testInstance, outcome.UpdatedFiles, 0)
			}
			require.Equal(testInstance, testCase.expectMentions, outcome.RemainingMainReferences)
		})
	}
}

func TestWorkflowRewriterMissingDirectory(testInstance *testing.T) {
	rewriter := migrate.NewWorkflowRewriter(zap.NewNop())
	outcome, rewriteError := rewriter.Rewrite(context.Background(), migrate.WorkflowRewriteConfig{
		RepositoryPath:     testInstance.TempDir(),
		WorkflowsDirectory: testRepositoryRelativeWorkflowsConstant,
		SourceBranch:       migrate.BranchMain,
		TargetBranch:       migrate.BranchMaster,
	})
	require.NoError(testInstance, rewriteError)
	require.Empty(testInstance, outcome.UpdatedFiles)
	require.False(testInstance, outcome.RemainingMainReferences)
}

func buildWorkflowSubtestName(index int, name string) string {
	return fmt.Sprintf("%02d_%s", index, name)
}
