package tests

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	workflowIntegrationTimeout                 = 15 * time.Second
	workflowIntegrationRunSubcommand           = "run"
	workflowIntegrationModulePathConstant      = "."
	workflowIntegrationLogLevelFlag            = "--log-level"
	workflowIntegrationConfigFlag              = "--config"
	workflowIntegrationErrorLevel              = "error"
	workflowIntegrationCommand                 = "workflow"
	workflowIntegrationRootsFlag               = "--roots"
	workflowIntegrationYesFlag                 = "--yes"
	workflowIntegrationGitExecutable           = "git"
	workflowIntegrationInitFlag                = "init"
	workflowIntegrationInitialBranchFlag       = "--initial-branch=main"
	workflowIntegrationConfigUserName          = "config"
	workflowIntegrationUserNameKey             = "user.name"
	workflowIntegrationUserEmailKey            = "user.email"
	workflowIntegrationUserNameValue           = "Workflow Tester"
	workflowIntegrationUserEmailValue          = "workflow@example.com"
	workflowIntegrationCheckoutCommand         = "checkout"
	workflowIntegrationBranchCommand           = "branch"
	workflowIntegrationMasterBranch            = "master"
	workflowIntegrationReadmeFileName          = "README.md"
	workflowIntegrationInitialCommitMessage    = "initial commit"
	workflowIntegrationWorkflowDirectory       = ".github/workflows"
	workflowIntegrationWorkflowFileName        = "ci.yml"
	workflowIntegrationWorkflowContent         = "name: CI\non:\n  push:\n    branches:\n      - main\n"
	workflowIntegrationWorkflowCommitMessage   = "add workflow"
	workflowIntegrationOriginRemoteName        = "origin"
	workflowIntegrationHTTPSRemote             = "https://github.com/origin/example.git"
	workflowIntegrationStubExecutable          = "gh"
	workflowIntegrationStateFileName           = "default_branch.txt"
	workflowIntegrationConfigFileName          = "config.yaml"
	workflowIntegrationConfigSearchPathEnvVar  = "GIX_CONFIG_SEARCH_PATH"
	workflowIntegrationAuditFileName           = "audit.csv"
	workflowIntegrationBranchCommitMessage     = "CI: switch workflow branch filters to master"
	workflowIntegrationRepoViewJSONTemplate    = "{\"nameWithOwner\":\"canonical/example\",\"defaultBranchRef\":{\"name\":\"%s\"},\"description\":\"\"}\n"
	workflowIntegrationConvertExpectedTemplate = "-- canonical/example (%s) --"
	workflowIntegrationRemoteUpdateLine        = "    - origin now git@github.com:canonical/example.git"
	workflowIntegrationRemoteSkipExpectedLine  = "    - already canonical"
	workflowIntegrationDefaultExpectedTemplate = "WORKFLOW-DEFAULT: %s (main → master)"
	workflowIntegrationAuditExpectedTemplate   = "WORKFLOW-AUDIT: wrote report to %s\n"
	workflowIntegrationCSVHeader               = "folder_name,final_github_repo,name_matches,remote_default_branch,local_branch,in_sync,remote_protocol,origin_matches_canonical\n"
	workflowIntegrationSubtestNameTemplate     = "%d_%s"
	workflowIntegrationDefaultCaseName         = "protocol_default_audit"
	workflowIntegrationConfigFlagCaseName      = "config_flag_without_positional"
	workflowIntegrationRepositoryConfigCase    = "repository_root_configuration"
	workflowIntegrationHelpCaseName            = "workflow_help_missing_configuration"
	workflowIntegrationUsageSnippet            = "workflow <configuration|preset>"
	workflowIntegrationMissingConfigMessage    = "workflow configuration path or preset name required; provide a positional argument or --config flag"
)

