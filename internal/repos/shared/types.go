package shared

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/githubcli"
)

const (
	// OriginRemoteNameConstant identifies the default upstream remote used for GitHub repositories.
	OriginRemoteNameConstant = "origin"
	// GitProtocolURLPrefixConstant matches git protocol remote URLs.
	GitProtocolURLPrefixConstant = "git@github.com:"
	// SSHProtocolURLPrefixConstant matches ssh protocol remote URLs.
	SSHProtocolURLPrefixConstant = "ssh://git@github.com/"
	// HTTPSProtocolURLPrefixConstant matches https protocol remote URLs.
	HTTPSProtocolURLPrefixConstant = "https://github.com/"
)

var (
	ErrRepositoryPathInvalid   = errors.New("repository path invalid")
	ErrOwnerSlugInvalid        = errors.New("owner slug invalid")
	ErrRepositoryNameInvalid   = errors.New("repository name invalid")
	ErrOwnerRepositoryInvalid  = errors.New("owner repository invalid")
	ErrRemoteURLInvalid        = errors.New("remote URL invalid")
	ErrRemoteNameInvalid       = errors.New("remote name invalid")
	ErrBranchNameInvalid       = errors.New("branch name invalid")
	ErrRemoteProtocolInvalid   = errors.New("remote protocol invalid")
	ownerSlugPattern           = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	repositoryNamePattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	remoteNamePattern          = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	branchWhitespaceCharacters = " \t\n\r"
)

// RemoteProtocol enumerates supported git remote protocols.
type RemoteProtocol string

// Supported remote protocols.
const (
	RemoteProtocolGit   RemoteProtocol = "git"
	RemoteProtocolSSH   RemoteProtocol = "ssh"
	RemoteProtocolHTTPS RemoteProtocol = "https"
	RemoteProtocolOther RemoteProtocol = "other"
)

// ParseRemoteProtocol normalizes and validates protocol strings.
func ParseRemoteProtocol(rawValue string) (RemoteProtocol, error) {
	trimmed := strings.ToLower(strings.TrimSpace(rawValue))
	if len(trimmed) == 0 {
		return RemoteProtocolOther, nil
	}
	switch RemoteProtocol(trimmed) {
	case RemoteProtocolGit, RemoteProtocolSSH, RemoteProtocolHTTPS, RemoteProtocolOther:
		return RemoteProtocol(trimmed), nil
	default:
		return "", fmt.Errorf("%w: %s", ErrRemoteProtocolInvalid, rawValue)
	}
}

// Validate ensures the protocol is one of the supported constants.
func (protocol RemoteProtocol) Validate() error {
	switch protocol {
	case RemoteProtocolGit, RemoteProtocolSSH, RemoteProtocolHTTPS, RemoteProtocolOther:
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrRemoteProtocolInvalid, protocol)
	}
}

// String returns the protocol value.
func (protocol RemoteProtocol) String() string {
	return string(protocol)
}

// RepositoryPath represents an absolute filesystem location for a Git repository.
type RepositoryPath struct {
	value string
}

// NewRepositoryPath validates and normalizes repository paths.
func NewRepositoryPath(rawValue string) (RepositoryPath, error) {
	if strings.ContainsAny(rawValue, "\r\n") {
		return RepositoryPath{}, fmt.Errorf("%w: contains newline", ErrRepositoryPathInvalid)
	}
	trimmed := strings.TrimSpace(rawValue)
	if len(trimmed) == 0 {
		return RepositoryPath{}, fmt.Errorf("%w: empty", ErrRepositoryPathInvalid)
	}
	cleaned := filepath.Clean(trimmed)
	return RepositoryPath{value: cleaned}, nil
}

// String exposes the normalized path string.
func (path RepositoryPath) String() string {
	if len(path.value) == 0 {
		panic("shared.RepositoryPath: zero value")
	}
	return path.value
}

// OwnerSlug represents a GitHub owner segment (user or organization).
type OwnerSlug struct {
	value string
}

