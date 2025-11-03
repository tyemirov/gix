package workflow

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/temirov/gix/internal/audit"
	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/gitrepo"
	repoerrors "github.com/temirov/gix/internal/repos/errors"
	"github.com/temirov/gix/internal/repos/filesystem"
	"github.com/temirov/gix/internal/repos/shared"
)

const namespaceTestCommitMessage = "chore: rewrite namespace"

func TestHandleNamespaceRewriteActionDryRun(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".git"), 0o755))

	goMod := "module github.com/old/org/app\n\ngo 1.22\nrequire github.com/old/org/dep v1.0.0\n"
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goMod), 0o644))

	source := `package main
import "github.com/old/org/dep"
func main() { dep.Do() }
`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.go"), []byte(source), 0o644))

	executor := &namespaceTestGitExecutor{}
	manager, err := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, err)

	output := &bytes.Buffer{}
	environment := &Environment{
		FileSystem:        filesystem.OSFileSystem{},
		GitExecutor:       executor,
		RepositoryManager: manager,
		Output:            output,
		Reporter:          shared.NewStructuredReporter(output, output, shared.WithRepositoryHeaders(false)),
		DryRun:            true,
	}

	repository := &RepositoryState{Path: tempDir}
	parameters := map[string]any{
		"old": "github.com/old/org",
		"new": "github.com/new/org",
	}

	err = handleNamespaceRewriteAction(context.Background(), environment, repository, parameters)
	require.NoError(t, err)
	events := parseStructuredEvents(output.String())
	require.Len(t, events, 1)
	planEvent := requireEventByCode(t, events, shared.EventCodeNamespacePlan)
	require.Equal(t, repository.Path, planEvent["path"])
	require.Equal(t, "true", planEvent["push"])
	_, hasBranch := planEvent["branch"]
	require.True(t, hasBranch)

	content, readErr := os.ReadFile(filepath.Join(tempDir, "main.go"))
	require.NoError(t, readErr)
	require.Contains(t, string(content), "github.com/old/org/dep")
}

func TestHandleNamespaceRewriteActionApply(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".git"), 0o755))

	goMod := "module github.com/old/org/app\n\ngo 1.22\nrequire github.com/old/org/dep v1.0.0\n"
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goMod), 0o644))

	source := `package main
import "github.com/old/org/dep"
func main() { dep.Do() }
`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.go"), []byte(source), 0o644))

	executor := &namespaceTestGitExecutor{
		configValues: map[string]string{
			"user.name":         "Test",
			"user.email":        "test@example.com",
			"remote.origin.url": "git@example.com:old/org.git",
		},
	}
	manager, err := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, err)

	output := &bytes.Buffer{}
	environment := &Environment{
		FileSystem:        filesystem.OSFileSystem{},
		GitExecutor:       executor,
		RepositoryManager: manager,
		Output:            output,
		Reporter:          shared.NewStructuredReporter(output, output, shared.WithRepositoryHeaders(false)),
		PromptState:       NewPromptState(true),
	}

	repository := &RepositoryState{Path: tempDir}
	parameters := map[string]any{
		"old":                          "github.com/old/org",
		"new":                          "github.com/new/org",
		"branch_prefix":                "rewrite",
		"remote":                       "origin",
		namespaceCommitMessageFlagName: namespaceTestCommitMessage,
	}

	err = handleNamespaceRewriteAction(context.Background(), environment, repository, parameters)
	require.NoError(t, err)
	events := parseStructuredEvents(output.String())
	require.Len(t, events, 1)
	applyEvent := requireEventByCode(t, events, shared.EventCodeNamespaceApply)
	require.Equal(t, repository.Path, applyEvent["path"])
	require.Equal(t, "true", applyEvent["push"])
	_, hasBranch := applyEvent["branch"]
	require.True(t, hasBranch)

	updatedSource, readErr := os.ReadFile(filepath.Join(tempDir, "main.go"))
	require.NoError(t, readErr)
	require.False(t, strings.Contains(string(updatedSource), "github.com/old/org"))
	require.True(t, strings.Contains(string(updatedSource), "github.com/new/org"))

	joinedCommands := strings.Join(executor.recorded(), "\n")
	require.Contains(t, joinedCommands, "checkout -b rewrite/")
	require.Contains(t, joinedCommands, "push --set-upstream origin")

	commitCommandFound := false
	for _, details := range executor.commands {
		if len(details.Arguments) == 0 {
			continue
		}
		if details.Arguments[0] != "commit" {
			continue
		}
		commitCommandFound = true
		require.Contains(t, details.Arguments, "-m")
		require.Contains(t, details.Arguments, namespaceTestCommitMessage)
	}
	require.True(t, commitCommandFound)
}

