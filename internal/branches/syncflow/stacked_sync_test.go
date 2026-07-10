package syncflow

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/audit"
	"github.com/tyemirov/gix/internal/githubcli"
	"github.com/tyemirov/gix/internal/gitrepo"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/workflow"
)

func TestPlanStrictSyncStack(t *testing.T) {
	testCases := []struct {
		name              string
		resolutionSource  string
		localBranch       string
		dirty             bool
		stashChanges      bool
		missingReferences map[string]bool
		configValues      map[string]string
		expectedPlan      *strictSyncStackPlan
		expectedError     string
	}{
		{
			name:             "current branch sync is not explicit child creation",
			resolutionSource: syncCurrentBranchSource,
			localBranch:      "feature/parent",
			dirty:            true,
		},
		{
			name:             "existing remote target keeps its established contract",
			resolutionSource: branchResolutionSourceExplicit,
			localBranch:      "feature/parent",
			dirty:            true,
			missingReferences: map[string]bool{
				"refs/heads/feature/child": true,
			},
		},
		{
			name:             "existing local target keeps its established contract",
			resolutionSource: branchResolutionSourceExplicit,
			localBranch:      "feature/parent",
			dirty:            true,
			missingReferences: map[string]bool{
				"origin/feature/child": true,
			},
		},
		{
			name:             "stored review base recovers an existing child on plain sync",
			resolutionSource: syncCurrentBranchSource,
			localBranch:      "feature/child",
			configValues: map[string]string{
				strictSyncStackReviewBaseKey("feature/child"): "feature/parent",
			},
			expectedPlan: &strictSyncStackPlan{
				ChildBranch:  "feature/child",
				ParentBranch: "feature/parent",
			},
		},
		{
			name:             "stored review base cannot point to the child",
			resolutionSource: syncCurrentBranchSource,
			localBranch:      "feature/child",
			configValues: map[string]string{
				strictSyncStackReviewBaseKey("feature/child"): "feature/child",
			},
			expectedError: `cannot resolve stacked pull-request chain: review base cycle at branch "feature/child"`,
		},
		{
			name:              "detached checkout has no parent branch",
			resolutionSource:  branchResolutionSourceExplicit,
			dirty:             true,
			missingReferences: missingStrictSyncStackChildReferences(),
			expectedError:     `cannot create stacked branch "feature/child" from a detached HEAD`,
		},
		{
			name:              "clean child has no pull request delta",
			resolutionSource:  branchResolutionSourceExplicit,
			localBranch:       "feature/parent",
			missingReferences: missingStrictSyncStackChildReferences(),
			expectedError:     `cannot create stacked branch "feature/child" from "feature/parent": no changes would remain for its pull request`,
		},
		{
			name:              "stashed child has no committed pull request delta",
			resolutionSource:  branchResolutionSourceExplicit,
			localBranch:       "feature/parent",
			dirty:             true,
			stashChanges:      true,
			missingReferences: missingStrictSyncStackChildReferences(),
			expectedError:     `cannot create stacked branch "feature/child" from "feature/parent": no changes would remain for its pull request`,
		},
		{
			name:              "dirty missing child stacks on current branch",
			resolutionSource:  branchResolutionSourceExplicit,
			localBranch:       "feature/parent",
			dirty:             true,
			missingReferences: missingStrictSyncStackChildReferences(),
			expectedPlan: &strictSyncStackPlan{
				ChildBranch:      "feature/child",
				ParentBranch:     "feature/parent",
				RecordReviewBase: true,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			gitExecutor := &strictSyncGitExecutor{
				missingReferences: testCase.missingReferences,
				configValues:      testCase.configValues,
			}
			environment := &workflow.Environment{GitExecutor: gitExecutor}
			repository := &workflow.RepositoryState{
				Path: "/tmp/project",
				Inspection: audit.RepositoryInspection{
					LocalBranch: testCase.localBranch,
				},
			}

			plan, planErr := planStrictSyncStack(context.Background(), environment, repository, strictSyncStackPlanningOptions{
				RemoteName:       shared.OriginRemoteNameConstant,
				ChildBranch:      "feature/child",
				ResolutionSource: testCase.resolutionSource,
				Dirty:            testCase.dirty,
				StashChanges:     testCase.stashChanges,
			})

			if testCase.expectedError != "" {
				require.EqualError(t, planErr, testCase.expectedError)
				require.Nil(t, plan)
				return
			}
			require.NoError(t, planErr)
			require.Equal(t, testCase.expectedPlan, plan)
		})
	}
}