func TestWorkflowRunIntegration(testInstance *testing.T) {
	workingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRoot := filepath.Dir(workingDirectory)

	testCases := []struct {
		name                         string
		includePositionalWorkflowArg bool
		includeConfigFlag            bool
		useRepositoryRootConfig      bool
	}{
		{
			name:                         workflowIntegrationDefaultCaseName,
			includePositionalWorkflowArg: true,
			includeConfigFlag:            false,
			useRepositoryRootConfig:      false,
		},
		{
			name:                         workflowIntegrationConfigFlagCaseName,
			includePositionalWorkflowArg: false,
			includeConfigFlag:            true,
			useRepositoryRootConfig:      false,
		},
		{
			name:                         workflowIntegrationRepositoryConfigCase,
			includePositionalWorkflowArg: false,
			includeConfigFlag:            false,
			useRepositoryRootConfig:      true,
		},
	}

	for testCaseIndex := range testCases {
		testCase := testCases[testCaseIndex]
		subtestName := fmt.Sprintf(workflowIntegrationSubtestNameTemplate, testCaseIndex, testCase.name)

		testInstance.Run(subtestName, func(subtest *testing.T) {
			tempDirectory := subtest.TempDir()
			repositoryPath := filepath.Join(tempDirectory, "legacy")

			initializeWorkflowRepository(subtest, repositoryPath)

			stateFilePath := filepath.Join(tempDirectory, workflowIntegrationStateFileName)
			require.NoError(subtest, os.WriteFile(stateFilePath, []byte("main\n"), 0o644))

			stubDirectory := filepath.Join(tempDirectory, "bin")
			require.NoError(subtest, os.Mkdir(stubDirectory, 0o755))
			stubPath := filepath.Join(stubDirectory, workflowIntegrationStubExecutable)
			stubScript := buildWorkflowStubScript(stateFilePath)
			require.NoError(subtest, os.WriteFile(stubPath, []byte(stubScript), 0o755))

			configDirectory := tempDirectory
			configFileName := workflowIntegrationConfigFileName
			if testCase.useRepositoryRootConfig {
				configDirectory = repositoryRoot
				configFileName = integrationConfigFileNameConstant
			}

			configPath := filepath.Join(configDirectory, configFileName)
			var (
				hadExistingRepositoryConfig bool
				originalConfigContent       []byte
				originalConfigMode          os.FileMode
			)

			if testCase.useRepositoryRootConfig {
				existingInfo, existingErr := os.Stat(configPath)
				if existingErr == nil {
					hadExistingRepositoryConfig = true
					originalConfigMode = existingInfo.Mode()

					originalConfigContent, existingErr = os.ReadFile(configPath)
					require.NoError(subtest, existingErr)
				} else if !errors.Is(existingErr, os.ErrNotExist) {
					require.NoError(subtest, existingErr)
				}
			}

			auditPath := filepath.Join(tempDirectory, workflowIntegrationAuditFileName)
			workflowConfig := buildWorkflowConfiguration(auditPath)
			require.NoError(subtest, os.WriteFile(configPath, []byte(workflowConfig), 0o644))

			if testCase.useRepositoryRootConfig {
				subtest.Cleanup(func() {
					if hadExistingRepositoryConfig {
						require.NoError(subtest, os.WriteFile(configPath, originalConfigContent, originalConfigMode))
						return
					}

					removeErr := os.Remove(configPath)
					if errors.Is(removeErr, os.ErrNotExist) {
						return
					}

					require.NoError(subtest, removeErr)
				})
			}

			extendedPath := stubDirectory + string(os.PathListSeparator) + os.Getenv("PATH")

			commandArguments := []string{
				workflowIntegrationRunSubcommand,
				workflowIntegrationModulePathConstant,
				workflowIntegrationLogLevelFlag,
				workflowIntegrationErrorLevel,
			}

			if testCase.includeConfigFlag {
				commandArguments = append(commandArguments, workflowIntegrationConfigFlag, configPath)
			}

			commandArguments = append(commandArguments, workflowIntegrationCommand)

			if testCase.includePositionalWorkflowArg {
				commandArguments = append(commandArguments, configPath)
			}

			commandArguments = append(commandArguments,
				workflowIntegrationRootsFlag,
				tempDirectory,
				workflowIntegrationYesFlag,
			)

			commandOptions := integrationCommandOptions{PathVariable: extendedPath}
			rawOutput := runIntegrationCommand(subtest, repositoryRoot, commandOptions, workflowIntegrationTimeout, commandArguments)
			filteredOutput := filterStructuredOutput(rawOutput)

			expectedConversion := fmt.Sprintf(workflowIntegrationConvertExpectedTemplate, repositoryPath)
			require.Contains(subtest, filteredOutput, workflowIntegrationRemoteUpdateLine)
			expectedRemoteUpdate := workflowIntegrationRemoteSkipExpectedLine
			expectedMigration := fmt.Sprintf(workflowIntegrationDefaultExpectedTemplate, repositoryPath)
			expectedAudit := fmt.Sprintf(workflowIntegrationAuditExpectedTemplate, auditPath)

			require.Contains(subtest, filteredOutput, expectedConversion)
			require.Contains(subtest, filteredOutput, expectedRemoteUpdate)
			require.Contains(subtest, filteredOutput, expectedMigration)
			require.Contains(subtest, filteredOutput, expectedAudit)

			verifyWorkflowRepositoryState(subtest, repositoryPath, auditPath)
		})
	}
}

