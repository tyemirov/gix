package cli

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	changelogcmd "github.com/temirov/gix/cmd/cli/changelog"
	commitcmd "github.com/temirov/gix/cmd/cli/commit"
	"github.com/temirov/gix/cmd/cli/repos"
	releasecmd "github.com/temirov/gix/cmd/cli/repos/release"
	workflowcmd "github.com/temirov/gix/cmd/cli/workflow"
	auditcli "github.com/temirov/gix/internal/audit/cli"
	"github.com/temirov/gix/internal/branches"
	branchcdcmd "github.com/temirov/gix/internal/branches/cd"
	migratecli "github.com/temirov/gix/internal/migrate/cli"
	"github.com/temirov/gix/internal/packages"
	"github.com/temirov/gix/internal/repos/prompt"
	"github.com/temirov/gix/internal/repos/shared"
)

func (application *Application) registerCommands(cobraCommand *cobra.Command) {
	auditBuilder := auditcli.CommandBuilder{
		LoggerProvider: func() *zap.Logger {
			return application.logger
		},
		HumanReadableLoggingProvider: func() bool { return false },
		ConfigurationProvider:        application.auditCommandConfiguration,
	}
	auditCommand, auditBuildError := auditBuilder.Build()
	if auditBuildError == nil {
		auditCommand.Aliases = appendUnique(auditCommand.Aliases, auditCommandAliasConstant)
		cobraCommand.AddCommand(auditCommand)
	}

	branchCleanupBuilder := branches.CommandBuilder{
		LoggerProvider: func() *zap.Logger {
			return application.logger
		},
		HumanReadableLoggingProvider: application.humanReadableLoggingEnabled,
		ConfigurationProvider:        application.branchCleanupConfiguration,
		PrompterFactory: func(command *cobra.Command) shared.ConfirmationPrompter {
			if command == nil {
				return nil
			}
			return prompt.NewIOConfirmationPrompter(command.InOrStdin(), command.OutOrStdout())
		},
	}

	branchChangeBuilder := branchcdcmd.CommandBuilder{
		LoggerProvider: func() *zap.Logger {
			return application.logger
		},
		HumanReadableLoggingProvider: application.humanReadableLoggingEnabled,
		ConfigurationProvider:        application.branchChangeConfiguration,
	}

	defaultCommandBuilder := migratecli.CommandBuilder{
		LoggerProvider: func() *zap.Logger {
			return application.logger
		},
		HumanReadableLoggingProvider: application.humanReadableLoggingEnabled,
		ConfigurationProvider:        application.defaultCommandConfiguration,
	}

	packagesBuilder := packages.CommandBuilder{
		LoggerProvider: func() *zap.Logger {
			return application.logger
		},
		ConfigurationProvider: application.packagesConfiguration,
	}

	releaseBuilder := releasecmd.CommandBuilder{
		LoggerProvider: func() *zap.Logger {
			return application.logger
		},
		HumanReadableLoggingProvider: application.humanReadableLoggingEnabled,
		ConfigurationProvider:        application.repoReleaseConfiguration,
	}

	retagBuilder := releasecmd.RetagCommandBuilder{
		LoggerProvider: func() *zap.Logger {
			return application.logger
		},
		HumanReadableLoggingProvider: application.humanReadableLoggingEnabled,
		ConfigurationProvider:        application.repoReleaseConfiguration,
	}

	renameBuilder := repos.RenameCommandBuilder{
		LoggerProvider: func() *zap.Logger {
			return application.logger
		},
		HumanReadableLoggingProvider: application.humanReadableLoggingEnabled,
		ConfigurationProvider:        application.reposRenameConfiguration,
	}

	remotesBuilder := repos.RemotesCommandBuilder{
		LoggerProvider: func() *zap.Logger {
			return application.logger
		},
		HumanReadableLoggingProvider: application.humanReadableLoggingEnabled,
		ConfigurationProvider:        application.reposRemotesConfiguration,
	}

	protocolBuilder := repos.ProtocolCommandBuilder{
		LoggerProvider: func() *zap.Logger {
			return application.logger
		},
		HumanReadableLoggingProvider: application.humanReadableLoggingEnabled,
		ConfigurationProvider:        application.reposProtocolConfiguration,
	}

	removeBuilder := repos.RemoveCommandBuilder{
		LoggerProvider: func() *zap.Logger {
			return application.logger
		},
		HumanReadableLoggingProvider: application.humanReadableLoggingEnabled,
		ConfigurationProvider:        application.reposRemoveConfiguration,
	}

	replaceBuilder := repos.ReplaceCommandBuilder{
		LoggerProvider: func() *zap.Logger {
			return application.logger
		},
		HumanReadableLoggingProvider: application.humanReadableLoggingEnabled,
		ConfigurationProvider:        application.reposReplaceConfiguration,
	}

	filesAddBuilder := repos.FilesAddCommandBuilder{
		LoggerProvider: func() *zap.Logger {
			return application.logger
		},
		HumanReadableLoggingProvider: application.humanReadableLoggingEnabled,
		ConfigurationProvider:        application.reposFilesAddConfiguration,
	}

	licenseBuilder := repos.LicenseCommandBuilder{
		LoggerProvider: func() *zap.Logger {
			return application.logger
		},
		HumanReadableLoggingProvider: application.humanReadableLoggingEnabled,
		ConfigurationProvider:        application.reposLicenseConfiguration,
	}

	workflowBuilder := workflowcmd.CommandBuilder{
		LoggerProvider: func() *zap.Logger {
			return application.logger
		},
		HumanReadableLoggingProvider: application.humanReadableLoggingEnabled,
		ConfigurationProvider:        application.workflowCommandConfiguration,
	}
	workflowCommand, workflowBuildError := workflowBuilder.Build()
	if workflowBuildError == nil {
		workflowCommand.Aliases = appendUnique(workflowCommand.Aliases, workflowCommandAliasConstant)
		cobraCommand.AddCommand(workflowCommand)
	}

	changelogMessageBuilder := changelogcmd.MessageCommandBuilder{
		LoggerProvider: func() *zap.Logger {
			return application.logger
		},
		HumanReadableLoggingProvider: application.humanReadableLoggingEnabled,
		ConfigurationProvider:        application.changelogMessageConfiguration,
	}
	messageNamespaceCommand := newNamespaceCommand(messageNamespaceUseNameConstant, messageNamespaceShortDescriptionConstant, messageNamespaceAliasConstant)
	var changelogNamespaceCommand *cobra.Command
	changelogMessageCommand, changelogMessageBuildError := changelogMessageBuilder.Build()
	if changelogMessageBuildError == nil {
		configureCommandMetadata(changelogMessageCommand, changelogMessageUseNameConstant, changelogMessageCommand.Short, changelogMessageLongDescriptionConstant, changelogMessageAliasConstant)
		messageNamespaceCommand.AddCommand(changelogMessageCommand)
	}
	if legacyChangelogCommand, legacyChangelogBuildError := changelogMessageBuilder.Build(); legacyChangelogBuildError == nil {
		configureCommandMetadata(legacyChangelogCommand, legacyChangelogMessageUseNameConstant, legacyChangelogCommand.Short, changelogMessageLongDescriptionConstant, changelogMessageAliasConstant)
		legacyChangelogCommand.Deprecated = legacyChangelogMessageDeprecatedMessageConstant
		changelogNamespaceCommand = newNamespaceCommand(changelogNamespaceUseNameConstant, changelogNamespaceShortDescriptionConstant, changelogNamespaceAliasConstant)
		changelogNamespaceCommand.AddCommand(legacyChangelogCommand)
	}
	commitMessageBuilder := commitcmd.MessageCommandBuilder{
		LoggerProvider: func() *zap.Logger {
			return application.logger
		},
		HumanReadableLoggingProvider: application.humanReadableLoggingEnabled,
		ConfigurationProvider:        application.commitMessageConfiguration,
	}
	var commitNamespaceCommand *cobra.Command
	commitMessageCommand, commitMessageBuildError := commitMessageBuilder.Build()
	if commitMessageBuildError == nil {
		configureCommandMetadata(commitMessageCommand, commitMessageUseNameConstant, commitMessageCommand.Short, commitMessageLongDescriptionConstant, commitMessageAliasConstant)
		messageNamespaceCommand.AddCommand(commitMessageCommand)
	}
	if commitNamespaceCommand == nil {
		commitNamespaceCommand = newNamespaceCommand(legacyCommitNamespaceUseNameConstant, commitNamespaceShortDescriptionConstant, commitNamespaceAliasConstant)
	}
	if legacyCommitMessageCommand, legacyCommitBuildError := commitMessageBuilder.Build(); legacyCommitBuildError == nil {
		configureCommandMetadata(legacyCommitMessageCommand, legacyCommitMessageUseNameConstant, legacyCommitMessageCommand.Short, commitMessageLongDescriptionConstant, commitMessageAliasConstant)
		legacyCommitMessageCommand.Deprecated = "command deprecated; use \"gix message commit\"."
		commitNamespaceCommand.AddCommand(legacyCommitMessageCommand)
	}

	repoFolderCommand := newNamespaceCommand(repoFolderNamespaceUseNameConstant, repoFolderNamespaceShortDescriptionConstant, repoFolderNamespaceAliasConstant)
	if renameNestedCommand, nestedRenameError := renameBuilder.Build(); nestedRenameError == nil {
		configureCommandMetadata(renameNestedCommand, renameCommandUseNameConstant, renameNestedCommand.Short, renameNestedLongDescriptionConstant)
		repoFolderCommand.AddCommand(renameNestedCommand)
	}
	if len(repoFolderCommand.Commands()) > 0 {
		cobraCommand.AddCommand(repoFolderCommand)
	}

	repoRemoteCommand := newNamespaceCommand(repoRemoteNamespaceUseNameConstant, repoRemoteNamespaceShortDescriptionConstant)
	if canonicalRemoteCommand, canonicalRemoteError := remotesBuilder.Build(); canonicalRemoteError == nil {
		configureCommandMetadata(canonicalRemoteCommand, updateRemoteCanonicalUseNameConstant, canonicalRemoteCommand.Short, updateRemoteCanonicalLongDescriptionConstant, updateRemoteCanonicalAliasConstant)
		repoRemoteCommand.AddCommand(canonicalRemoteCommand)
	}
	if protocolConversionCommand, protocolConversionError := protocolBuilder.Build(); protocolConversionError == nil {
		configureCommandMetadata(protocolConversionCommand, updateProtocolCommandUseNameConstant, protocolConversionCommand.Short, updateProtocolLongDescriptionConstant, updateProtocolAliasConstant)
		repoRemoteCommand.AddCommand(protocolConversionCommand)
	}
	if len(repoRemoteCommand.Commands()) > 0 {
		cobraCommand.AddCommand(repoRemoteCommand)
	}

	repoPullRequestsCommand := newNamespaceCommand(repoPullRequestsNamespaceUseNameConstant, repoPullRequestsNamespaceShortDescriptionConstant)
	if pullRequestCleanupCommand, pullRequestCleanupError := branchCleanupBuilder.Build(); pullRequestCleanupError == nil {
		configureCommandMetadata(pullRequestCleanupCommand, prsDeleteCommandUseNameConstant, pullRequestCleanupCommand.Short, prsDeleteLongDescriptionConstant, prsDeleteCommandAliasConstant)
		repoPullRequestsCommand.AddCommand(pullRequestCleanupCommand)
	}
	if len(repoPullRequestsCommand.Commands()) > 0 {
		cobraCommand.AddCommand(repoPullRequestsCommand)
	}

	repoPackagesCommand := newNamespaceCommand(repoPackagesNamespaceUseNameConstant, repoPackagesNamespaceShortDescriptionConstant)
	if packagesCleanupCommand, packagesCleanupError := packagesBuilder.Build(); packagesCleanupError == nil {
		configureCommandMetadata(packagesCleanupCommand, packagesDeleteCommandUseNameConstant, packagesCleanupCommand.Short, packagesDeleteLongDescriptionConstant, packagesDeleteCommandAliasConstant)
		repoPackagesCommand.AddCommand(packagesCleanupCommand)
	}
	if len(repoPackagesCommand.Commands()) > 0 {
		cobraCommand.AddCommand(repoPackagesCommand)
	}

	repoFilesCommand := newNamespaceCommand(repoFilesNamespaceUseNameConstant, repoFilesNamespaceShortDescriptionConstant, repoFilesNamespaceAliasConstant)
	if filesReplaceCommand, filesReplaceBuildError := replaceBuilder.Build(); filesReplaceBuildError == nil {
		configureCommandMetadata(filesReplaceCommand, filesReplaceCommandUseNameConstant, filesReplaceCommand.Short, filesReplaceCommandLongDescriptionConstant, filesReplaceCommandAliasConstant)
		repoFilesCommand.AddCommand(filesReplaceCommand)
	}
	if filesAddCommand, filesAddBuildError := filesAddBuilder.Build(); filesAddBuildError == nil {
		configureCommandMetadata(filesAddCommand, filesAddCommandUseNameConstant, filesAddCommand.Short, filesAddCommandLongDescriptionConstant, filesAddCommandAliasConstant)
		repoFilesCommand.AddCommand(filesAddCommand)
	}
	if filesRemoveCommand, filesRemoveBuildError := removeBuilder.Build(); filesRemoveBuildError == nil {
		configureCommandMetadata(filesRemoveCommand, removeCommandUseNameConstant, filesRemoveCommand.Short, removeCommandLongDescriptionConstant, removeCommandAliasConstant)
		repoFilesCommand.AddCommand(filesRemoveCommand)
	}
	if len(repoFilesCommand.Commands()) > 0 {
		cobraCommand.AddCommand(repoFilesCommand)
	}

	repoLicenseCommand := newNamespaceCommand(repoLicenseNamespaceUseNameConstant, repoLicenseNamespaceShortDescriptionConstant)
	if licenseApplyCommand, licenseBuildError := licenseBuilder.Build(); licenseBuildError == nil {
		configureCommandMetadata(licenseApplyCommand, licenseApplyCommandUseNameConstant, licenseApplyCommand.Short, licenseApplyCommandLongDescriptionConstant, licenseApplyCommandAliasConstant)
		repoLicenseCommand.AddCommand(licenseApplyCommand)
	}
	if len(repoLicenseCommand.Commands()) > 0 {
		cobraCommand.AddCommand(repoLicenseCommand)
	}

	if legacyRemoveCommand, legacyRemoveBuildError := removeBuilder.Build(); legacyRemoveBuildError == nil {
		configureCommandMetadata(legacyRemoveCommand, removeCommandUseNameConstant, removeCommandShortDescriptionConstant, removeCommandLongDescriptionConstant, removeCommandAliasConstant)
		legacyRemoveCommand.Deprecated = "command deprecated; use \"gix files rm\"."
		cobraCommand.AddCommand(legacyRemoveCommand)
	}

	var releaseCommand *cobra.Command
	if builtReleaseCommand, releaseBuildError := releaseBuilder.Build(); releaseBuildError == nil {
		configureCommandMetadata(builtReleaseCommand, repoReleaseCommandUsageTemplateConstant, builtReleaseCommand.Short, repoReleaseCommandLongDescriptionConstant, repoReleaseCommandAliasConstant)
		releaseCommand = builtReleaseCommand
	}
	if retagCommand, retagBuildError := retagBuilder.Build(); retagBuildError == nil {
		configureCommandMetadata(retagCommand, releaseRetagCommandUseNameConstant, retagCommand.Short, releaseRetagCommandLongDescriptionConstant, releaseRetagCommandAliasConstant)
		if releaseCommand != nil {
			releaseCommand.AddCommand(retagCommand)
		} else {
			cobraCommand.AddCommand(retagCommand)
		}
	}
	if releaseCommand != nil {
		cobraCommand.AddCommand(releaseCommand)
	}

	if len(messageNamespaceCommand.Commands()) > 0 {
		cobraCommand.AddCommand(messageNamespaceCommand)
	}
	if changelogNamespaceCommand != nil && len(changelogNamespaceCommand.Commands()) > 0 {
		cobraCommand.AddCommand(changelogNamespaceCommand)
	}
	if commitNamespaceCommand != nil && len(commitNamespaceCommand.Commands()) > 0 {
		cobraCommand.AddCommand(commitNamespaceCommand)
	}

	if defaultCommand, defaultCommandError := defaultCommandBuilder.Build(); defaultCommandError == nil {
		configureCommandMetadata(defaultCommand, defaultCommandUsageTemplateConstant, defaultCommand.Short, defaultCommandLongDescriptionConstant, legacyBranchDefaultTopLevelUseNameConstant)
		cobraCommand.AddCommand(defaultCommand)
	}
	if branchChangeCommand, branchChangeError := branchChangeBuilder.Build(); branchChangeError == nil {
		configureCommandMetadata(
			branchChangeCommand,
			branchChangeTopLevelUsageTemplateConstant,
			branchChangeCommand.Short,
			branchChangeLongDescriptionConstant,
			branchChangeCommandAliasConstant,
			branchChangeLegacyTopLevelUseNameConstant,
		)
		cobraCommand.AddCommand(branchChangeCommand)
	}

}

