package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/stretchr/testify/require"
)

const (
	auditIntegrationTimeout                       = 10 * time.Second
	auditIntegrationLogLevelFlag                  = "--log-level"
	auditIntegrationErrorLevel                    = "error"
	auditIntegrationDebugLevel                    = "debug"
	auditIntegrationRunSubcommand                 = "run"
	auditIntegrationModulePathConstant            = "."
	auditIntegrationAuditCommandName              = "audit"
	auditIntegrationWorkflowCommandName           = "workflow"
	auditIntegrationRootFlag                      = "--roots"
	auditIntegrationIncludeAllFlag                = "--all"
	auditIntegrationFormatFlag                    = "--format"
	auditIntegrationGitExecutable                 = "git"
	auditIntegrationInitFlag                      = "init"
	auditIntegrationInitialBranchFlag             = "--initial-branch=main"
	auditIntegrationRemoteSubcommand              = "remote"
	auditIntegrationAddSubcommand                 = "add"
	auditIntegrationOriginRemoteName              = "origin"
	auditIntegrationOriginURL                     = "git@github.com:origin/example.git"
	auditIntegrationStubExecutableName            = "gh"
	auditIntegrationStubScript                    = "#!/bin/sh\nif [ \"$1\" = \"repo\" ] && [ \"$2\" = \"view\" ]; then\n  cat <<'EOF'\n{\"nameWithOwner\":\"canonical/example\",\"defaultBranchRef\":{\"name\":\"main\"},\"description\":\"\"}\nEOF\n  exit 0\nfi\nexit 0\n"
	auditIntegrationRepositoryPrefixConstant      = "audit-integration-repository-"
	auditIntegrationHomeShortcutPrefixConstant    = "~/"
	auditIntegrationCSVHeaderConstant             = "folder_name,final_github_repo,origin_remote_status,name_matches,remote_default_branch,local_branch,in_sync,remote_protocol,origin_matches_canonical,worktree_dirty,dirty_files\n"
	auditIntegrationCSVRowTemplate                = "%[1]s,canonical/example,configured,no,main,,n/a,ssh,no,no,\n"
	auditIntegrationCSVTemplate                   = auditIntegrationCSVHeaderConstant + auditIntegrationCSVRowTemplate
	auditIntegrationTableCaseNameConstant         = "audit_table_default"
	auditIntegrationCSVCaseNameConstant           = "audit_csv_export"
	auditIntegrationHTMLCaseNameConstant          = "audit_html_export"
	auditIntegrationTableEscapingCaseNameConstant = "audit_table_escapes_delimiters_and_aligns_wide_characters"
	auditIntegrationWorkflowTableCaseNameConstant = "workflow_audit_table_respects_terminal_width"
	auditIntegrationDebugCaseNameConstant         = "audit_debug"
	auditIntegrationTildeCaseNameConstant         = "audit_tilde"
	auditIntegrationIncludeAllCaseNameConstant    = "audit_include_all"
	auditIntegrationSubtestNameTemplate           = "%d_%s"
	auditIntegrationTableFormattingFolderName     = "界界|a"
	auditIntegrationEscapedTableFolderName        = "界界\\|a"
	auditIntegrationTableFirstBorder              = "+---------+"
	auditIntegrationColumnsEnvironmentVariable    = "COLUMNS"
	auditIntegrationDisabledColumnsWidth          = "0"
	auditIntegrationNarrowTerminalWidth           = 40
	auditIntegrationWideTerminalWidth             = 190
	auditIntegrationTableEllipsis                 = "…"
	auditIntegrationWorkflowFileName              = "audit-workflow.yaml"
	auditIntegrationWorkflowConfiguration         = "workflow:\n  - step:\n      command: ['audit', 'report']\n      with:\n        format: table\n"
)