func TestWorkflowProcessesRepositoriesSequentially(testInstance *testing.T) {
	workingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRoot := filepath.Dir(workingDirectory)

	rootDirectory := testInstance.TempDir()
	fastRepository := filepath.Join(rootDirectory, "fast-repo")
	slowRepository := filepath.Join(rootDirectory, "slow-repo")

	initializeWorkflowRepository(testInstance, fastRepository)
	initializeWorkflowRepository(testInstance, slowRepository)
	require.NoError(testInstance, os.WriteFile(filepath.Join(fastRepository, "fast.flag"), []byte("fast\n"), 0o644))
	require.NoError(testInstance, os.WriteFile(filepath.Join(slowRepository, "slow.flag"), []byte("slow\n"), 0o644))

	stateFilePath := filepath.Join(rootDirectory, workflowIntegrationStateFileName)
	require.NoError(testInstance, os.WriteFile(stateFilePath, []byte("main\n"), 0o644))

	stubDirectory := filepath.Join(rootDirectory, "bin")
	require.NoError(testInstance, os.MkdirAll(stubDirectory, 0o755))
	stubScript := buildWorkflowStubScript(stateFilePath)
	stubPath := filepath.Join(stubDirectory, workflowIntegrationStubExecutable)
	require.NoError(testInstance, os.WriteFile(stubPath, []byte(stubScript), 0o755))

	workflowConfiguration := `workflow:
  - step:
      name: stage-one
      command: ["tasks", "apply"]
      with:
        tasks:
          - name: "Stage One"
            ensure_clean: false
            steps: [files.apply]
            files:
              - path: stage.log
                content: |
                  {{- $slow := index .Environment "slow_repo_path" }}
                  {{- $fast := index .Environment "fast_repo_path" }}
                  {{- if eq .Repository.Path $slow }}
                  stage1:slow-repo
                  slow-padding-0001
                  slow-padding-0002
                  slow-padding-0003
                  slow-padding-0004
                  slow-padding-0005
                  slow-padding-0006
                  slow-padding-0007
                  slow-padding-0008
                  slow-padding-0009
                  slow-padding-0010
                  slow-padding-0011
                  slow-padding-0012
                  slow-padding-0013
                  slow-padding-0014
                  slow-padding-0015
                  slow-padding-0016
                  slow-padding-0017
                  slow-padding-0018
                  slow-padding-0019
                  slow-padding-0020
                  {{- else if eq .Repository.Path $fast }}
                  stage1:fast-repo
                  {{- end }}
                mode: append-if-missing
  - step:
      name: stage-two
      after: ["stage-one"]
      command: ["tasks", "apply"]
      with:
        tasks:
          - name: "Stage Two"
            ensure_clean: false
            steps: [files.apply]
            files:
              - path: stage.log
                content: |
                  {{- $slow := index .Environment "slow_repo_path" }}
                  {{- $fast := index .Environment "fast_repo_path" }}
                  {{- if eq .Repository.Path $slow }}
                  stage2:slow-repo
                  {{- else if eq .Repository.Path $fast }}
                  stage2:fast-repo
                  {{- end }}
                mode: append-if-missing
`
	configurationPath := filepath.Join(rootDirectory, "sequential_workflow.yaml")
	require.NoError(testInstance, os.WriteFile(configurationPath, []byte(workflowConfiguration), 0o644))

	commandArguments := []string{
		workflowIntegrationRunSubcommand,
		workflowIntegrationModulePathConstant,
		workflowIntegrationLogLevelFlag,
		workflowIntegrationErrorLevel,
		workflowIntegrationCommand,
		configurationPath,
		workflowIntegrationRootsFlag,
		fastRepository,
		workflowIntegrationRootsFlag,
		slowRepository,
		workflowIntegrationYesFlag,
		"--var", fmt.Sprintf("slow_repo_path=%s", slowRepository),
		"--var", fmt.Sprintf("fast_repo_path=%s", fastRepository),
		"--workflow-workers=2",
	}

	commandOptions := integrationCommandOptions{
		PathVariable: stubDirectory + string(os.PathListSeparator) + os.Getenv("PATH"),
	}

	rawOutput := runIntegrationCommand(testInstance, repositoryRoot, commandOptions, workflowIntegrationTimeout, commandArguments)
	filteredOutput := filterStructuredOutput(rawOutput)

	fastLog := readStageLog(testInstance, fastRepository)
	slowLog := readStageLog(testInstance, slowRepository)

	requireStageOrder(testInstance, fastLog, "fast-repo")
	requireStageOrder(testInstance, slowLog, "slow-repo")

	require.NotContains(testInstance, filteredOutput, "TASK_SKIP", "unexpected task skips in sequential workflow validation")
}

