package syncflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/execshell"
)

type mergeConflictIndexCheckExecutor struct {
	commands          []execshell.CommandDetails
	conflictsResolved bool
}

func (executor *mergeConflictIndexCheckExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.commands = append(executor.commands, details)
	command := strings.Join(details.Arguments, " ")
	switch command {
	case "diff --name-only --diff-filter=U":
		if executor.conflictsResolved {
			return execshell.ExecutionResult{}, nil
		}
		return execshell.ExecutionResult{StandardOutput: ".mprlab/ISSUES.md\n"}, nil
	case "ls-files -u -- .mprlab/ISSUES.md":
		return execshell.ExecutionResult{
			StandardOutput: "100644 aaaaaaa 1\t.mprlab/ISSUES.md\n100644 bbbbbbb 2\t.mprlab/ISSUES.md\n100644 ccccccc 3\t.mprlab/ISSUES.md\n",
		}, nil
	case "show :1:.mprlab/ISSUES.md":
		return execshell.ExecutionResult{StandardOutput: "stable prefix\nstable suffix\n"}, nil
	case "show :2:.mprlab/ISSUES.md":
		return execshell.ExecutionResult{StandardOutput: "stable prefix\nlocal insertion\nstable suffix\n"}, nil
	case "show :3:.mprlab/ISSUES.md":
		return execshell.ExecutionResult{StandardOutput: "stable prefix\nincoming insertion\nstable suffix\n"}, nil
	case "checkout --conflict=diff3 -- .mprlab/ISSUES.md":
		return execshell.ExecutionResult{}, nil
	case "add -- .mprlab/ISSUES.md":
		executor.conflictsResolved = true
		return execshell.ExecutionResult{}, nil
	case "rev-parse --verify --end-of-options MERGE_HEAD^{commit}":
		return execshell.ExecutionResult{StandardOutput: "dddddddddddddddddddddddddddddddddddddddd\n"}, nil
	case "diff --cached --check":
		return execshell.ExecutionResult{}, errors.New("trailing whitespace in staged resolution")
	case "diff --cached --name-only --no-renames -z --":
		return execshell.ExecutionResult{StandardOutput: ".mprlab/ISSUES.md\x00"}, nil
	case "diff --cached --check -- .mprlab/ISSUES.md":
		return execshell.ExecutionResult{}, errors.New("trailing whitespace in staged resolution")
	case "diff --cached --check dddddddddddddddddddddddddddddddddddddddd -- .mprlab/ISSUES.md":
		return execshell.ExecutionResult{}, errors.New("trailing whitespace added by merge resolution")
	case "commit --no-edit":
		return execshell.ExecutionResult{}, errors.New("commit must not run after failed cached diff validation")
	default:
		return execshell.ExecutionResult{}, nil
	}
}

func (executor *mergeConflictIndexCheckExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func TestParseMergeConflictDocumentReconstructsMultipleDiff3Regions(t *testing.T) {
	content := "stable prefix\n" +
		"<<<<<<< HEAD\n" +
		"ours first\n" +
		"||||||| parent\n" +
		"base first\n" +
		"=======\n" +
		"theirs first\n" +
		">>>>>>> origin/master\n" +
		"stable middle\n" +
		"<<<<<<< HEAD\n" +
		"ours insertion\n" +
		"||||||| parent\n" +
		"=======\n" +
		"theirs insertion\n" +
		">>>>>>> origin/master\n" +
		"stable suffix\n"

	document, parseErr := parseMergeConflictDocument(content)

	require.NoError(t, parseErr)
	require.Equal(t, []string{"stable prefix\n", "stable middle\n", "stable suffix\n"}, document.NonConflictingRegions)
	require.Equal(
		t,
		[]mergeConflictRegion{
			{
				Ours:        "ours first\n",
				Base:        "base first\n",
				BasePresent: true,
				Theirs:      "theirs first\n",
			},
			{
				Ours:        "ours insertion\n",
				BasePresent: true,
				Theirs:      "theirs insertion\n",
			},
		},
		document.ConflictRegions,
	)
	require.Equal(
		t,
		"stable prefix\nresolved first\nstable middle\nours insertion\ntheirs insertion\nstable suffix\n",
		document.resolve([]string{"resolved first\n", "ours insertion\ntheirs insertion\n"}),
	)
}

func TestParseMergeConflictDocumentRejectsInvalidMarkerStructures(t *testing.T) {
	testCases := map[string]string{
		"unexpected marker": "stable\n=======\nstable\n",
		"invalid ours":      "<<<<<<< HEAD\nours\n<<<<<<< nested\n=======\ntheirs\n>>>>>>> source\n",
		"invalid base":      "<<<<<<< HEAD\nours\n||||||| base\n>>>>>>> source\n=======\ntheirs\n>>>>>>> source\n",
		"invalid theirs":    "<<<<<<< HEAD\nours\n=======\ntheirs\n=======\n>>>>>>> source\n",
		"unterminated":      "<<<<<<< HEAD\nours\n=======\ntheirs\n",
	}

	for testName, content := range testCases {
		t.Run(testName, func(t *testing.T) {
			_, parseErr := parseMergeConflictDocument(content)
			require.Error(t, parseErr)
		})
	}
}

func TestResolveRejectsCachedDiffCheckBeforeMergeCommit(t *testing.T) {
	repositoryPath := t.TempDir()
	issuesDirectory := filepath.Join(repositoryPath, ".mprlab")
	require.NoError(t, os.MkdirAll(issuesDirectory, 0o755))
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(issuesDirectory, "ISSUES.md"),
			[]byte(
				"stable prefix\n"+
					"<<<<<<< HEAD\n"+
					"local insertion\n"+
					"||||||| parent\n"+
					"=======\n"+
					"incoming insertion\n"+
					">>>>>>> origin/master\n"+
					"stable suffix\n",
			),
			0o644,
		),
	)

	executor := &mergeConflictIndexCheckExecutor{}
	service := mergeConflictResolutionService{
		executor:       executor,
		repositoryPath: repositoryPath,
		commitMessages: worktreeAdoptionCommitMessageOptions{
			Client: &strictSyncChatClient{responses: []string{`{"status":"resolved","decisions":[{"id":"conflict-1","action":"ours","reason":"Selected source."}]}`, `{"status":"approved"}`}},
		},
	}

	conflictObserved, resolutionErr := service.Resolve(
		context.Background(),
		mergeConflictResolutionOptions{
			SourceReference: "origin/master",
			TargetBranch:    "feature/target",
		},
	)

	require.True(t, conflictObserved)
	require.Error(t, resolutionErr)
	require.Contains(t, resolutionErr.Error(), "validate resolved merge index")
	require.Contains(t, resolutionErr.Error(), "trailing whitespace")
	recordedCommands := make([]string, 0, len(executor.commands))
	for _, command := range executor.commands {
		recordedCommands = append(recordedCommands, strings.Join(command.Arguments, " "))
	}
	require.Contains(t, recordedCommands, "diff --cached --check")
	require.Contains(t, recordedCommands, "rev-parse --verify --end-of-options MERGE_HEAD^{commit}")
	require.Contains(t, recordedCommands, "diff --cached --check dddddddddddddddddddddddddddddddddddddddd -- .mprlab/ISSUES.md")
	require.NotContains(t, recordedCommands, "diff --cached --check origin/master -- .mprlab/ISSUES.md")
	require.NotContains(t, recordedCommands, "commit --no-edit")
}