func appendUnique(values []string, candidates ...string) []string {
	result := values
	for _, candidate := range candidates {
		trimmedCandidate := strings.TrimSpace(candidate)
		if len(trimmedCandidate) == 0 {
			continue
		}
		duplicate := false
		for _, existing := range result {
			if existing == trimmedCandidate {
				duplicate = true
				break
			}
		}
		if !duplicate {
			result = append(result, trimmedCandidate)
		}
	}
	return result
}

func configureCommandMetadata(command *cobra.Command, use string, shortDescription string, longDescription string, aliases ...string) {
	if command == nil {
		return
	}

	useValue := strings.TrimSpace(use)
	if len(useValue) > 0 {
		command.Use = useValue
	}

	shortValue := strings.TrimSpace(shortDescription)
	if len(shortValue) > 0 {
		command.Short = shortValue
	}

	longValue := strings.TrimSpace(longDescription)
	if len(longValue) > 0 {
		command.Long = longValue
	}

	command.Aliases = appendUnique(command.Aliases, aliases...)
}

func newNamespaceCommand(use string, shortDescription string, aliases ...string) *cobra.Command {
	useValue := strings.TrimSpace(use)
	if len(useValue) == 0 {
		useValue = repoNamespaceUseNameConstant
	}

	shortValue := strings.TrimSpace(shortDescription)
	if len(shortValue) == 0 {
		shortValue = applicationShortDescriptionConstant
	}

	command := &cobra.Command{
		Use:           useValue,
		Short:         shortValue,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	command.Aliases = appendUnique(command.Aliases, aliases...)

	return command
}

func collectRequiredOperationConfigurationNames() []string {
	uniqueNames := make(map[string]struct{})
	for _, operationNames := range commandOperationRequirements {
		for _, operationName := range operationNames {
			uniqueNames[operationName] = struct{}{}
		}
	}

	orderedNames := make([]string, 0, len(uniqueNames))
	for operationName := range uniqueNames {
		orderedNames = append(orderedNames, operationName)
	}

	sort.Strings(orderedNames)

	return orderedNames
}