func TestWorkflowLogHeaderFormatting(testInstance *testing.T) {
	workingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRoot := filepath.Dir(workingDirectory)

	rootDirectory := testInstance.TempDir()
	repositoryPath := filepath.Join(rootDirectory, "legacy")

	initializeWorkflowRepository(testInstance, repositoryPath)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "dirty.txt"), []byte("dirty\n"), 0o644))

	workflowConfiguration := `workflow:
  - step:
      name: task-clean
      command: ["tasks", "apply"]
      with:
        tasks:
          - name: "Ensure clean"
            safeguards:
              hard_stop:
                require_clean: true
            steps: [files.apply]
            files:
              - path: README.md
                content: "noop"
                mode: overwrite
# dirty repo triggers safeguards warning but workflow continues
`
	configPath := filepath.Join(rootDirectory, "header_format.yaml")
	require.NoError(testInstance, os.WriteFile(configPath, []byte(workflowConfiguration), 0o644))

	commandArguments := []string{
		workflowIntegrationRunSubcommand,
		workflowIntegrationModulePathConstant,
		workflowIntegrationLogLevelFlag,
		workflowIntegrationErrorLevel,
		workflowIntegrationCommand,
		configPath,
		workflowIntegrationRootsFlag,
		repositoryPath,
		workflowIntegrationYesFlag,
	}

	extendedTimeout := workflowIntegrationTimeout * 2
	outputText := runIntegrationCommand(testInstance, repositoryRoot, integrationCommandOptions{}, extendedTimeout, commandArguments)
	filteredOutput := filterStructuredOutput(outputText)

	lines := strings.Split(filteredOutput, "\n")
	foundStructuredHeader := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "-- ") {
			continue
		}
		if !strings.Contains(trimmed, ") --") {
			testInstance.Fatalf("header missing path context: %s", trimmed)
		}
		foundStructuredHeader = true
	}
	require.True(testInstance, foundStructuredHeader, "expected at least one structured header in workflow output")
}
func TestWorkflowRunDisplaysHelpWhenConfigurationMissing(testInstance *testing.T) {
	workingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRoot := filepath.Dir(workingDirectory)

	testCases := []struct {
		name             string
		arguments        []string
		expectedSnippets []string
	}{
		{
			name: workflowIntegrationHelpCaseName,
			arguments: []string{
				workflowIntegrationRunSubcommand,
				workflowIntegrationModulePathConstant,
				workflowIntegrationLogLevelFlag,
				workflowIntegrationErrorLevel,
				workflowIntegrationCommand,
			},
			expectedSnippets: []string{
				integrationHelpUsagePrefixConstant,
				workflowIntegrationUsageSnippet,
				workflowIntegrationMissingConfigMessage,
			},
		},
	}

	for testCaseIndex := range testCases {
		testCase := testCases[testCaseIndex]
		subtestName := fmt.Sprintf(workflowIntegrationSubtestNameTemplate, testCaseIndex, testCase.name)
		testInstance.Run(subtestName, func(subtest *testing.T) {
			emptyDirectory := subtest.TempDir()
			commandOptions := integrationCommandOptions{
				EnvironmentOverrides: map[string]string{
					workflowIntegrationConfigSearchPathEnvVar: emptyDirectory,
				},
			}
			outputText, _ := runFailingIntegrationCommand(subtest, repositoryRoot, commandOptions, workflowIntegrationTimeout, testCase.arguments)
			filteredOutput := filterStructuredOutput(outputText)
			for _, expectedSnippet := range testCase.expectedSnippets {
				require.Contains(subtest, filteredOutput, expectedSnippet)
			}
		})
	}
}