func TestPlanStrictSyncStackUsesMergedChildPullRequestBaseForHandoff(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{configValues: map[string]string{
		strictSyncStackReviewBaseKey("feature/child"): "feature/stale-parent",
	}}
	githubExecutor := &strictSyncGitHubExecutor{outputs: []string{
		`[]`,
		`[{"number":8,"title":"Child","headRefName":"feature/child","baseRefName":"feature/actual-parent"}]`,
	}}
	environment, repository := strictSyncStackTestContext(t, gitExecutor, githubExecutor)

	plan, planErr := planStrictSyncStack(context.Background(), environment, repository, strictSyncStackPlanningOptions{
		RemoteName:       shared.OriginRemoteNameConstant,
		ChildBranch:      "feature/child",
		ResolutionSource: syncCurrentBranchSource,
	})

	require.NoError(t, planErr)
	require.Equal(t, &strictSyncStackPlan{
		ChildBranch:            "feature/child",
		ParentBranch:           "feature/actual-parent",
		ChildPullRequestMerged: true,
	}, plan)
	require.Len(t, githubExecutor.commands, 2)
	require.Equal(t, string(githubcli.PullRequestStateOpen), githubCommandOption(githubExecutor.commands[0].Arguments, "--state"))
	require.Equal(t, string(githubcli.PullRequestStateMerged), githubCommandOption(githubExecutor.commands[1].Arguments, "--state"))
}

func TestEnsureStrictSyncStackParentUsesExistingOpenPullRequest(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{}
	githubExecutor := &strictSyncGitHubExecutor{output: `[{"number":7,"title":"Parent","headRefName":"feature/parent","baseRefName":"feature/grandparent"}]`}
	environment, repository := strictSyncStackTestContext(t, gitExecutor, githubExecutor)

	parentErr := ensureStrictSyncStackParent(context.Background(), environment, repository, strictSyncStackParentOptions{
		RemoteName:           shared.OriginRemoteNameConstant,
		ConfiguredBaseBranch: defaultSyncBaseBranch,
		Plan: strictSyncStackPlan{
			ChildBranch:  "feature/child",
			ParentBranch: "feature/parent",
		},
	})

	require.NoError(t, parentErr)
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "push -u origin feature/parent")
	require.Len(t, githubExecutor.commands, 1)
	require.Equal(t, string(githubcli.PullRequestStateOpen), githubCommandOption(githubExecutor.commands[0].Arguments, "--state"))
	require.Equal(t, "feature/parent", githubCommandOption(githubExecutor.commands[0].Arguments, "--head"))
}

func TestEnsureStrictSyncStackParentAcceptsOpenRemoteAncestorWithoutLocalBranch(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{missingReferences: map[string]bool{
		"refs/heads/feature/grandparent": true,
	}}
	githubExecutor := &strictSyncGitHubExecutor{output: `[{"number":7,"title":"Grandparent","headRefName":"feature/grandparent","baseRefName":"master"}]`}
	environment, repository := strictSyncStackTestContext(t, gitExecutor, githubExecutor)

	parentErr := ensureStrictSyncStackParent(context.Background(), environment, repository, strictSyncStackParentOptions{
		RemoteName:           shared.OriginRemoteNameConstant,
		ConfiguredBaseBranch: defaultSyncBaseBranch,
		Plan: strictSyncStackPlan{
			ChildBranch:  "feature/parent",
			ParentBranch: "feature/grandparent",
		},
	})

	require.NoError(t, parentErr)
	require.NotContains(t, recordedGitCommands(gitExecutor.commands), "push -u origin feature/grandparent")
	require.NotContains(t, recordedGitCommands(gitExecutor.commands), "rev-list --count feature/grandparent..origin/feature/grandparent")
}

func TestEnsureStrictSyncStackParentPreservesRecordedParentBase(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{
		configValues: map[string]string{
			strictSyncStackReviewBaseKey("feature/parent"): "feature/grandparent",
		},
		revListOutputs: map[string]string{
			"rev-list --count origin/feature/grandparent..feature/parent":      "1\n",
			"rev-list --count feature/parent..origin/feature/parent":           "0\n",
			"rev-list --count feature/grandparent..origin/feature/grandparent": "0\n",
		},
	}
	githubExecutor := &strictSyncGitHubExecutor{
		output: "https://github.com/owner/project/pull/8\n",
		outputs: []string{
			`[]`,
			`[]`,
			`[{"number":7,"title":"Grandparent","headRefName":"feature/grandparent","baseRefName":"master"}]`,
		},
	}
	environment, repository := strictSyncStackTestContext(t, gitExecutor, githubExecutor)

	parentErr := ensureStrictSyncStackParent(context.Background(), environment, repository, strictSyncStackParentOptions{
		RemoteName:           shared.OriginRemoteNameConstant,
		ConfiguredBaseBranch: defaultSyncBaseBranch,
		Plan: strictSyncStackPlan{
			ChildBranch:  "feature/child",
			ParentBranch: "feature/parent",
		},
		CommitMessages: worktreeAdoptionCommitMessageOptions{
			Client: &strictSyncChatClient{response: "Preserve the recorded review stack."},
		},
	})

	require.NoError(t, parentErr)
	recordedCommands := recordedGitCommands(gitExecutor.commands)
	require.Contains(t, recordedCommands, "push -u origin feature/grandparent")
	require.Contains(t, recordedCommands, "push -u origin feature/parent")
	require.Contains(t, recordedCommands, "diff --stat origin/feature/grandparent...feature/parent")
	require.Len(t, githubExecutor.commands, 4)
	createCommand := strings.Join(githubExecutor.commands[3].Arguments, " ")
	require.Contains(t, createCommand, "--base feature/grandparent")
	require.Contains(t, createCommand, "--head feature/parent")
	require.NotContains(t, createCommand, "--base master")
}

