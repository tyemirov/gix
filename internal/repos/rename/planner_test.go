package rename_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/repos/rename"
	"github.com/tyemirov/gix/internal/repos/shared"
)

const (
	plannerOwnerRepositoryConstant          = "owner/example"
	plannerAlternateOwnerRepositoryConstant = "alternate/sample"
	plannerDefaultFolderNameConstant        = "example"
	plannerRepositoryPathConstant           = "/tmp/example"
	plannerOwnerRepositoryPathConstant      = "/tmp/owner/example"
)

func parseOwnerRepo(t *testing.T, raw string) *shared.OwnerRepository {
	t.Helper()
	repo, err := shared.ParseOwnerRepositoryOptional(raw)
	require.NoError(t, err)
	return repo
}

func TestDirectoryPlannerPlan(testInstance *testing.T) {
	testCases := []struct {
		name              string
		includeOwner      bool
		finalOwnerRepo    string
		defaultFolderName string
		expectedFolder    string
		expectedInclude   bool
	}{
		{
			name:              "without_owner_uses_default",
			includeOwner:      false,
			finalOwnerRepo:    plannerOwnerRepositoryConstant,
			defaultFolderName: plannerDefaultFolderNameConstant,
			expectedFolder:    plannerDefaultFolderNameConstant,
			expectedInclude:   false,
		},
		{
			name:              "with_owner_builds_nested_folder",
			includeOwner:      true,
			finalOwnerRepo:    plannerOwnerRepositoryConstant,
			defaultFolderName: plannerDefaultFolderNameConstant,
			expectedFolder:    filepath.Join("owner", "example"),
			expectedInclude:   true,
		},
		{
			name:              "with_owner_missing_identifier_uses_default",
			includeOwner:      true,
			finalOwnerRepo:    "",
			defaultFolderName: plannerDefaultFolderNameConstant,
			expectedFolder:    plannerDefaultFolderNameConstant,
			expectedInclude:   false,
		},
	}

	planner := rename.NewDirectoryPlanner()
	for _, testCase := range testCases {
		testCase := testCase
		testInstance.Run(testCase.name, func(subtest *testing.T) {
			owner, err := shared.ParseOwnerRepositoryOptional(testCase.finalOwnerRepo)
			require.NoError(subtest, err)

			plan := planner.Plan(testCase.includeOwner, owner, testCase.defaultFolderName)
			require.Equal(subtest, testCase.expectedFolder, plan.FolderName)
			require.Equal(subtest, testCase.expectedInclude, plan.IncludeOwner)
		})
	}
}

func TestDirectoryPlanIsNoop(testInstance *testing.T) {
	planner := rename.NewDirectoryPlanner()
	ownerRepo := parseOwnerRepo(testInstance, plannerOwnerRepositoryConstant)
	alternateOwner := parseOwnerRepo(testInstance, plannerAlternateOwnerRepositoryConstant)
	testCases := []struct {
		name           string
		plan           rename.DirectoryPlan
		repositoryPath string
		currentFolder  string
		expectedIsNoop bool
	}{
		{
			name:           "empty_target_skips",
			plan:           rename.DirectoryPlan{FolderName: ""},
			repositoryPath: plannerRepositoryPathConstant,
			currentFolder:  plannerDefaultFolderNameConstant,
			expectedIsNoop: true,
		},
		{
			name:           "matching_folder_without_owner",
			plan:           planner.Plan(false, ownerRepo, plannerDefaultFolderNameConstant),
			repositoryPath: plannerRepositoryPathConstant,
			currentFolder:  plannerDefaultFolderNameConstant,
			expectedIsNoop: true,
		},
		{
			name:           "mismatched_folder_without_owner",
			plan:           planner.Plan(false, ownerRepo, plannerDefaultFolderNameConstant),
			repositoryPath: plannerRepositoryPathConstant,
			currentFolder:  "legacy",
			expectedIsNoop: false,
		},
		{
			name:           "matching_owner_repository_suffix",
			plan:           planner.Plan(true, ownerRepo, plannerDefaultFolderNameConstant),
			repositoryPath: plannerOwnerRepositoryPathConstant,
			currentFolder:  plannerDefaultFolderNameConstant,
			expectedIsNoop: true,
		},
		{
			name:           "different_owner_repository_suffix",
			plan:           planner.Plan(true, alternateOwner, plannerDefaultFolderNameConstant),
			repositoryPath: plannerOwnerRepositoryPathConstant,
			currentFolder:  plannerDefaultFolderNameConstant,
			expectedIsNoop: false,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		testInstance.Run(testCase.name, func(subtest *testing.T) {
			isNoop := testCase.plan.IsNoop(testCase.repositoryPath, testCase.currentFolder)
			require.Equal(subtest, testCase.expectedIsNoop, isNoop)
		})
	}
}
