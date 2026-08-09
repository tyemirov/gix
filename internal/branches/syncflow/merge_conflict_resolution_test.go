package syncflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/v5/internal/execshell"
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
	case "diff --cached --check":
		return execshell.ExecutionResult{}, errors.New("trailing whitespace in staged resolution")
	case "diff --cached --name-only --no-renames -z --":
		return execshell.ExecutionResult{StandardOutput: ".mprlab/ISSUES.md\x00"}, nil
	case "diff --cached --check -- .mprlab/ISSUES.md":
		return execshell.ExecutionResult{}, errors.New("trailing whitespace in staged resolution")
	case "diff --cached --check origin/master -- .mprlab/ISSUES.md":
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

func TestMergeConflictResolutionContentPreservesBoundaryWhitespace(t *testing.T) {
	expectedContent := "\nresolved region\n\n"
	response := "\n" +
		mergeConflictResolutionContentBegin + "\n" +
		expectedContent +
		"\n" + mergeConflictResolutionContentEnd +
		"\n"

	content, contentErr := mergeConflictResolutionContent("ISSUES.md", response)

	require.NoError(t, contentErr)
	require.Equal(t, expectedContent, content)
}

func TestMergeConflictResolutionContentRejectsMissingOrEmptyEnvelope(t *testing.T) {
	_, missingEnvelopeErr := mergeConflictResolutionContent("ISSUES.md", "resolved region")
	require.Error(t, missingEnvelopeErr)
	require.Contains(t, missingEnvelopeErr.Error(), "required content envelope")

	emptyResponse := mergeConflictResolutionContentBegin + "\n\n" + mergeConflictResolutionContentEnd
	_, emptyContentErr := mergeConflictResolutionContent("ISSUES.md", emptyResponse)
	require.Error(t, emptyContentErr)
	require.Contains(t, emptyContentErr.Error(), "empty merge resolution")
}

func TestValidateMergeConflictRegionResponseContracts(t *testing.T) {
	additiveRegion := mergeConflictRegion{
		Ours:        "ours insertion\n",
		BasePresent: true,
		Theirs:      "theirs insertion\n",
	}
	require.NoError(t, validateMergeConflictRegionResponse("ISSUES.md", 0, additiveRegion, additiveRegion.Ours+additiveRegion.Theirs))
	require.NoError(t, validateMergeConflictRegionResponse("ISSUES.md", 0, additiveRegion, additiveRegion.Theirs+additiveRegion.Ours))

	additiveLossErr := validateMergeConflictRegionResponse("ISSUES.md", 0, additiveRegion, additiveRegion.Ours)
	require.Error(t, additiveLossErr)
	require.Contains(t, additiveLossErr.Error(), "not an exact ordering of OURS and THEIRS")

	nonAdditiveRegion := mergeConflictRegion{
		Ours:        "reviewers: alice, bob\n",
		Base:        "reviewers: alice\n",
		BasePresent: true,
		Theirs:      "reviewers: alice, carol\n",
	}
	require.NoError(t, validateMergeConflictRegionResponse("ISSUES.md", 1, nonAdditiveRegion, "reviewers: alice, bob, carol\n"))

	oursLossErr := validateMergeConflictRegionResponse("ISSUES.md", 1, nonAdditiveRegion, nonAdditiveRegion.Theirs)
	require.Error(t, oursLossErr)
	require.Contains(t, oursLossErr.Error(), "does not preserve OURS replacement intent")

	theirsLossErr := validateMergeConflictRegionResponse("ISSUES.md", 1, nonAdditiveRegion, nonAdditiveRegion.Ours)
	require.Error(t, theirsLossErr)
	require.Contains(t, theirsLossErr.Error(), "does not preserve THEIRS replacement intent")
}

func TestDeterministicMergeConflictRegionResolutionBuildsAuthoritativeResultsAndAuditedCandidates(t *testing.T) {
	testCases := map[string]struct {
		region                mergeConflictRegion
		expectedContent       string
		expectedStrategy      string
		requiresSemanticAudit bool
	}{
		"identical sides": {
			region: mergeConflictRegion{
				Ours:        "same\n",
				Base:        "base\n",
				BasePresent: true,
				Theirs:      "same\n",
			},
			expectedContent:  "same\n",
			expectedStrategy: "identical sides",
		},
		"ours unchanged": {
			region: mergeConflictRegion{
				Ours:        "base\n",
				Base:        "base\n",
				BasePresent: true,
				Theirs:      "incoming\n",
			},
			expectedContent:  "incoming\n",
			expectedStrategy: "incoming-only change",
		},
		"theirs unchanged": {
			region: mergeConflictRegion{
				Ours:        "local\n",
				Base:        "base\n",
				BasePresent: true,
				Theirs:      "base\n",
			},
			expectedContent:  "local\n",
			expectedStrategy: "local-only change",
		},
		"concurrent insertions require audit": {
			region: mergeConflictRegion{
				Ours:        "local insertion\n",
				BasePresent: true,
				Theirs:      "incoming insertion\n",
			},
			expectedContent:       "local insertion\nincoming insertion\n",
			expectedStrategy:      "concurrent insertions",
			requiresSemanticAudit: true,
		},
		"non-overlapping token edits": {
			region: mergeConflictRegion{
				Ours:        "policy: strict timeout=30\n",
				Base:        "policy: standard timeout=30\n",
				BasePresent: true,
				Theirs:      "policy: standard timeout=60\n",
			},
			expectedContent:       "policy: strict timeout=60\n",
			expectedStrategy:      "non-overlapping token edits",
			requiresSemanticAudit: true,
		},
	}

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			resolution, resolved := deterministicMergeConflictRegionResolution(testCase.region)

			require.True(t, resolved)
			require.Equal(t, testCase.expectedContent, resolution.Content)
			require.Equal(t, testCase.expectedStrategy, resolution.Strategy)
			require.Equal(t, testCase.requiresSemanticAudit, resolution.RequiresSemanticAudit)
		})
	}
}

func TestDeterministicMergeConflictRegionResolutionDefersOverlappingTokenInsertions(t *testing.T) {
	region := mergeConflictRegion{
		Ours:        "reviewers: alice, bob\n",
		Base:        "reviewers: alice\n",
		BasePresent: true,
		Theirs:      "reviewers: alice, carol\n",
	}

	_, resolved := deterministicMergeConflictRegionResolution(region)

	require.False(t, resolved)
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
			Client: &strictSyncChatClient{response: mergeConflictResolutionReviewApproved},
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
	require.Contains(t, recordedCommands, "diff --cached --check origin/master -- .mprlab/ISSUES.md")
	require.NotContains(t, recordedCommands, "commit --no-edit")
}