func initializeWorkflowRepository(testInstance *testing.T, repositoryPath string) {
	initCommand := exec.Command(workflowIntegrationGitExecutable, workflowIntegrationInitFlag, workflowIntegrationInitialBranchFlag, repositoryPath)
	initCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, initCommand.Run())

	configNameCommand := exec.Command(workflowIntegrationGitExecutable, "-C", repositoryPath, workflowIntegrationConfigUserName, workflowIntegrationUserNameKey, workflowIntegrationUserNameValue)
	configNameCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, configNameCommand.Run())

	configEmailCommand := exec.Command(workflowIntegrationGitExecutable, "-C", repositoryPath, workflowIntegrationConfigUserName, workflowIntegrationUserEmailKey, workflowIntegrationUserEmailValue)
	configEmailCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, configEmailCommand.Run())

	readmePath := filepath.Join(repositoryPath, workflowIntegrationReadmeFileName)
	require.NoError(testInstance, os.WriteFile(readmePath, []byte("hello\n"), 0o644))

	addReadme := exec.Command(workflowIntegrationGitExecutable, "-C", repositoryPath, "add", workflowIntegrationReadmeFileName)
	addReadme.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, addReadme.Run())

	commitInitial := exec.Command(workflowIntegrationGitExecutable, "-C", repositoryPath, "commit", "-m", workflowIntegrationInitialCommitMessage)
	commitInitial.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, commitInitial.Run())

	workflowsDirectory := filepath.Join(repositoryPath, workflowIntegrationWorkflowDirectory)
	require.NoError(testInstance, os.MkdirAll(workflowsDirectory, 0o755))
	workflowPath := filepath.Join(workflowsDirectory, workflowIntegrationWorkflowFileName)
	require.NoError(testInstance, os.WriteFile(workflowPath, []byte(workflowIntegrationWorkflowContent), 0o644))

	addWorkflow := exec.Command(workflowIntegrationGitExecutable, "-C", repositoryPath, "add", workflowIntegrationWorkflowDirectory)
	addWorkflow.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, addWorkflow.Run())

	commitWorkflow := exec.Command(workflowIntegrationGitExecutable, "-C", repositoryPath, "commit", "-m", workflowIntegrationWorkflowCommitMessage)
	commitWorkflow.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, commitWorkflow.Run())

	createMaster := exec.Command(workflowIntegrationGitExecutable, "-C", repositoryPath, workflowIntegrationBranchCommand, workflowIntegrationMasterBranch)
	createMaster.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, createMaster.Run())

	checkoutMaster := exec.Command(workflowIntegrationGitExecutable, "-C", repositoryPath, workflowIntegrationCheckoutCommand, workflowIntegrationMasterBranch)
	checkoutMaster.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, checkoutMaster.Run())

	remoteCommand := exec.Command(workflowIntegrationGitExecutable, "-C", repositoryPath, "remote", "add", workflowIntegrationOriginRemoteName, workflowIntegrationHTTPSRemote)
	remoteCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, remoteCommand.Run())
}

