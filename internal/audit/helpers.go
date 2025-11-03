package audit

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tyemirov/gix/internal/repos/shared"
)

const (
	githubHostConstant                 = "github.com"
	gitSuffixConstant                  = ".git"
	repositoryOwnerSeparatorConstant   = "/"
	refsHeadsPrefixConstant            = "refs/heads/"
	upstreamReferenceCommandArgument   = "@{u}"
	gitFetchSubcommandConstant         = "fetch"
	gitQuietFlagConstant               = "-q"
	gitNoTagsFlagConstant              = "--no-tags"
	gitNoRecurseSubmodulesFlagConstant = "--no-recurse-submodules"
	gitRevParseSubcommandConstant      = "rev-parse"
	gitAbbrevRefFlagConstant           = "--abbrev-ref"
	gitSymbolicFullNameFlagConstant    = "--symbolic-full-name"
	gitHeadReferenceConstant           = "HEAD"
	gitLSRemoteSubcommandConstant      = "ls-remote"
	gitSymrefFlagConstant              = "--symref"
	gitReferenceSeparator              = "\t"
)

var errOwnerRepoNotDetected = errors.New("owner repository not detected")

func detectRemoteProtocol(remote string) RemoteProtocolType {
	switch {
	case strings.HasPrefix(remote, shared.GitProtocolURLPrefixConstant):
		return RemoteProtocolGit
	case strings.HasPrefix(remote, shared.SSHProtocolURLPrefixConstant):
		return RemoteProtocolSSH
	case strings.HasPrefix(remote, shared.HTTPSProtocolURLPrefixConstant):
		return RemoteProtocolHTTPS
	default:
		return RemoteProtocolOther
	}
}

func canonicalizeOwnerRepo(remote string) (string, error) {
	trimmed := strings.TrimSpace(remote)
	switch {
	case strings.HasPrefix(trimmed, shared.GitProtocolURLPrefixConstant):
		trimmed = strings.TrimPrefix(trimmed, shared.GitProtocolURLPrefixConstant)
	case strings.HasPrefix(trimmed, shared.SSHProtocolURLPrefixConstant):
		trimmed = strings.TrimPrefix(trimmed, shared.SSHProtocolURLPrefixConstant)
	case strings.HasPrefix(trimmed, shared.HTTPSProtocolURLPrefixConstant):
		trimmed = strings.TrimPrefix(trimmed, shared.HTTPSProtocolURLPrefixConstant)
	default:
		return "", errOwnerRepoNotDetected
	}

	trimmed = strings.TrimSuffix(trimmed, gitSuffixConstant)
	segments := strings.Split(trimmed, repositoryOwnerSeparatorConstant)
	if len(segments) < 2 {
		return "", errOwnerRepoNotDetected
	}
	owner := segments[0]
	repository := segments[1]
	if len(owner) == 0 || len(repository) == 0 {
		return "", errOwnerRepoNotDetected
	}
	return fmt.Sprintf("%s/%s", owner, repository), nil
}

func finalRepositoryName(ownerRepo string) string {
	segments := strings.Split(ownerRepo, repositoryOwnerSeparatorConstant)
	if len(segments) == 0 {
		return ""
	}
	return segments[len(segments)-1]
}

func ownerRepoCaseInsensitiveEqual(first string, second string) bool {
	return strings.EqualFold(first, second)
}

func sanitizeBranchName(branch string) string {
	trimmed := strings.TrimSpace(branch)
	if trimmed == gitHeadReferenceConstant {
		return "DETACHED"
	}
	return trimmed
}

func remoteFetchArguments(branch string) []string {
	return []string{
		gitFetchSubcommandConstant,
		gitQuietFlagConstant,
		gitNoTagsFlagConstant,
		gitNoRecurseSubmodulesFlagConstant,
		shared.OriginRemoteNameConstant,
		branch,
	}
}

func upstreamReferenceArguments() []string {
	return []string{
		gitRevParseSubcommandConstant,
		gitAbbrevRefFlagConstant,
		gitSymbolicFullNameFlagConstant,
		upstreamReferenceCommandArgument,
	}
}

func headRevisionArguments() []string {
	return []string{
		gitRevParseSubcommandConstant,
		gitHeadReferenceConstant,
	}
}

func revisionArguments(reference string) []string {
	return []string{
		gitRevParseSubcommandConstant,
		reference,
	}
}

func fallbackRemoteRevisionReferences(branch string) []string {
	return []string{
		fmt.Sprintf("refs/remotes/%s/%s", shared.OriginRemoteNameConstant, branch),
		fmt.Sprintf("%s/%s", shared.OriginRemoteNameConstant, branch),
	}
}

func lsRemoteHeadArguments() []string {
	return []string{
		gitLSRemoteSubcommandConstant,
		gitSymrefFlagConstant,
		shared.OriginRemoteNameConstant,
		gitHeadReferenceConstant,
	}
}
