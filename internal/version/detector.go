package version

import (
	"context"
	"errors"
	"os"
	"runtime/debug"
	"strings"

	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
)

const (
	unknownVersionFallbackConstant            = "unknown"
	buildInfoDevelVersionValue                = "devel"
	gitRevParseSubcommandConstant             = "rev-parse"
	gitShowTopLevelFlagConstant               = "--show-toplevel"
	gitDescribeSubcommandConstant             = "describe"
	gitTagsFlagConstant                       = "--tags"
	gitExactMatchFlagConstant                 = "--exact-match"
	gitLongFlagConstant                       = "--long"
	gitDirtyFlagConstant                      = "--dirty"
	gitTerminalPromptEnvironmentNameConstant  = "GIT_TERMINAL_PROMPT"
	gitTerminalPromptEnvironmentValueConstant = "0"
	gitExecutorMissingMessageConstant         = "git executor not configured"
)

// BuildInfoProvider exposes runtime build metadata.
type BuildInfoProvider interface {
	Read() (*debug.BuildInfo, bool)
}

// Detector resolves application version strings.
type Detector struct {
	buildInfoProvider BuildInfoProvider
	gitExecutor       shared.GitExecutor
	workingDirectory  string
}

// Dependencies describes the collaborators required for version detection.
type Dependencies struct {
	BuildInfoProvider BuildInfoProvider
	GitExecutor       shared.GitExecutor
	WorkingDirectory  string
}

// NewDetector constructs a Detector with the supplied dependencies or sensible defaults.
func NewDetector(dependencies Dependencies) (*Detector, error) {
	provider := dependencies.BuildInfoProvider
	if provider == nil {
		provider = runtimeBuildInfoProvider{}
	}

	executor := dependencies.GitExecutor
	if executor == nil {
		shellExecutor, creationError := defaultGitExecutor()
		if creationError != nil {
			return nil, creationError
		}
		executor = shellExecutor
	}

	workingDirectory := strings.TrimSpace(dependencies.WorkingDirectory)
	if len(workingDirectory) == 0 {
		currentDirectory, workingDirectoryError := os.Getwd()
		if workingDirectoryError == nil {
			workingDirectory = currentDirectory
		}
	}

	return &Detector{
		buildInfoProvider: provider,
		gitExecutor:       executor,
		workingDirectory:  workingDirectory,
	}, nil
}

// Detect resolves the application version using the supplied dependencies.
func Detect(executionContext context.Context, dependencies Dependencies) string {
	detector, detectorError := NewDetector(dependencies)
	if detectorError != nil {
		return unknownVersionFallbackConstant
	}
	return detector.Version(executionContext)
}

// Version returns the detected application version string.
func (detector *Detector) Version(executionContext context.Context) string {
	if detector == nil {
		return unknownVersionFallbackConstant
	}

	if buildVersion := detector.versionFromBuildInfo(); len(buildVersion) > 0 {
		return buildVersion
	}

	repositoryRoot := detector.resolveRepositoryRoot(executionContext)

	if exactVersion := detector.describeVersion(executionContext, repositoryRoot, []string{gitDescribeSubcommandConstant, gitTagsFlagConstant, gitExactMatchFlagConstant}); len(exactVersion) > 0 {
		return exactVersion
	}

	if longVersion := detector.describeVersion(executionContext, repositoryRoot, []string{gitDescribeSubcommandConstant, gitTagsFlagConstant, gitLongFlagConstant, gitDirtyFlagConstant}); len(longVersion) > 0 {
		return longVersion
	}

	return unknownVersionFallbackConstant
}

func (detector *Detector) versionFromBuildInfo() string {
	if detector.buildInfoProvider == nil {
		return ""
	}

	buildInfo, available := detector.buildInfoProvider.Read()
	if !available || buildInfo == nil {
		return ""
	}

	trimmedVersion := strings.TrimSpace(buildInfo.Main.Version)
	if len(trimmedVersion) == 0 {
		return ""
	}

	if strings.EqualFold(trimmedVersion, buildInfoDevelVersionValue) {
		return ""
	}

	return trimmedVersion
}

func (detector *Detector) resolveRepositoryRoot(executionContext context.Context) string {
	if len(detector.workingDirectory) == 0 {
		return ""
	}

	executionResult, executionError := detector.executeGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{gitRevParseSubcommandConstant, gitShowTopLevelFlagConstant},
		WorkingDirectory: detector.workingDirectory,
	})
	if executionError != nil {
		return detector.workingDirectory
	}

	trimmedPath := strings.TrimSpace(executionResult.StandardOutput)
	if len(trimmedPath) == 0 {
		return detector.workingDirectory
	}

	return trimmedPath
}

func (detector *Detector) describeVersion(executionContext context.Context, repositoryRoot string, arguments []string) string {
	executionResult, executionError := detector.executeGit(executionContext, execshell.CommandDetails{
		Arguments:        arguments,
		WorkingDirectory: repositoryRoot,
	})
	if executionError != nil {
		return ""
	}

	return strings.TrimSpace(executionResult.StandardOutput)
}

func (detector *Detector) executeGit(executionContext context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	if detector.gitExecutor == nil {
		return execshell.ExecutionResult{}, errors.New(gitExecutorMissingMessageConstant)
	}

	environment := details.EnvironmentVariables
	if environment == nil {
		environment = map[string]string{}
	}
	environment[gitTerminalPromptEnvironmentNameConstant] = gitTerminalPromptEnvironmentValueConstant
	details.EnvironmentVariables = environment

	return detector.gitExecutor.ExecuteGit(executionContext, details)
}

type runtimeBuildInfoProvider struct{}

func (runtimeBuildInfoProvider) Read() (*debug.BuildInfo, bool) {
	return debug.ReadBuildInfo()
}

func defaultGitExecutor() (shared.GitExecutor, error) {
	shellExecutor, creationError := execshell.NewShellExecutor(zap.NewNop(), execshell.NewOSCommandRunner(), false)
	if creationError != nil {
		return nil, creationError
	}
	return shellExecutor, nil
}
