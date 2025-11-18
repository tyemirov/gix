package migrate_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	migrate "github.com/tyemirov/gix/internal/migrate"
)

const (
	safetySubtestNameTemplateConstant = "%02d_%s"
)

func TestSafetyEvaluatorScenarios(testInstance *testing.T) {
	testCases := []struct {
		name            string
		inputs          migrate.SafetyInputs
		expectedSafe    bool
		expectedReasons []string
	}{
		{
			name:            "all_clear",
			inputs:          migrate.SafetyInputs{},
			expectedSafe:    true,
			expectedReasons: []string{},
		},
		{
			name: "open_pull_requests",
			inputs: migrate.SafetyInputs{
				OpenPullRequestCount: 3,
			},
			expectedSafe:    false,
			expectedReasons: []string{"open pull requests still target source branch"},
		},
		{
			name: "branch_protected",
			inputs: migrate.SafetyInputs{
				BranchProtected: true,
			},
			expectedSafe:    false,
			expectedReasons: []string{"source branch is protected"},
		},
		{
			name: "workflow_mentions",
			inputs: migrate.SafetyInputs{
				WorkflowMentions: true,
			},
			expectedSafe:    false,
			expectedReasons: []string{"workflow files still reference source branch"},
		},
		{
			name: "multiple_blockers",
			inputs: migrate.SafetyInputs{
				OpenPullRequestCount: 1,
				BranchProtected:      true,
				WorkflowMentions:     true,
			},
			expectedSafe: false,
			expectedReasons: []string{
				"open pull requests still target source branch",
				"source branch is protected",
				"workflow files still reference source branch",
			},
		},
	}

	evaluator := migrate.SafetyEvaluator{}
	for index, testCase := range testCases {
		testInstance.Run(buildSafetySubtestName(index, testCase.name), func(testInstance *testing.T) {
			status := evaluator.Evaluate(testCase.inputs)
			require.Equal(testInstance, testCase.expectedSafe, status.SafeToDelete)
			require.Equal(testInstance, testCase.expectedReasons, status.BlockingReasons)
		})
	}
}

func buildSafetySubtestName(index int, name string) string {
	return fmt.Sprintf(safetySubtestNameTemplateConstant, index, name)
}