func TestEnsureStrictSyncStackParentRejectsRecordedReviewBaseCycle(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{
		configValues: map[string]string{
			strictSyncStackReviewBaseKey("feature/parent"):      "feature/grandparent",
			strictSyncStackReviewBaseKey("feature/grandparent"): "feature/parent",
		},
		revListOutputs: map[string]string{
			"rev-list --count origin/feature/grandparent..feature/parent":      "1\n",
			"rev-list --count feature/parent..origin/feature/parent":           "0\n",
			"rev-list --count origin/feature/parent..feature/grandparent":      "1\n",
			"rev-list --count feature/grandparent..origin/feature/grandparent": "0\n",
		},
	}
	githubExecutor := &strictSyncGitHubExecutor{output: `[]`}
	environment, repository := strictSyncStackTestContext(t, gitExecutor, githubExecutor)

	parentErr := ensureStrictSyncStackParent(context.Background(), environment, repository, strictSyncStackParentOptions{
		RemoteName:           shared.OriginRemoteNameConstant,
		ConfiguredBaseBranch: defaultSyncBaseBranch,
		Plan: strictSyncStackPlan{
			ChildBranch:  "feature/child",
			ParentBranch: "feature/parent",
		},
	})

	require.EqualError(t, parentErr, `cannot resolve stacked pull-request chain: review base cycle at branch "feature/parent"`)
	require.NotContains(t, recordedGitCommands(gitExecutor.commands), "push -u origin")
}

func TestEnsureStrictSyncStackParentRejectsInvalidReviewState(t *testing.T) {
	testCases := []struct {
		name          string
		githubOutputs []string
		revListOutput string
		expectedError string
	}{
		{
			name:          "open pull request omits base branch",
			githubOutputs: []string{`[{"number":7,"title":"Parent","headRefName":"feature/parent","baseRefName":""}]`},
			expectedError: `open pull request for branch "feature/parent" did not report a base branch`,
		},
		{
			name: "parent pull request is merged",
			githubOutputs: []string{
				`[]`,
				`[{"number":7,"title":"Parent","headRefName":"feature/parent","baseRefName":"master"}]`,
			},
			expectedError: `cannot create stacked branch "feature/child" from "feature/parent": the parent branch pull request is already merged`,
		},
		{
			name:          "remote parent is ahead",
			githubOutputs: []string{`[{"number":7,"title":"Parent","headRefName":"feature/parent","baseRefName":"master"}]`},
			revListOutput: "1\n",
			expectedError: `cannot create stacked branch "feature/child" from "feature/parent": the parent branch is behind origin/feature/parent and must be synced first`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			gitExecutor := &strictSyncGitExecutor{revListOutput: testCase.revListOutput}
			githubExecutor := &strictSyncGitHubExecutor{outputs: testCase.githubOutputs}
			environment, repository := strictSyncStackTestContext(t, gitExecutor, githubExecutor)

			parentErr := ensureStrictSyncStackParent(context.Background(), environment, repository, strictSyncStackParentOptions{
				RemoteName:           shared.OriginRemoteNameConstant,
				ConfiguredBaseBranch: defaultSyncBaseBranch,
				Plan: strictSyncStackPlan{
					ChildBranch:  "feature/child",
					ParentBranch: "feature/parent",
				},
			})

			require.EqualError(t, parentErr, testCase.expectedError)
			require.NotContains(t, recordedGitCommands(gitExecutor.commands), "push -u origin feature/parent")
			require.NotContains(t, recordedGitCommands(gitExecutor.commands), "switch -c feature/child")
		})
	}
}

