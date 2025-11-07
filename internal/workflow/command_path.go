package workflow

import "strings"

const (
	commandAuditReportKey                     = "audit report"
	commandBranchDefaultKey                   = "default"
	legacyCommandBranchDefaultHyphenKey       = "branch-default"
	legacyCommandBranchDefaultKey             = "branch default"
	commandFolderRenameKey                    = "folder rename"
	commandRemoteCanonicalKey                 = "remote update-to-canonical"
	commandRemoteConvertProtocolKey           = "remote update-protocol"
	commandTasksApplyKey                      = "tasks apply"
	legacyCommandRepoTasksApplyKey            = "repo tasks apply"
	legacyCommandRepoFolderRenameKey          = "repo folder rename"
	legacyCommandRepoRemoteCanonicalKey       = "repo remote update-to-canonical"
	legacyCommandRepoRemoteConvertProtocolKey = "repo remote update-protocol"
)

func normalizeCommandParts(parts []string) []string {
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if len(trimmed) == 0 {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

// CommandPathKey normalizes a command path into a lowercase, space-separated key.
func CommandPathKey(parts []string) string {
	normalized := normalizeCommandParts(parts)
	if len(normalized) == 0 {
		return ""
	}
	lowered := make([]string, len(normalized))
	for index := range normalized {
		lowered[index] = strings.ToLower(normalized[index])
	}
	return strings.Join(lowered, " ")
}
