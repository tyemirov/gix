package migrate_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/githubcli"
	migrate "github.com/tyemirov/gix/internal/migrate"
)

const (
	pagesSubtestNameTemplateConstant = "%02d_%s"
	legacyBuildTypeCaseNameConstant  = "legacy_updates"
	disabledPagesCaseNameConstant    = "disabled"
	workflowBuildCaseNameConstant    = "workflow_build"
	branchMismatchCaseNameConstant   = "branch_mismatch"
	updateFailureCaseNameConstant    = "update_failure"
	pagesTestRepositoryIdentifier    = "owner/example"
)

type stubGitHubOperations struct {
	getPagesFunc    func(context.Context, string) (githubcli.PagesStatus, error)
	updatePagesFunc func(context.Context, string, githubcli.PagesConfiguration) error
}

func (stub *stubGitHubOperations) ResolveRepoMetadata(context.Context, string) (githubcli.RepositoryMetadata, error) {
	return githubcli.RepositoryMetadata{}, nil
}

func (stub *stubGitHubOperations) GetPagesConfig(ctx context.Context, repository string) (githubcli.PagesStatus, error) {
	if stub.getPagesFunc != nil {
		return stub.getPagesFunc(ctx, repository)
	}
	return githubcli.PagesStatus{}, nil
}

func (stub *stubGitHubOperations) UpdatePagesConfig(ctx context.Context, repository string, configuration githubcli.PagesConfiguration) error {
	if stub.updatePagesFunc != nil {
		return stub.updatePagesFunc(ctx, repository, configuration)
	}
	return nil
}

func (stub *stubGitHubOperations) ListPullRequests(context.Context, string, githubcli.PullRequestListOptions) ([]githubcli.PullRequest, error) {
	return nil, nil
}

func (stub *stubGitHubOperations) UpdatePullRequestBase(context.Context, string, int, string) error {
	return nil
}

func (stub *stubGitHubOperations) SetDefaultBranch(context.Context, string, string) error {
	return nil
}

func (stub *stubGitHubOperations) CheckBranchProtection(context.Context, string, string) (bool, error) {
	return false, nil
}

func TestPagesManagerScenarios(testInstance *testing.T) {
	testCases := []struct {
		name          string
		status        githubcli.PagesStatus
		updateError   error
		expectUpdated bool
	}{
		{
			name: legacyBuildTypeCaseNameConstant,
			status: githubcli.PagesStatus{
				Enabled:      true,
				BuildType:    githubcli.PagesBuildTypeLegacy,
				SourceBranch: "main",
				SourcePath:   "/docs",
			},
			expectUpdated: true,
		},
		{
			name: disabledPagesCaseNameConstant,
			status: githubcli.PagesStatus{
				Enabled: false,
			},
			expectUpdated: false,
		},
		{
			name: workflowBuildCaseNameConstant,
			status: githubcli.PagesStatus{
				Enabled:      true,
				BuildType:    githubcli.PagesBuildTypeWorkflow,
				SourceBranch: "main",
			},
			expectUpdated: false,
		},
		{
			name: branchMismatchCaseNameConstant,
			status: githubcli.PagesStatus{
				Enabled:      true,
				BuildType:    githubcli.PagesBuildTypeLegacy,
				SourceBranch: "develop",
			},
			expectUpdated: false,
		},
		{
			name: updateFailureCaseNameConstant,
			status: githubcli.PagesStatus{
				Enabled:      true,
				BuildType:    githubcli.PagesBuildTypeLegacy,
				SourceBranch: "main",
			},
			updateError:   errors.New("update failed"),
			expectUpdated: false,
		},
	}

	for index, testCase := range testCases {
		testInstance.Run(buildPagesSubtestName(index, testCase.name), func(testInstance *testing.T) {
			var capturedConfiguration githubcli.PagesConfiguration
			operationsStub := &stubGitHubOperations{}
			operationsStub.getPagesFunc = func(_ context.Context, repository string) (githubcli.PagesStatus, error) {
				_ = repository
				return testCase.status, nil
			}
			operationsStub.updatePagesFunc = func(_ context.Context, repository string, configuration githubcli.PagesConfiguration) error {
				_ = repository
				capturedConfiguration = configuration
				return testCase.updateError
			}

			manager := migrate.NewPagesManager(zap.NewNop(), operationsStub)
			updated, executionError := manager.EnsureLegacyBranch(context.Background(), migrate.PagesUpdateConfig{
				RepositoryIdentifier: pagesTestRepositoryIdentifier,
				SourceBranch:         migrate.BranchMain,
				TargetBranch:         migrate.BranchMaster,
			})

			if testCase.updateError != nil {
				require.Error(testInstance, executionError)
				return
			}

			require.NoError(testInstance, executionError)
			require.Equal(testInstance, testCase.expectUpdated, updated)
			if testCase.expectUpdated {
				require.Equal(testInstance, string(migrate.BranchMaster), capturedConfiguration.SourceBranch)
				require.Equal(testInstance, testCase.status.SourcePath, capturedConfiguration.SourcePath)
			}
		})
	}
}

func buildPagesSubtestName(index int, name string) string {
	return fmt.Sprintf(pagesSubtestNameTemplateConstant, index, name)
}