func TestHandleNamespaceRewriteActionPushFailure(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".git"), 0o755))

	goMod := "module github.com/old/org/app\n\ngo 1.22\nrequire github.com/old/org/dep v1.0.0\n"
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goMod), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main\n"), 0o644))

	pushFailure := execshell.CommandFailedError{
		Command: execshell.ShellCommand{Name: execshell.CommandGit},
		Result:  execshell.ExecutionResult{ExitCode: 1, StandardError: "authentication required"},
	}

	executor := &namespaceTestGitExecutor{
		configValues: map[string]string{
			"user.name":         "Test",
			"user.email":        "test@example.com",
			"remote.origin.url": "git@example.com:old/org.git",
		},
		pushError: pushFailure,
	}
	manager, err := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, err)

	output := &bytes.Buffer{}
	errorOutput := &bytes.Buffer{}
	environment := &Environment{
		FileSystem:        filesystem.OSFileSystem{},
		GitExecutor:       executor,
		RepositoryManager: manager,
		Output:            output,
		Errors:            errorOutput,
		Reporter:          shared.NewStructuredReporter(output, errorOutput, shared.WithRepositoryHeaders(false)),
		PromptState:       NewPromptState(true),
	}

	repository := &RepositoryState{Path: tempDir, Inspection: audit.RepositoryInspection{FinalOwnerRepo: "owner/repo"}}
	parameters := map[string]any{
		"old":                          "github.com/old/org",
		"new":                          "github.com/new/org",
		"remote":                       "origin",
		"branch_prefix":                "rewrite",
		namespaceCommitMessageFlagName: namespaceTestCommitMessage,
	}

	err = handleNamespaceRewriteAction(context.Background(), environment, repository, parameters)
	require.NoError(t, err)

	events := parseStructuredEvents(output.String())
	require.Len(t, events, 2)
	applyEvent := requireEventByCode(t, events, shared.EventCodeNamespaceApply)
	require.Equal(t, repository.Path, applyEvent["path"])
	require.Equal(t, "false", applyEvent["push"])
	skipEvent := requireEventByCode(t, events, shared.EventCodeNamespaceSkip)
	require.NotEmpty(t, skipEvent["reason"])

	errorEvents := parseStructuredEvents(errorOutput.String())
	require.NotEmpty(t, errorEvents)
	require.Contains(t, errorOutput.String(), string(repoerrors.ErrNamespacePushFailed))
}

func TestHandleNamespaceRewriteActionSkipsWhenRemoteUpToDate(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module github.com/old/org/app\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main\n"), 0o644))

	executor := &namespaceTestGitExecutor{
		configValues: map[string]string{
			"remote.origin.url": "git@example.com:old/org.git",
		},
		headHash:     "abcdef1234567890",
		mirrorRemote: true,
	}
	manager, err := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, err)

	output := &bytes.Buffer{}
	environment := &Environment{
		FileSystem:        filesystem.OSFileSystem{},
		GitExecutor:       executor,
		RepositoryManager: manager,
		Output:            output,
		Reporter:          shared.NewStructuredReporter(output, output, shared.WithRepositoryHeaders(false)),
		PromptState:       NewPromptState(true),
	}

	repository := &RepositoryState{Path: tempDir}
	parameters := map[string]any{
		"old":                          "github.com/old/org",
		"new":                          "github.com/new/org",
		"remote":                       "origin",
		namespaceCommitMessageFlagName: namespaceTestCommitMessage,
	}

	err = handleNamespaceRewriteAction(context.Background(), environment, repository, parameters)
	require.NoError(t, err)

	events := parseStructuredEvents(output.String())
	require.Len(t, events, 2)
	applyEvent := requireEventByCode(t, events, shared.EventCodeNamespaceApply)
	require.Equal(t, repository.Path, applyEvent["path"])
	require.Equal(t, "false", applyEvent["push"])
	skipEvent := requireEventByCode(t, events, shared.EventCodeNamespaceSkip)
	require.Contains(t, skipEvent["reason"], "already")
}

func TestHandleNamespaceRewriteActionRespectsSafeguards(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".git"), 0o755))

	goMod := "module github.com/old/org/app\n\ngo 1.22\nrequire github.com/old/org/dep v1.0.0\n"
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goMod), 0o644))

	source := `package main
import "github.com/old/org/dep"
func main() { dep.Do() }
`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.go"), []byte(source), 0o644))

	executor := &namespaceTestGitExecutor{statusOutput: " M main.go"}
	manager, err := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, err)
	output := &bytes.Buffer{}
	environment := &Environment{
		FileSystem:        filesystem.OSFileSystem{},
		GitExecutor:       executor,
		RepositoryManager: manager,
		Output:            output,
		Reporter:          shared.NewStructuredReporter(output, output, shared.WithRepositoryHeaders(false)),
	}

	repository := &RepositoryState{Path: tempDir}
	parameters := map[string]any{
		"old":        "github.com/old/org",
		"new":        "github.com/new/org",
		"safeguards": map[string]any{"require_clean": true},
	}

	err = handleNamespaceRewriteAction(context.Background(), environment, repository, parameters)
	require.NoError(t, err)

	updatedSource, readErr := os.ReadFile(filepath.Join(tempDir, "main.go"))
	require.NoError(t, readErr)
	require.Contains(t, string(updatedSource), "github.com/old/org/dep")

	events := parseStructuredEvents(output.String())
	require.Len(t, events, 1)
	skipEvent := requireEventByCode(t, events, shared.EventCodeNamespaceSkip)
	require.Equal(t, repository.Path, skipEvent["path"])
	require.NotEmpty(t, skipEvent["reason"])
	recorded := executor.recorded()
	require.GreaterOrEqual(t, len(recorded), 1)
	require.Equal(t, "status --porcelain", recorded[0])
}