func buildWorkflowConfiguration(auditPath string) string {
	return fmt.Sprintf(`common:
  log_level: error
operations:
  - command: ["workflow"]
    with: &workflow_defaults
      roots:
        - .
      assume_yes: true
  - command: ["remote", "update-protocol"]
    with: &conversion_defaults
      roots:
        - .
      assume_yes: true
      from: https
      to: ssh
  - command: ["remote", "update-to-canonical"]
    with: &remote_defaults
      roots:
        - .
      assume_yes: true
      owner: canonical
  - command: ["default"]
    with: &migration_defaults
      roots:
        - .
      debug: false
      targets:
        - remote_name: origin
          target_branch: master
          push_to_remote: false
          delete_source_branch: false
workflow:
  - step:
      command: ["remote", "update-protocol"]
      with:
        <<: *conversion_defaults
  - step:
      command: ["remote", "update-to-canonical"]
      with:
        <<: *remote_defaults
  - step:
      command: ["default"]
      with:
        <<: *migration_defaults
  - step:
      command: ["audit", "report"]
      with:
        output: %s
`, auditPath)
}

func buildWorkflowStubScript(stateFilePath string) string {
	const template = `#!/bin/sh
STATE_FILE=%[1]q
REPO_VIEW_TEMPLATE=%[2]q
if [ "$1" = "repo" ] && [ "$2" = "view" ]; then
  DEFAULT_BRANCH=$(cat "$STATE_FILE")
  printf "$REPO_VIEW_TEMPLATE" "$DEFAULT_BRANCH"
  exit 0
fi
if [ "$1" = "pr" ] && [ "$2" = "list" ]; then
  echo '[]'
  exit 0
fi
if [ "$1" = "pr" ] && [ "$2" = "edit" ]; then
  exit 0
fi
if [ "$1" = "api" ] && [ "$2" = "repos/canonical/example/pages" ]; then
  if [ "$4" = "GET" ]; then
    echo '{"build_type":"legacy","source":{"branch":"main","path":"/"}}'
    exit 0
  fi
  if [ "$4" = "PUT" ]; then
    cat >/dev/null
    exit 0
  fi
fi
if [ "$1" = "api" ] && [ "$2" = "repos/canonical/example" ]; then
  if [ "$4" = "PATCH" ]; then
    for argument in "$@"; do
      case $argument in
        default_branch=*)
          echo "${argument#default_branch=}" >"$STATE_FILE"
          ;;
      esac
    done
  fi
  exit 0
fi
if [ "$1" = "api" ] && [ "$2" = "repos/canonical/example/branches/main/protection" ]; then
  echo 'gh: Not Found (HTTP 404)' >&2
  exit 1
fi
exit 0
`
	return fmt.Sprintf(template, stateFilePath, workflowIntegrationRepoViewJSONTemplate)
}