// NewOwnerSlug validates GitHub owner strings.
func NewOwnerSlug(rawValue string) (OwnerSlug, error) {
	trimmed := strings.TrimSpace(rawValue)
	if len(trimmed) == 0 {
		return OwnerSlug{}, fmt.Errorf("%w: empty", ErrOwnerSlugInvalid)
	}
	if strings.Contains(trimmed, "/") {
		return OwnerSlug{}, fmt.Errorf("%w: contains slash", ErrOwnerSlugInvalid)
	}
	if !ownerSlugPattern.MatchString(trimmed) {
		return OwnerSlug{}, fmt.Errorf("%w: %s", ErrOwnerSlugInvalid, trimmed)
	}
	return OwnerSlug{value: trimmed}, nil
}

// String returns the owner slug.
func (slug OwnerSlug) String() string {
	if len(slug.value) == 0 {
		panic("shared.OwnerSlug: zero value")
	}
	return slug.value
}

// RepositoryName models a GitHub repository name segment.
type RepositoryName struct {
	value string
}

// NewRepositoryName validates repository names.
func NewRepositoryName(rawValue string) (RepositoryName, error) {
	trimmed := strings.TrimSpace(rawValue)
	if len(trimmed) == 0 {
		return RepositoryName{}, fmt.Errorf("%w: empty", ErrRepositoryNameInvalid)
	}
	if strings.Contains(trimmed, "/") {
		return RepositoryName{}, fmt.Errorf("%w: contains slash", ErrRepositoryNameInvalid)
	}
	if !repositoryNamePattern.MatchString(trimmed) {
		return RepositoryName{}, fmt.Errorf("%w: %s", ErrRepositoryNameInvalid, trimmed)
	}
	return RepositoryName{value: trimmed}, nil
}

// String returns the repository name.
func (name RepositoryName) String() string {
	if len(name.value) == 0 {
		panic("shared.RepositoryName: zero value")
	}
	return name.value
}

// OwnerRepository represents the owner/repository tuple for a GitHub project.
type OwnerRepository struct {
	owner      OwnerSlug
	repository RepositoryName
}

// NewOwnerRepository parses an owner/repository tuple (e.g., "owner/repo").
func NewOwnerRepository(rawValue string) (OwnerRepository, error) {
	trimmed := strings.TrimSpace(rawValue)
	if len(trimmed) == 0 {
		return OwnerRepository{}, fmt.Errorf("%w: empty", ErrOwnerRepositoryInvalid)
	}

	segments := strings.Split(trimmed, "/")
	if len(segments) != 2 {
		return OwnerRepository{}, fmt.Errorf("%w: expected owner/repository", ErrOwnerRepositoryInvalid)
	}

	ownerSlug, ownerError := NewOwnerSlug(segments[0])
	if ownerError != nil {
		return OwnerRepository{}, fmt.Errorf("%w: owner invalid: %w", ErrOwnerRepositoryInvalid, ownerError)
	}

	repositoryName, repositoryError := NewRepositoryName(segments[1])
	if repositoryError != nil {
		return OwnerRepository{}, fmt.Errorf("%w: repository invalid: %w", ErrOwnerRepositoryInvalid, repositoryError)
	}

	return OwnerRepository{owner: ownerSlug, repository: repositoryName}, nil
}

// NewOwnerRepositoryFromParts constructs the tuple from validated segments.
func NewOwnerRepositoryFromParts(owner OwnerSlug, repository RepositoryName) OwnerRepository {
	return OwnerRepository{owner: owner, repository: repository}
}

// Owner returns the owner slug.
func (tuple OwnerRepository) Owner() OwnerSlug {
	if len(tuple.owner.value) == 0 || len(tuple.repository.value) == 0 {
		panic("shared.OwnerRepository: zero value")
	}
	return tuple.owner
}

// Repository returns the repository name.
func (tuple OwnerRepository) Repository() RepositoryName {
	if len(tuple.owner.value) == 0 || len(tuple.repository.value) == 0 {
		panic("shared.OwnerRepository: zero value")
	}
	return tuple.repository
}

// String returns the owner/repository tuple.
func (tuple OwnerRepository) String() string {
	return tuple.Owner().String() + "/" + tuple.Repository().String()
}

// RemoteURL models a canonical Git remote URL.
type RemoteURL struct {
	value string
}