func TestHandleStrictSyncRejectsConflictsBeforePublishingStackParent(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{
		statusOutput:      "UU README.md\n",
		revListOutput:     "1\n",
		missingReferences: missingStrictSyncStackChildReferences(),
	}
	repositoryManager, repositoryManagerErr := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, repositoryManagerErr)
	githubExecutor := &strictSyncGitHubExecutor{output: `[]`}
	githubClient, githubClientErr := githubcli.NewClient(githubExecutor)
	require.NoError(t, githubClientErr)
	environment := &workflow.Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: repositoryManager,
		GitHubClient:      githubClient,
		Logger:            zap.NewNop(),
		Output:            io.Discard,
		Errors:            io.Discard,
		Reporter:          &recordingReporter{},
	}
	repository := &workflow.RepositoryState{
		Path: "/tmp/project",
		Inspection: audit.RepositoryInspection{
			LocalBranch:    "feature/parent",
			FinalOwnerRepo: "owner/project",
		},
	}

	syncErr := handleBranchSyncAction(context.Background(), environment, repository, map[string]any{
		taskOptionBranchName:         "feature/child",
		taskOptionBranchRemote:       shared.OriginRemoteNameConstant,
		taskOptionRequirePullRequest: true,
		taskOptionBaseBranch:         defaultSyncBaseBranch,
		taskOptionCommitChanges:      true,
		taskOptionWorktreeCommitMessage: worktreeAdoptionCommitMessageOptions{
			Client: &strictSyncChatClient{response: "fix: resolve conflict"},
		},
	})

	require.EqualError(t, syncErr, strictSyncConflictWorktreeMessage)
	require.Empty(t, githubExecutor.commands)
	recordedCommands := recordedGitCommands(gitExecutor.commands)
	require.NotContains(t, recordedCommands, "push -u origin feature/parent")
	require.NotContains(t, recordedCommands, "switch -c feature/child")
	require.NotContains(t, recordedCommands, strictSyncStackReviewBaseKey("feature/child"))
}

func TestHandleStrictSyncRemovesReviewBaseWhenChildCreationFails(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{
		statusOutput:      " M README.md\n",
		missingReferences: missingStrictSyncStackChildReferences(),
		commandErrors: map[string]error{
			"switch -c feature/child": commandFailedError("cannot create child branch"),
		},
	}
	repositoryManager, repositoryManagerErr := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, repositoryManagerErr)
	environment := &workflow.Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: repositoryManager,
		Logger:            zap.NewNop(),
		Output:            io.Discard,
		Errors:            io.Discard,
		Reporter:          &recordingReporter{},
	}
	repository := &workflow.RepositoryState{
		Path: "/tmp/project",
		Inspection: audit.RepositoryInspection{
			LocalBranch: "master",
		},
	}

	syncErr := handleBranchSyncAction(context.Background(), environment, repository, map[string]any{
		taskOptionBranchName:         "feature/child",
		taskOptionBranchRemote:       shared.OriginRemoteNameConstant,
		taskOptionRequirePullRequest: true,
		taskOptionBaseBranch:         defaultSyncBaseBranch,
		taskOptionCommitChanges:      true,
		taskOptionWorktreeCommitMessage: worktreeAdoptionCommitMessageOptions{
			Client: &strictSyncChatClient{response: "feat: create child"},
		},
	})

	require.ErrorContains(t, syncErr, "cannot create child branch")
	_, reviewBaseExists := gitExecutor.configValues[strictSyncStackReviewBaseKey("feature/child")]
	require.False(t, reviewBaseExists)
	recordedCommands := recordedGitCommands(gitExecutor.commands)
	require.Contains(t, recordedCommands, "config "+strictSyncStackReviewBaseKey("feature/child")+" master")
	require.Contains(t, recordedCommands, "config --unset "+strictSyncStackReviewBaseKey("feature/child"))
}

func missingStrictSyncStackChildReferences() map[string]bool {
	return map[string]bool{
		"origin/feature/child":     true,
		"refs/heads/feature/child": true,
	}
}

func strictSyncStackTestContext(t *testing.T, gitExecutor *strictSyncGitExecutor, githubExecutor *strictSyncGitHubExecutor) (*workflow.Environment, *workflow.RepositoryState) {
	t.Helper()
	githubClient, githubClientErr := githubcli.NewClient(githubExecutor)
	require.NoError(t, githubClientErr)
	return &workflow.Environment{
			GitExecutor:  gitExecutor,
			GitHubClient: githubClient,
		}, &workflow.RepositoryState{
			Path: "/tmp/project",
			Inspection: audit.RepositoryInspection{
				LocalBranch:    "feature/parent",
				FinalOwnerRepo: "owner/project",
			},
		}
}

func githubCommandOption(arguments []string, option string) string {
	for argumentIndex := 0; argumentIndex+1 < len(arguments); argumentIndex++ {
		if arguments[argumentIndex] == option {
			return arguments[argumentIndex+1]
		}
	}
	return ""
}