func verifyWorkflowRepositoryState(testInstance *testing.T, repositoryPath string, auditPath string) {
	remoteCommand := exec.Command(workflowIntegrationGitExecutable, "-C", repositoryPath, "remote", "get-url", workflowIntegrationOriginRemoteName)
	remoteCommand.Env = buildGitCommandEnvironment(nil)
	remoteOutput, remoteError := remoteCommand.CombinedOutput()
	require.NoError(testInstance, remoteError, string(remoteOutput))
	require.Equal(testInstance, "ssh://git@github.com/canonical/example.git\n", string(remoteOutput))

	workflowPath := filepath.Join(repositoryPath, workflowIntegrationWorkflowDirectory, workflowIntegrationWorkflowFileName)
	workflowBytes, workflowReadError := os.ReadFile(workflowPath)
	require.NoError(testInstance, workflowReadError)
	require.Contains(testInstance, string(workflowBytes), "- master")

	logCommand := exec.Command(workflowIntegrationGitExecutable, "-C", repositoryPath, "log", "-1", "--pretty=%s")
	logCommand.Env = buildGitCommandEnvironment(nil)
	logOutput, logError := logCommand.CombinedOutput()
	require.NoError(testInstance, logError, string(logOutput))
	require.Equal(testInstance, workflowIntegrationBranchCommitMessage+"\n", string(logOutput))

	auditBytes, auditReadError := os.ReadFile(auditPath)
	require.NoError(testInstance, auditReadError)

	reader := csv.NewReader(strings.NewReader(string(auditBytes)))
	records, parseError := reader.ReadAll()
	require.NoError(testInstance, parseError)
	require.Len(testInstance, records, 2)
	require.Equal(testInstance, strings.Split(strings.TrimSuffix(workflowIntegrationCSVHeader, "\n"), ","), records[0])
	require.Equal(testInstance, "legacy", records[1][0])
	require.Equal(testInstance, "canonical/example", records[1][1])
	require.Equal(testInstance, "no", records[1][2])
	require.Equal(testInstance, "master", records[1][3])
	require.Equal(testInstance, "master", records[1][4])
	require.Equal(testInstance, "n/a", records[1][5])
	require.Equal(testInstance, "ssh", records[1][6])
	require.Equal(testInstance, "yes", records[1][7])
}

func parseStageEntries(orderLog string) []string {
	lines := strings.Split(strings.TrimSpace(orderLog), "\n")
	entries := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		if strings.HasPrefix(trimmed, "stage1:") || strings.HasPrefix(trimmed, "stage2:") {
			entries = append(entries, trimmed)
		}
	}
	return entries
}

func findEntryIndex(entries []string, target string) int {
	for index, entry := range entries {
		if strings.Contains(entry, target) {
			return index
		}
	}
	return -1
}

func readStageLog(testInstance *testing.T, repositoryPath string) string {
	logPath := filepath.Join(repositoryPath, "stage.log")
	content, err := os.ReadFile(logPath)
	require.NoError(testInstance, err)
	return string(content)
}

func requireStageOrder(testInstance *testing.T, logContents string, label string) {
	stageEntries := parseStageEntries(logContents)
	stageOneIndex := findEntryIndex(stageEntries, fmt.Sprintf("stage1:%s", label))
	stageTwoIndex := findEntryIndex(stageEntries, fmt.Sprintf("stage2:%s", label))
	require.NotEqual(testInstance, -1, stageOneIndex, "missing stage one entry for %s. log:\n%s", label, logContents)
	require.NotEqual(testInstance, -1, stageTwoIndex, "missing stage two entry for %s. log:\n%s", label, logContents)
	require.Less(testInstance, stageOneIndex, stageTwoIndex, "stage order violated for %s. log:\n%s", label, logContents)
}