func TestAuditRunCommandIntegration(testInstance *testing.T) {
	workingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRoot := filepath.Dir(workingDirectory)

	homeDirectory, homeDirectoryError := os.UserHomeDir()
	require.NoError(testInstance, homeDirectoryError)

	repositoryPath, repositoryPathError := os.MkdirTemp(homeDirectory, auditIntegrationRepositoryPrefixConstant)
	require.NoError(testInstance, repositoryPathError)
	testInstance.Cleanup(func() {
		_ = os.RemoveAll(repositoryPath)
	})

	tempDirectory := testInstance.TempDir()

	initCommand := exec.Command(auditIntegrationGitExecutable, auditIntegrationInitFlag, auditIntegrationInitialBranchFlag, repositoryPath)
	initCommand.Env = buildGitCommandEnvironment(nil)
	initError := initCommand.Run()
	require.NoError(testInstance, initError)

	remoteCommand := exec.Command(auditIntegrationGitExecutable, "-C", repositoryPath, auditIntegrationRemoteSubcommand, auditIntegrationAddSubcommand, auditIntegrationOriginRemoteName, auditIntegrationOriginURL)
	remoteCommand.Env = buildGitCommandEnvironment(nil)
	remoteError := remoteCommand.Run()
	require.NoError(testInstance, remoteError)

	stubPath := filepath.Join(tempDirectory, auditIntegrationStubExecutableName)
	stubWriteError := os.WriteFile(stubPath, []byte(auditIntegrationStubScript), 0o755)
	require.NoError(testInstance, stubWriteError)

	pathWithStub := filepath.Join(tempDirectory, "bin")
	require.NoError(testInstance, os.Mkdir(pathWithStub, 0o755))
	finalStubPath := filepath.Join(pathWithStub, auditIntegrationStubExecutableName)
	require.NoError(testInstance, os.Rename(stubPath, finalStubPath))

	extendedPath := pathWithStub + string(os.PathListSeparator) + os.Getenv("PATH")

	repositoryFolderName := filepath.Base(repositoryPath)
	expectedCSVOutput := fmt.Sprintf(auditIntegrationCSVTemplate, repositoryFolderName)
	relativeRepositoryPath := strings.TrimPrefix(repositoryPath, homeDirectory)
	relativeRepositoryPath = strings.TrimPrefix(relativeRepositoryPath, string(os.PathSeparator))
	tildeRootArgument := auditIntegrationHomeShortcutPrefixConstant + filepath.ToSlash(relativeRepositoryPath)

	includeAllRoot := filepath.Join(tempDirectory, "include_all_root")
	require.NoError(testInstance, os.Mkdir(includeAllRoot, 0o755))
	testInstance.Cleanup(func() {
		_ = os.RemoveAll(includeAllRoot)
	})
	includeAllRepositoryPath := filepath.Join(includeAllRoot, "audit-all-repository")
	initIncludeAllCommand := exec.Command(auditIntegrationGitExecutable, auditIntegrationInitFlag, auditIntegrationInitialBranchFlag, includeAllRepositoryPath)
	initIncludeAllCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, initIncludeAllCommand.Run())
	includeAllRemoteCommand := exec.Command(auditIntegrationGitExecutable, "-C", includeAllRepositoryPath, auditIntegrationRemoteSubcommand, auditIntegrationAddSubcommand, auditIntegrationOriginRemoteName, auditIntegrationOriginURL)
	includeAllRemoteCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, includeAllRemoteCommand.Run())

	nonGitFolderName := "notes"
	nonGitFolderPath := filepath.Join(includeAllRoot, nonGitFolderName)
	require.NoError(testInstance, os.Mkdir(nonGitFolderPath, 0o755))
	nestedNonGitFolderName := "drafts"
	nestedNonGitFolderPath := filepath.Join(nonGitFolderPath, nestedNonGitFolderName)
	require.NoError(testInstance, os.MkdirAll(nestedNonGitFolderPath, 0o755))

	tableFormattingRoot := filepath.Join(tempDirectory, "table-formatting-root")
	require.NoError(testInstance, os.Mkdir(tableFormattingRoot, 0o755))
	tableFormattingRepositoryPath := filepath.Join(tableFormattingRoot, auditIntegrationTableFormattingFolderName)
	tableFormattingInitCommand := exec.Command(auditIntegrationGitExecutable, auditIntegrationInitFlag, auditIntegrationInitialBranchFlag, tableFormattingRepositoryPath)
	tableFormattingInitCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, tableFormattingInitCommand.Run())
	tableFormattingRemoteCommand := exec.Command(auditIntegrationGitExecutable, "-C", tableFormattingRepositoryPath, auditIntegrationRemoteSubcommand, auditIntegrationAddSubcommand, auditIntegrationOriginRemoteName, auditIntegrationOriginURL)
	tableFormattingRemoteCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, tableFormattingRemoteCommand.Run())

	buildArguments := func(logLevel string, root string) []string {
		return []string{
			auditIntegrationRunSubcommand,
			auditIntegrationModulePathConstant,
			auditIntegrationLogLevelFlag,
			logLevel,
			auditIntegrationAuditCommandName,
			auditIntegrationRootFlag,
			root,
		}
	}
	withFormat := func(arguments []string, reportFormat string) []string {
		formattedArguments := append([]string{}, arguments...)
		return append(formattedArguments, auditIntegrationFormatFlag, reportFormat)
	}

	rootFlagArguments := buildArguments(auditIntegrationErrorLevel, repositoryPath)
	csvArguments := withFormat(rootFlagArguments, "csv")
	htmlArguments := withFormat(rootFlagArguments, "html")
	debugLogLevelArguments := buildArguments(auditIntegrationDebugLevel, repositoryPath)
	tildeRootArguments := buildArguments(auditIntegrationErrorLevel, tildeRootArgument)
	includeAllArguments := append(buildArguments(auditIntegrationErrorLevel, includeAllRoot), auditIntegrationIncludeAllFlag)
	includeAllRepositoryFolderName := filepath.Base(includeAllRepositoryPath)
	tableFormattingArguments := buildArguments(auditIntegrationErrorLevel, tableFormattingRoot)

	testCases := []struct {
		name                string
		arguments           []string
		expectedPrefix      string
		expectedOutput      string
		expectedFragments   []string
		unexpectedFragments []string
	}{
		{
			name:      auditIntegrationTableCaseNameConstant,
			arguments: rootFlagArguments,
			expectedFragments: []string{
				"| Folder ",
				"| Final Repository ",
				repositoryFolderName,
				"canonical/example",
			},
			unexpectedFragments: []string{auditIntegrationCSVHeaderConstant},
		},
		{
			name:           auditIntegrationCSVCaseNameConstant,
			arguments:      csvArguments,
			expectedOutput: expectedCSVOutput,
		},
		{
			name:      auditIntegrationHTMLCaseNameConstant,
			arguments: htmlArguments,
			expectedFragments: []string{
				"<!doctype html>",
				"<title>gix audit report</title>",
				"<table>",
				"<th>Folder</th>",
				"<td>" + repositoryFolderName + "</td>",
				"<td>canonical/example</td>",
			},
			unexpectedFragments: []string{auditIntegrationCSVHeaderConstant},
		},
		{
			name:           auditIntegrationTableEscapingCaseNameConstant,
			arguments:      tableFormattingArguments,
			expectedPrefix: auditIntegrationTableFirstBorder,
			expectedFragments: []string{
				auditIntegrationEscapedTableFolderName,
			},
			unexpectedFragments: []string{auditIntegrationTableFormattingFolderName, auditIntegrationCSVHeaderConstant},
		},
		{
			name:      auditIntegrationDebugCaseNameConstant,
			arguments: debugLogLevelArguments,
			expectedFragments: []string{
				fmt.Sprintf("DEBUG: discovered 1 candidate repos under: %s", repositoryPath),
				fmt.Sprintf("DEBUG: checking %s", repositoryPath),
				"| Folder ",
				repositoryFolderName,
			},
			unexpectedFragments: []string{auditIntegrationCSVHeaderConstant},
		},
		{
			name:      auditIntegrationTildeCaseNameConstant,
			arguments: tildeRootArguments,
			expectedFragments: []string{
				"| Folder ",
				repositoryFolderName,
			},
			unexpectedFragments: []string{auditIntegrationCSVHeaderConstant},
		},
		{
			name:      auditIntegrationIncludeAllCaseNameConstant,
			arguments: includeAllArguments,
			expectedFragments: []string{
				"| Folder ",
				includeAllRepositoryFolderName,
				nonGitFolderName,
			},
			unexpectedFragments: []string{auditIntegrationCSVHeaderConstant, nestedNonGitFolderName},
		},
	}

	for testCaseIndex, testCase := range testCases {
		testInstance.Run(fmt.Sprintf(auditIntegrationSubtestNameTemplate, testCaseIndex, testCase.name), func(subtest *testing.T) {
			commandOptions := integrationCommandOptions{
				PathVariable: extendedPath,
				EnvironmentOverrides: map[string]string{
					auditIntegrationColumnsEnvironmentVariable: auditIntegrationDisabledColumnsWidth,
				},
			}
			subtestOutput := runIntegrationCommand(subtest, repositoryRoot, commandOptions, auditIntegrationTimeout, testCase.arguments)
			filteredOutput := filterStructuredOutput(subtestOutput)
			if len(testCase.expectedOutput) > 0 {
				require.Equal(subtest, testCase.expectedOutput, filteredOutput)
			}
			if len(testCase.expectedPrefix) > 0 {
				require.True(subtest, strings.HasPrefix(filteredOutput, testCase.expectedPrefix))
			}
			for _, fragment := range testCase.expectedFragments {
				require.Contains(subtest, filteredOutput, fragment)
			}
			for _, fragment := range testCase.unexpectedFragments {
				require.NotContains(subtest, filteredOutput, fragment)
			}
		})
	}

	testInstance.Run("audit_table_respects_terminal_width", func(subtest *testing.T) {
		constrainedOutput := runIntegrationCommand(
			subtest,
			repositoryRoot,
			integrationCommandOptions{
				PathVariable: extendedPath,
				EnvironmentOverrides: map[string]string{
					auditIntegrationColumnsEnvironmentVariable: fmt.Sprint(auditIntegrationNarrowTerminalWidth),
				},
			},
			auditIntegrationTimeout,
			rootFlagArguments,
		)
		constrainedTable := filterStructuredOutput(constrainedOutput)
		require.Contains(subtest, constrainedTable, auditIntegrationTableEllipsis)
		for _, header := range []string{
			"Folder",
			"Final Repository",
			"Origin",
			"Name Matches",
			"Remote Default",
			"Local Branch",
			"In Sync",
			"Protocol",
			"Origin Canonical",
			"Worktree Dirty",
			"Dirty Files",
		} {
			require.Contains(subtest, constrainedTable, header)
		}
		requireAuditTableFitsTerminal(subtest, constrainedTable, auditIntegrationNarrowTerminalWidth)
	})

	testInstance.Run("audit_table_truncates_horizontal_layout_to_terminal_width", func(subtest *testing.T) {
		constrainedOutput := runIntegrationCommand(
			subtest,
			repositoryRoot,
			integrationCommandOptions{
				PathVariable: extendedPath,
				EnvironmentOverrides: map[string]string{
					auditIntegrationColumnsEnvironmentVariable: fmt.Sprint(auditIntegrationWideTerminalWidth),
				},
			},
			auditIntegrationTimeout,
			rootFlagArguments,
		)
		constrainedTable := filterStructuredOutput(constrainedOutput)
		constrainedLines := strings.Split(strings.TrimSpace(constrainedTable), "\n")
		require.GreaterOrEqual(subtest, len(constrainedLines), 3)
		require.Contains(subtest, constrainedLines[1], "Folder")
		require.Contains(subtest, constrainedLines[1], "Final Repository")
		require.Contains(subtest, constrainedLines[1], "Dirty Files")
		require.Contains(subtest, constrainedTable, auditIntegrationTableEllipsis)
		requireAuditTableFitsTerminal(subtest, constrainedTable, auditIntegrationWideTerminalWidth)
	})

	testInstance.Run(auditIntegrationWorkflowTableCaseNameConstant, func(subtest *testing.T) {
		workflowPath := filepath.Join(tempDirectory, auditIntegrationWorkflowFileName)
		require.NoError(subtest, os.WriteFile(workflowPath, []byte(auditIntegrationWorkflowConfiguration), 0o644))

		workflowArguments := []string{
			auditIntegrationRunSubcommand,
			auditIntegrationModulePathConstant,
			auditIntegrationLogLevelFlag,
			auditIntegrationErrorLevel,
			auditIntegrationWorkflowCommandName,
			workflowPath,
			auditIntegrationRootFlag,
			repositoryPath,
		}
		workflowOutput := runIntegrationCommand(
			subtest,
			repositoryRoot,
			integrationCommandOptions{
				PathVariable: extendedPath,
				EnvironmentOverrides: map[string]string{
					auditIntegrationColumnsEnvironmentVariable: fmt.Sprint(auditIntegrationNarrowTerminalWidth),
				},
			},
			auditIntegrationTimeout,
			workflowArguments,
		)
		workflowTable := auditTableOutput(filterStructuredOutput(workflowOutput))
		require.NotEmpty(subtest, workflowTable)
		require.Contains(subtest, workflowTable, auditIntegrationTableEllipsis)
		requireAuditTableFitsTerminal(subtest, workflowTable, auditIntegrationNarrowTerminalWidth)
	})

	invalidFormatOutput, invalidFormatError := runFailingIntegrationCommand(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{PathVariable: extendedPath},
		auditIntegrationTimeout,
		withFormat(rootFlagArguments, "json"),
	)
	require.Error(testInstance, invalidFormatError)
	require.Contains(testInstance, filterStructuredOutput(invalidFormatOutput), "unsupported audit report format \"json\"")
}

func requireAuditTableFitsTerminal(testInstance *testing.T, table string, terminalWidth int) {
	testInstance.Helper()
	for _, line := range strings.Split(strings.TrimSpace(table), "\n") {
		require.LessOrEqualf(
			testInstance,
			runewidth.StringWidth(line),
			terminalWidth,
			"audit table line exceeds terminal width: %q",
			line,
		)
	}
}

func auditTableOutput(output string) string {
	tableLines := []string{}
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "|") {
			tableLines = append(tableLines, line)
		}
	}
	return strings.Join(tableLines, "\n")
}