// NewRemoteURL validates remote URLs for whitespace and emptiness.
func NewRemoteURL(rawValue string) (RemoteURL, error) {
	trimmed := strings.TrimSpace(rawValue)
	if len(trimmed) == 0 {
		return RemoteURL{}, fmt.Errorf("%w: empty", ErrRemoteURLInvalid)
	}
	if strings.ContainsAny(trimmed, branchWhitespaceCharacters) {
		return RemoteURL{}, fmt.Errorf("%w: contains whitespace", ErrRemoteURLInvalid)
	}
	return RemoteURL{value: trimmed}, nil
}

// String returns the remote URL.
func (remoteURL RemoteURL) String() string {
	if len(remoteURL.value) == 0 {
		panic("shared.RemoteURL: zero value")
	}
	return remoteURL.value
}

// RemoteName models named git remotes (origin, upstream, etc).
type RemoteName struct {
	value string
}

// NewRemoteName validates remote names.
func NewRemoteName(rawValue string) (RemoteName, error) {
	trimmed := strings.TrimSpace(rawValue)
	if len(trimmed) == 0 {
		return RemoteName{}, fmt.Errorf("%w: empty", ErrRemoteNameInvalid)
	}
	if !remoteNamePattern.MatchString(trimmed) {
		return RemoteName{}, fmt.Errorf("%w: %s", ErrRemoteNameInvalid, trimmed)
	}
	return RemoteName{value: trimmed}, nil
}

// String exposes the remote name value.
func (remoteName RemoteName) String() string {
	if len(remoteName.value) == 0 {
		panic("shared.RemoteName: zero value")
	}
	return remoteName.value
}

// BranchName captures validated branch identifiers.
type BranchName struct {
	value string
}

// NewBranchName validates branch names for whitespace and emptiness.
func NewBranchName(rawValue string) (BranchName, error) {
	trimmed := strings.TrimSpace(rawValue)
	if len(trimmed) == 0 {
		return BranchName{}, fmt.Errorf("%w: empty", ErrBranchNameInvalid)
	}
	if strings.ContainsAny(trimmed, branchWhitespaceCharacters) {
		return BranchName{}, fmt.Errorf("%w: contains whitespace", ErrBranchNameInvalid)
	}
	return BranchName{value: trimmed}, nil
}

// String returns the branch name.
func (branch BranchName) String() string {
	if len(branch.value) == 0 {
		panic("shared.BranchName: zero value")
	}
	return branch.value
}

// Clock abstracts time acquisition for deterministic testing.
type Clock interface {
	Now() time.Time
}

// SystemClock implements Clock using the system time source.
type SystemClock struct{}

// Now returns the current system time.
func (SystemClock) Now() time.Time {
	return time.Now()
}

// FileSystem exposes filesystem operations required by repository services.
type FileSystem interface {
	Stat(path string) (fs.FileInfo, error)
	Rename(oldPath string, newPath string) error
	Abs(path string) (string, error)
	MkdirAll(path string, permissions fs.FileMode) error
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, permissions fs.FileMode) error
}

// ConfirmationResult captures the outcome of a user confirmation prompt.
type ConfirmationResult struct {
	Confirmed  bool
	ApplyToAll bool
}

// ConfirmationPrompter collects user confirmations prior to mutating actions.
type ConfirmationPrompter interface {
	Confirm(prompt string) (ConfirmationResult, error)
}

// GitExecutor exposes the subset of shell execution used by repository services.
type GitExecutor interface {
	ExecuteGit(executionContext context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error)
	ExecuteGitHubCLI(executionContext context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error)
}

// GitRepositoryManager exposes repository-level git operations.
type GitRepositoryManager interface {
	CheckCleanWorktree(executionContext context.Context, repositoryPath string) (bool, error)
	WorktreeStatus(executionContext context.Context, repositoryPath string) ([]string, error)
	GetCurrentBranch(executionContext context.Context, repositoryPath string) (string, error)
	GetRemoteURL(executionContext context.Context, repositoryPath string, remoteName string) (string, error)
	SetRemoteURL(executionContext context.Context, repositoryPath string, remoteName string, remoteURL string) error
}

// GitHubMetadataResolver resolves canonical repository metadata via GitHub CLI.
type GitHubMetadataResolver interface {
	ResolveRepoMetadata(executionContext context.Context, repository string) (githubcli.RepositoryMetadata, error)
}

// RepositoryDiscoverer locates Git repositories for bulk operations.
type RepositoryDiscoverer interface {
	DiscoverRepositories(roots []string) ([]string, error)
}