type namespaceTestGitExecutor struct {
	commands     []execshell.CommandDetails
	staged       map[string]struct{}
	configValues map[string]string
	statusOutput string
	pushError    error
	remoteRefs   map[string]string
	headHash     string
	mirrorRemote bool
}

func (executor *namespaceTestGitExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	if executor.staged == nil {
		executor.staged = map[string]struct{}{}
	}
	executor.commands = append(executor.commands, details)

	args := details.Arguments
	if len(args) == 0 {
		return execshell.ExecutionResult{}, nil
	}

	switch args[0] {
	case "status":
		return execshell.ExecutionResult{StandardOutput: executor.statusOutput}, nil
	case "checkout":
		return execshell.ExecutionResult{}, nil
	case "check-ignore":
		if len(args) >= 2 && args[1] == "--stdin" {
			return execshell.ExecutionResult{}, execshell.CommandFailedError{Result: execshell.ExecutionResult{ExitCode: 1}}
		}
		return execshell.ExecutionResult{}, execshell.CommandFailedError{Result: execshell.ExecutionResult{ExitCode: 1}}
	case "add":
		if len(args) > 1 {
			executor.staged[args[1]] = struct{}{}
		}
		return execshell.ExecutionResult{}, nil
	case "diff":
		if len(executor.staged) == 0 {
			return execshell.ExecutionResult{}, nil
		}
		return execshell.ExecutionResult{}, execshell.CommandFailedError{Result: execshell.ExecutionResult{ExitCode: 1}}
	case "config":
		return executor.handleConfig(details)
	case "commit":
		executor.staged = map[string]struct{}{}
		return execshell.ExecutionResult{}, nil
	case "push":
		if executor.pushError != nil {
			return execshell.ExecutionResult{}, executor.pushError
		}
		return execshell.ExecutionResult{}, nil
	case "rev-parse":
		hash := executor.headHash
		if len(hash) == 0 {
			hash = "HEADHASH"
		}
		return execshell.ExecutionResult{StandardOutput: hash + "\n"}, nil
	case "ls-remote":
		if len(details.Arguments) >= 4 {
			if executor.mirrorRemote {
				hash := executor.headHash
				if len(hash) == 0 {
					hash = "HEADHASH"
				}
				output := fmt.Sprintf("%s\trefs/heads/%s\n", hash, details.Arguments[3])
				return execshell.ExecutionResult{StandardOutput: output}, nil
			}
			if executor.remoteRefs != nil {
				key := fmt.Sprintf("%s:%s", details.Arguments[2], details.Arguments[3])
				if hash, exists := executor.remoteRefs[key]; exists {
					output := fmt.Sprintf("%s\trefs/heads/%s\n", hash, details.Arguments[3])
					return execshell.ExecutionResult{StandardOutput: output}, nil
				}
			}
		}
		return execshell.ExecutionResult{StandardOutput: ""}, nil
	default:
		return execshell.ExecutionResult{}, nil
	}
}

func (executor *namespaceTestGitExecutor) ExecuteGitHubCLI(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.commands = append(executor.commands, details)
	return execshell.ExecutionResult{}, nil
}

func (executor *namespaceTestGitExecutor) handleConfig(details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	args := details.Arguments
	if len(args) >= 3 && args[1] == "--bool" {
		return execshell.ExecutionResult{StandardOutput: "false\n"}, nil
	}
	if len(args) >= 4 && args[1] == "--local" && args[2] == "--get" {
		if value := executor.configValues[args[3]]; value != "" {
			return execshell.ExecutionResult{StandardOutput: value + "\n"}, nil
		}
		return execshell.ExecutionResult{}, execshell.CommandFailedError{Result: execshell.ExecutionResult{ExitCode: 1}}
	}
	if len(args) >= 3 && args[1] == "--get" {
		if value := executor.configValues[args[2]]; value != "" {
			return execshell.ExecutionResult{StandardOutput: value + "\n"}, nil
		}
		return execshell.ExecutionResult{}, execshell.CommandFailedError{Result: execshell.ExecutionResult{ExitCode: 1}}
	}
	return execshell.ExecutionResult{}, nil
}

func (executor *namespaceTestGitExecutor) recorded() []string {
	results := make([]string, 0, len(executor.commands)*3)
	for _, details := range executor.commands {
		args := details.Arguments
		if len(args) == 0 {
			continue
		}

		results = append(results, strings.Join(args, " "))

		if len(args) >= 2 {
			results = append(results, strings.Join(args[:2], " "))
		}

		if len(args) >= 3 {
			prefix := args[2]
			if slash := strings.Index(prefix, "/"); slash >= 0 {
				prefix = prefix[:slash+1]
			}
			results = append(results, args[0]+" "+args[1]+" "+prefix)
		}
	}
	return results
}
