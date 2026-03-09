package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/web"
)

const (
	testBrowserEnvironmentVariableConstant = "GIX_TEST_BROWSER"
	testServerAddressConstant              = "127.0.0.1:8080"
	browserRunTimeoutConstant              = 30 * time.Second
	browserReadyTimeoutConstant            = 10 * time.Second
	browserReadyPollIntervalConstant       = 100 * time.Millisecond
	repositoryTitleLoadingConstant         = "Loading..."

	auditRootsInputSelectorConstant        = "#audit-roots-input"
	auditIncludeAllSelectorConstant        = "#audit-include-all"
	auditRunButtonSelectorConstant         = "#task-inspect-load"
	auditResultsPanelSelectorConstant      = "#audit-results-panel"
	auditResultsSummarySelectorConstant    = "#audit-results-summary"
	auditResultsBodySelectorConstant       = "#audit-results-body"
	auditNameMatchesFilterSelectorConstant = "[data-audit-column-filter='name_matches']"
	auditQueuePanelSelectorConstant        = "#audit-queue-panel"
	auditQueueSummarySelectorConstant      = "#audit-queue-summary"
	auditQueueListSelectorConstant         = "#audit-queue-list"
	auditQueueApplySelectorConstant        = "#audit-queue-apply"
	auditQueueDeleteSelectorConstant       = "[data-audit-action='delete_folder']"
	auditQueueDeleteConfirmSelector        = "[data-queue-confirm-delete]"
	auditQueueProtocolSelectorConstant     = "[data-audit-action='convert_protocol']"
	auditQueueRenameSelectorConstant       = "[data-audit-action='rename_folder']"
	auditQueueSyncSelectorConstant         = "[data-audit-action='sync_with_remote']"
	auditQueueTargetProtocolSelector       = "[data-queue-target-protocol]"
	auditQueueSyncStrategySelector         = "[data-queue-sync-strategy]"
	repoFilterSelectorConstant             = "#repo-filter"
	repoSidebarSelectorConstant            = "#repo-sidebar"
	repoTreeSelectorConstant               = "#repo-tree"
	workspaceLayoutSelectorConstant        = "#workspace-layout"
	workspaceMainSelectorConstant          = "#workspace-main"
	branchTaskButtonSelectorConstant       = "#task-branch"
	filesTaskButtonSelectorConstant        = "#task-files"
	remotesTaskButtonSelectorConstant      = "#task-remotes"
	workflowsTaskButtonSelectorConstant    = "#task-workflows"
	advancedTaskButtonSelectorConstant     = "#task-advanced"
	scopeCheckedButtonSelectorConstant     = "#scope-checked"
	scopeAllButtonSelectorConstant         = "#scope-all"
	targetRefModeSelectorConstant          = "#target-ref-mode"
	targetRefSelectSelectorConstant        = "#target-ref-select"
	targetPathModeSelectorConstant         = "#target-path-mode"
	targetPathValueSelectorConstant        = "#target-path-value"
	fileTaskModeSelectorConstant           = "#file-task-mode"
	fileFindInputSelectorConstant          = "#file-find-input"
	fileReplaceInputSelectorConstant       = "#file-replace-input"
	fileLoadButtonSelectorConstant         = "#task-file-load"
	remoteOwnerInputSelectorConstant       = "#remote-owner-input"
	remoteLoadButtonSelectorConstant       = "#task-remote-load"
	workflowTargetInputSelectorConstant    = "#workflow-target-input"
	workflowVarsInputSelectorConstant      = "#workflow-vars-input"
	workflowVarFilesInputSelectorConstant  = "#workflow-var-files-input"
	workflowWorkersInputSelectorConstant   = "#workflow-workers-input"
	workflowRequireCleanSelectorConstant   = "#workflow-require-clean"
	workflowLoadButtonSelectorConstant     = "#task-workflow-load"
	switchTargetButtonSelectorConstant     = "#action-switch-target"
	selectedPathSelectorConstant           = "#selected-path"
	commandGroupsSelectorConstant          = "#command-groups"
	argumentsInputSelectorConstant         = "#arguments-input"
	commandPreviewSelectorConstant         = "#command-preview"
	runCommandSelectorConstant             = "#run-command"
	auditCommandPathConstant               = "gix audit"
	branchCommandPathConstant              = "gix cd"
	filesReplaceCommandPathConstant        = "gix files replace"
	remoteCanonicalCommandPathConstant     = "gix remote update-to-canonical"
	workflowCommandPathConstant            = "gix workflow"
	versionCommandPathConstant             = "gix version"
	refModeNamedConstant                   = "named"
	pathModeRelativeConstant               = "relative"
	fileTaskModeReplaceConstant            = "replace"
	repositoryReadmePathConstant           = "README.md"
	replacementFindValueConstant           = "initial"
	replacementTextValueConstant           = "updated"
	remoteOwnerValueConstant               = "mprlab"
	workflowTargetValueConstant            = "configs/proprietary-licensing.yaml"
	workflowVariableAssignmentConstant     = "license_year=2026"
	workflowVariableFileConstant           = "./vars.yaml"
	workflowWorkersValueConstant           = "3"
	auditCustomRootValueConstant           = "/tmp/browser-audit-root"
)

var browserExecutableCandidates = []string{
	"google-chrome",
	"google-chrome-stable",
	"chromium",
	"chromium-browser",
	"chrome",
	"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	"/opt/google/chrome/chrome",
}

func TestWebInterfaceBrowserPrefillsBranchAndFileTasks(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))
	createTestBranch(t, repositoryPath, "feature/demo")

	httpServer, repositoryCatalog := newBrowserTestServer(t, repositoryPath)
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(branchTaskButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(branchTaskButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(switchTargetButtonSelectorConstant, chromedp.ByQuery),
		setControlValue(targetRefModeSelectorConstant, refModeNamedConstant),
		setControlValue(targetRefSelectSelectorConstant, "master"),
		chromedp.Click(switchTargetButtonSelectorConstant, chromedp.ByQuery),
	))

	assertSelectedCommand(t, browserContext, branchCommandPathConstant)
	assertRunnerArguments(t, browserContext, []string{
		"cd",
		"master",
		"--roots",
		expectedRepository.Path,
	})

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(filesTaskButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(fileLoadButtonSelectorConstant, chromedp.ByQuery),
		setControlValue(targetRefSelectSelectorConstant, "feature/demo"),
		setControlValue(targetPathModeSelectorConstant, pathModeRelativeConstant),
		setControlValue(targetPathValueSelectorConstant, repositoryReadmePathConstant),
		setControlValue(fileTaskModeSelectorConstant, fileTaskModeReplaceConstant),
		setControlValue(fileFindInputSelectorConstant, replacementFindValueConstant),
		setControlValue(fileReplaceInputSelectorConstant, replacementTextValueConstant),
		chromedp.Click(fileLoadButtonSelectorConstant, chromedp.ByQuery),
	))

	assertSelectedCommand(t, browserContext, filesReplaceCommandPathConstant)
	assertRunnerArguments(t, browserContext, []string{
		"files",
		"replace",
		"--roots",
		expectedRepository.Path,
		"--pattern",
		repositoryReadmePathConstant,
		"--branch",
		"feature/demo",
		"--find",
		replacementFindValueConstant,
		"--replace",
		replacementTextValueConstant,
	})
}

func TestWebInterfaceBrowserInspectsAuditRootsAndDisplaysTable(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))

	httpServer, repositoryCatalog := newBrowserTestServerWithInspector(t, repositoryPath, func(_ context.Context, request web.AuditInspectionRequest) web.AuditInspectionResponse {
		return web.AuditInspectionResponse{
			Roots: request.Roots,
			Rows: []web.AuditInspectionRow{
				{
					Path:                   filepath.Join(auditCustomRootValueConstant, "example"),
					FolderName:             "example",
					IsGitRepository:        true,
					FinalGitHubRepository:  "canonical/example",
					OriginRemoteStatus:     "missing",
					NameMatches:            "no",
					RemoteDefaultBranch:    "",
					LocalBranch:            "",
					InSync:                 "n/a",
					RemoteProtocol:         "n/a",
					OriginMatchesCanonical: "n/a",
					WorktreeDirty:          "no",
					DirtyFiles:             "",
				},
			},
		}
	})
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	require.NoError(t, chromedp.Run(browserContext,
		setControlValue(auditRootsInputSelectorConstant, auditCustomRootValueConstant),
		setCheckboxValue(auditIncludeAllSelectorConstant, true),
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditResultsPanelSelectorConstant, chromedp.ByQuery),
	))

	auditSummary, auditSummaryError := readTextContent(browserContext, auditResultsSummarySelectorConstant)
	require.NoError(t, auditSummaryError)
	require.Equal(t, "1 row", auditSummary)

	auditResultsText, auditResultsError := readTextContent(browserContext, auditResultsBodySelectorConstant)
	require.NoError(t, auditResultsError)
	require.Contains(t, auditResultsText, "example")
	require.Contains(t, auditResultsText, "canonical/example")
	require.Contains(t, auditResultsText, "missing")
	require.Contains(t, auditResultsText, auditCustomRootValueConstant)
}

func TestWebInterfaceBrowserFiltersAuditRowsByColumnValue(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))

	httpServer, repositoryCatalog := newBrowserTestServerWithInspector(t, repositoryPath, func(_ context.Context, request web.AuditInspectionRequest) web.AuditInspectionResponse {
		return web.AuditInspectionResponse{
			Roots: request.Roots,
			Rows: []web.AuditInspectionRow{
				{
					Path:                   filepath.Join(auditCustomRootValueConstant, "alpha"),
					FolderName:             "alpha",
					IsGitRepository:        true,
					FinalGitHubRepository:  "canonical/alpha",
					OriginRemoteStatus:     "configured",
					NameMatches:            "yes",
					RemoteDefaultBranch:    "main",
					LocalBranch:            "main",
					InSync:                 "yes",
					RemoteProtocol:         "https",
					OriginMatchesCanonical: "yes",
					WorktreeDirty:          "no",
					DirtyFiles:             "",
				},
				{
					Path:                   filepath.Join(auditCustomRootValueConstant, "beta"),
					FolderName:             "beta",
					IsGitRepository:        true,
					FinalGitHubRepository:  "canonical/beta",
					OriginRemoteStatus:     "configured",
					NameMatches:            "no",
					RemoteDefaultBranch:    "main",
					LocalBranch:            "main",
					InSync:                 "yes",
					RemoteProtocol:         "https",
					OriginMatchesCanonical: "yes",
					WorktreeDirty:          "no",
					DirtyFiles:             "",
				},
			},
		}
	})
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	require.NoError(t, chromedp.Run(browserContext,
		setControlValue(auditRootsInputSelectorConstant, auditCustomRootValueConstant),
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditResultsPanelSelectorConstant, chromedp.ByQuery),
	))

	require.NoError(t, chromedp.Run(browserContext,
		setControlValue(auditNameMatchesFilterSelectorConstant, "no"),
	))

	auditSummary, auditSummaryError := readTextContent(browserContext, auditResultsSummarySelectorConstant)
	require.NoError(t, auditSummaryError)
	require.Equal(t, "1 of 2 rows", auditSummary)

	auditResultsText, auditResultsError := readTextContent(browserContext, auditResultsBodySelectorConstant)
	require.NoError(t, auditResultsError)
	require.Contains(t, auditResultsText, "beta")
	require.NotContains(t, auditResultsText, "alpha")
}

func TestWebInterfaceBrowserRunButtonUsesActionableAuditInspection(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))

	httpServer, repositoryCatalog := newBrowserTestServerWithInspector(t, repositoryPath, func(_ context.Context, request web.AuditInspectionRequest) web.AuditInspectionResponse {
		return web.AuditInspectionResponse{
			Roots: request.Roots,
			Rows: []web.AuditInspectionRow{
				{
					Path:                   filepath.Join(auditCustomRootValueConstant, "example"),
					FolderName:             "example",
					IsGitRepository:        true,
					FinalGitHubRepository:  "canonical/example",
					OriginRemoteStatus:     "missing",
					NameMatches:            "no",
					RemoteDefaultBranch:    "",
					LocalBranch:            "",
					InSync:                 "n/a",
					RemoteProtocol:         "n/a",
					OriginMatchesCanonical: "n/a",
					WorktreeDirty:          "no",
					DirtyFiles:             "",
				},
			},
		}
	})
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(runCommandSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)
	assertSelectedCommand(t, browserContext, auditCommandPathConstant)

	require.NoError(t, chromedp.Run(browserContext,
		setControlValue(auditRootsInputSelectorConstant, auditCustomRootValueConstant),
		chromedp.Click(runCommandSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditResultsPanelSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditQueueRenameSelectorConstant, chromedp.ByQuery),
	))

	runButtonLabel, runButtonLabelError := readTextContent(browserContext, runCommandSelectorConstant)
	require.NoError(t, runButtonLabelError)
	require.Equal(t, "Inspect audit table", runButtonLabel)
}

func TestWebInterfaceBrowserInspectionUsesEditedAuditArguments(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))
	alternateRoot := "/Users/tyemirov/Development/marcoPoloResearchLab"

	httpServer, repositoryCatalog := newBrowserTestServerWithInspector(t, repositoryPath, func(_ context.Context, request web.AuditInspectionRequest) web.AuditInspectionResponse {
		return web.AuditInspectionResponse{
			Roots: request.Roots,
			Rows: []web.AuditInspectionRow{
				{
					Path:                   filepath.Join(request.Roots[0], "example"),
					FolderName:             "example",
					IsGitRepository:        true,
					FinalGitHubRepository:  "canonical/example",
					OriginRemoteStatus:     "configured",
					NameMatches:            "yes",
					RemoteDefaultBranch:    "main",
					LocalBranch:            "main",
					InSync:                 "yes",
					RemoteProtocol:         "https",
					OriginMatchesCanonical: "yes",
					WorktreeDirty:          "no",
					DirtyFiles:             "",
				},
			},
		}
	})
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)
	assertSelectedCommand(t, browserContext, auditCommandPathConstant)

	require.NoError(t, chromedp.Run(browserContext,
		setControlValue(argumentsInputSelectorConstant, "audit\n--roots\n"+alternateRoot),
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditResultsPanelSelectorConstant, chromedp.ByQuery),
	))

	auditResultsText, auditResultsError := readTextContent(browserContext, auditResultsBodySelectorConstant)
	require.NoError(t, auditResultsError)
	require.Contains(t, auditResultsText, alternateRoot)
	require.NotContains(t, auditResultsText, expectedRepository.Path)
}

func TestWebInterfaceBrowserQueuesRenameChangeAndAppliesIt(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))

	renameQueued := false
	httpServer, repositoryCatalog := newBrowserTestServerWithAuditHandlers(
		t,
		repositoryPath,
		func(_ context.Context, request web.AuditInspectionRequest) web.AuditInspectionResponse {
			nameMatchStatus := "no"
			if renameQueued {
				nameMatchStatus = "yes"
			}
			return web.AuditInspectionResponse{
				Roots: request.Roots,
				Rows: []web.AuditInspectionRow{
					{
						Path:                   filepath.Join(auditCustomRootValueConstant, "example"),
						FolderName:             "example",
						IsGitRepository:        true,
						FinalGitHubRepository:  "canonical/example",
						OriginRemoteStatus:     "configured",
						NameMatches:            nameMatchStatus,
						RemoteDefaultBranch:    "main",
						LocalBranch:            "main",
						InSync:                 "yes",
						RemoteProtocol:         "https",
						OriginMatchesCanonical: "yes",
						WorktreeDirty:          "no",
						DirtyFiles:             "",
					},
				},
			}
		},
		func(_ context.Context, request web.AuditChangeApplyRequest) web.AuditChangeApplyResponse {
			if len(request.Changes) == 1 && request.Changes[0].Kind == "rename_folder" {
				renameQueued = true
			}
			return web.AuditChangeApplyResponse{
				Results: []web.AuditChangeApplyResult{
					{
						ID:      request.Changes[0].ID,
						Kind:    request.Changes[0].Kind,
						Path:    request.Changes[0].Path,
						Status:  "succeeded",
						Message: "rename applied",
					},
				},
			}
		},
	)
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	require.NoError(t, chromedp.Run(browserContext,
		setControlValue(auditRootsInputSelectorConstant, auditCustomRootValueConstant),
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditResultsPanelSelectorConstant, chromedp.ByQuery),
		chromedp.Click(auditQueueRenameSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditQueuePanelSelectorConstant, chromedp.ByQuery),
	))

	queueSummary, queueSummaryError := readTextContent(browserContext, auditQueueSummarySelectorConstant)
	require.NoError(t, queueSummaryError)
	require.Equal(t, "1 pending change", queueSummary)

	queueText, queueTextError := readTextContent(browserContext, auditQueueListSelectorConstant)
	require.NoError(t, queueTextError)
	require.Contains(t, queueText, "Rename folder")
	require.Contains(t, queueText, "canonical/example")

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(auditQueueApplySelectorConstant, chromedp.ByQuery),
	))

	require.Eventually(t, func() bool {
		summaryText, summaryError := readTextContent(browserContext, auditQueueSummarySelectorConstant)
		if summaryError != nil {
			return false
		}
		return summaryText == "0 pending changes"
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)

	auditResultsText, auditResultsError := readTextContent(browserContext, auditResultsBodySelectorConstant)
	require.NoError(t, auditResultsError)
	require.Contains(t, auditResultsText, "yes")
}

func TestWebInterfaceBrowserAppliesQueueUsingLastInspectedScope(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))

	renameQueued := false
	alternateRoot := "/tmp/browser-audit-root-alternate"
	httpServer, repositoryCatalog := newBrowserTestServerWithAuditHandlers(
		t,
		repositoryPath,
		func(_ context.Context, request web.AuditInspectionRequest) web.AuditInspectionResponse {
			if len(request.Roots) == 1 && request.Roots[0] == alternateRoot {
				return web.AuditInspectionResponse{
					Roots: request.Roots,
					Rows: []web.AuditInspectionRow{
						{
							Path:                   filepath.Join(alternateRoot, "other"),
							FolderName:             "other",
							IsGitRepository:        true,
							FinalGitHubRepository:  "canonical/other",
							OriginRemoteStatus:     "configured",
							NameMatches:            "no",
							RemoteDefaultBranch:    "main",
							LocalBranch:            "main",
							InSync:                 "yes",
							RemoteProtocol:         "https",
							OriginMatchesCanonical: "yes",
							WorktreeDirty:          "no",
							DirtyFiles:             "",
						},
					},
				}
			}

			nameMatchStatus := "no"
			if renameQueued {
				nameMatchStatus = "yes"
			}
			return web.AuditInspectionResponse{
				Roots: request.Roots,
				Rows: []web.AuditInspectionRow{
					{
						Path:                   filepath.Join(auditCustomRootValueConstant, "example"),
						FolderName:             "example",
						IsGitRepository:        true,
						FinalGitHubRepository:  "canonical/example",
						OriginRemoteStatus:     "configured",
						NameMatches:            nameMatchStatus,
						RemoteDefaultBranch:    "main",
						LocalBranch:            "main",
						InSync:                 "yes",
						RemoteProtocol:         "https",
						OriginMatchesCanonical: "yes",
						WorktreeDirty:          "no",
						DirtyFiles:             "",
					},
				},
			}
		},
		func(_ context.Context, request web.AuditChangeApplyRequest) web.AuditChangeApplyResponse {
			require.Len(t, request.Changes, 1)
			require.Equal(t, web.AuditChangeKindRenameFolder, request.Changes[0].Kind)
			renameQueued = true
			return web.AuditChangeApplyResponse{
				Results: []web.AuditChangeApplyResult{
					{
						ID:      request.Changes[0].ID,
						Kind:    request.Changes[0].Kind,
						Path:    request.Changes[0].Path,
						Status:  "succeeded",
						Message: "rename applied",
					},
				},
			}
		},
	)
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	require.NoError(t, chromedp.Run(browserContext,
		setControlValue(auditRootsInputSelectorConstant, auditCustomRootValueConstant),
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditResultsPanelSelectorConstant, chromedp.ByQuery),
		chromedp.Click(auditQueueRenameSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditQueuePanelSelectorConstant, chromedp.ByQuery),
		setControlValue(auditRootsInputSelectorConstant, alternateRoot),
		chromedp.Click(auditQueueApplySelectorConstant, chromedp.ByQuery),
	))

	require.Eventually(t, func() bool {
		summaryText, summaryError := readTextContent(browserContext, auditQueueSummarySelectorConstant)
		if summaryError != nil {
			return false
		}
		return summaryText == "0 pending changes"
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)

	auditResultsText, auditResultsError := readTextContent(browserContext, auditResultsBodySelectorConstant)
	require.NoError(t, auditResultsError)
	require.Contains(t, auditResultsText, auditCustomRootValueConstant)
	require.Contains(t, auditResultsText, "yes")
	require.NotContains(t, auditResultsText, alternateRoot)
}

func TestWebInterfaceBrowserQueuesDeleteChangeAndRequiresConfirmation(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))

	deleteApplied := false
	httpServer, repositoryCatalog := newBrowserTestServerWithAuditHandlers(
		t,
		repositoryPath,
		func(_ context.Context, request web.AuditInspectionRequest) web.AuditInspectionResponse {
			rows := []web.AuditInspectionRow{
				{
					Path:                   filepath.Join(auditCustomRootValueConstant, "example"),
					FolderName:             "example",
					IsGitRepository:        true,
					FinalGitHubRepository:  "canonical/example",
					OriginRemoteStatus:     "configured",
					NameMatches:            "yes",
					RemoteDefaultBranch:    "main",
					LocalBranch:            "main",
					InSync:                 "yes",
					RemoteProtocol:         "https",
					OriginMatchesCanonical: "yes",
					WorktreeDirty:          "no",
					DirtyFiles:             "",
				},
			}
			if deleteApplied {
				rows = nil
			}
			return web.AuditInspectionResponse{
				Roots: request.Roots,
				Rows:  rows,
			}
		},
		func(_ context.Context, request web.AuditChangeApplyRequest) web.AuditChangeApplyResponse {
			require.Len(t, request.Changes, 1)
			require.Equal(t, web.AuditChangeKindDeleteFolder, request.Changes[0].Kind)
			require.True(t, request.Changes[0].ConfirmDelete)
			deleteApplied = true
			return web.AuditChangeApplyResponse{
				Results: []web.AuditChangeApplyResult{
					{
						ID:      request.Changes[0].ID,
						Kind:    request.Changes[0].Kind,
						Path:    request.Changes[0].Path,
						Status:  "succeeded",
						Message: "delete applied",
					},
				},
			}
		},
	)
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	require.NoError(t, chromedp.Run(browserContext,
		setControlValue(auditRootsInputSelectorConstant, auditCustomRootValueConstant),
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditResultsPanelSelectorConstant, chromedp.ByQuery),
		chromedp.Click(auditQueueDeleteSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditQueuePanelSelectorConstant, chromedp.ByQuery),
	))

	queueSummary, queueSummaryError := readTextContent(browserContext, auditQueueSummarySelectorConstant)
	require.NoError(t, queueSummaryError)
	require.Equal(t, "1 pending change", queueSummary)

	queueText, queueTextError := readTextContent(browserContext, auditQueueListSelectorConstant)
	require.NoError(t, queueTextError)
	require.Contains(t, queueText, "Delete folder")
	require.Contains(t, queueText, auditCustomRootValueConstant)

	applyDisabled, applyDisabledError := readDisabledState(browserContext, auditQueueApplySelectorConstant)
	require.NoError(t, applyDisabledError)
	require.True(t, applyDisabled)

	require.NoError(t, chromedp.Run(browserContext,
		setCheckboxValue(auditQueueDeleteConfirmSelector, true),
	))

	require.Eventually(t, func() bool {
		disabled, disabledError := readDisabledState(browserContext, auditQueueApplySelectorConstant)
		if disabledError != nil {
			return false
		}
		return !disabled
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(auditQueueApplySelectorConstant, chromedp.ByQuery),
	))

	require.Eventually(t, func() bool {
		summaryText, summaryError := readTextContent(browserContext, auditQueueSummarySelectorConstant)
		if summaryError != nil {
			return false
		}
		return summaryText == "0 pending changes"
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)

	auditSummary, auditSummaryError := readTextContent(browserContext, auditResultsSummarySelectorConstant)
	require.NoError(t, auditSummaryError)
	require.Equal(t, "0 rows", auditSummary)
}

func TestWebInterfaceBrowserQueuesProtocolAndSyncChangesWithEditableOptions(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))

	protocolUpdated := false
	syncUpdated := false
	httpServer, repositoryCatalog := newBrowserTestServerWithAuditHandlers(
		t,
		repositoryPath,
		func(_ context.Context, request web.AuditInspectionRequest) web.AuditInspectionResponse {
			remoteProtocol := "https"
			inSyncStatus := "no"
			if protocolUpdated {
				remoteProtocol = "ssh"
			}
			if syncUpdated {
				inSyncStatus = "yes"
			}
			return web.AuditInspectionResponse{
				Roots: request.Roots,
				Rows: []web.AuditInspectionRow{
					{
						Path:                   filepath.Join(auditCustomRootValueConstant, "example"),
						FolderName:             "example",
						IsGitRepository:        true,
						FinalGitHubRepository:  "canonical/example",
						OriginRemoteStatus:     "configured",
						NameMatches:            "yes",
						RemoteDefaultBranch:    "main",
						LocalBranch:            "feature/demo",
						InSync:                 inSyncStatus,
						RemoteProtocol:         remoteProtocol,
						OriginMatchesCanonical: "yes",
						WorktreeDirty:          "no",
						DirtyFiles:             "",
					},
				},
			}
		},
		func(_ context.Context, request web.AuditChangeApplyRequest) web.AuditChangeApplyResponse {
			require.Len(t, request.Changes, 2)

			for _, change := range request.Changes {
				switch change.Kind {
				case web.AuditChangeKindConvertProtocol:
					require.Equal(t, "https", change.SourceProtocol)
					require.Equal(t, "ssh", change.TargetProtocol)
					protocolUpdated = true
				case web.AuditChangeKindSyncWithRemote:
					require.Equal(t, web.AuditChangeSyncStrategyStashChanges, change.SyncStrategy)
					syncUpdated = true
				default:
					t.Fatalf("unexpected change kind %s", change.Kind)
				}
			}

			return web.AuditChangeApplyResponse{
				Results: []web.AuditChangeApplyResult{
					{
						ID:      request.Changes[0].ID,
						Kind:    request.Changes[0].Kind,
						Path:    request.Changes[0].Path,
						Status:  "succeeded",
						Message: "change applied",
					},
					{
						ID:      request.Changes[1].ID,
						Kind:    request.Changes[1].Kind,
						Path:    request.Changes[1].Path,
						Status:  "succeeded",
						Message: "change applied",
					},
				},
			}
		},
	)
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	require.NoError(t, chromedp.Run(browserContext,
		setControlValue(auditRootsInputSelectorConstant, auditCustomRootValueConstant),
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditResultsPanelSelectorConstant, chromedp.ByQuery),
		chromedp.Click(auditQueueProtocolSelectorConstant, chromedp.ByQuery),
		chromedp.Click(auditQueueSyncSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditQueuePanelSelectorConstant, chromedp.ByQuery),
	))

	require.NoError(t, chromedp.Run(browserContext,
		setControlValue(auditQueueTargetProtocolSelector, "ssh"),
		setControlValue(auditQueueSyncStrategySelector, web.AuditChangeSyncStrategyStashChanges),
	))

	queueSummary, queueSummaryError := readTextContent(browserContext, auditQueueSummarySelectorConstant)
	require.NoError(t, queueSummaryError)
	require.Equal(t, "2 pending changes", queueSummary)

	queueText, queueTextError := readTextContent(browserContext, auditQueueListSelectorConstant)
	require.NoError(t, queueTextError)
	require.Contains(t, queueText, "Fix protocol")
	require.Contains(t, queueText, "Sync with remote")

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(auditQueueApplySelectorConstant, chromedp.ByQuery),
	))

	require.Eventually(t, func() bool {
		summaryText, summaryError := readTextContent(browserContext, auditQueueSummarySelectorConstant)
		if summaryError != nil {
			return false
		}
		return summaryText == "0 pending changes"
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)

	auditResultsText, auditResultsError := readTextContent(browserContext, auditResultsBodySelectorConstant)
	require.NoError(t, auditResultsError)
	require.Contains(t, auditResultsText, "ssh")
	require.Contains(t, auditResultsText, "yes")
}

func TestWebInterfaceBrowserPrefillsRemoteAndWorkflowTasksAcrossRepositoryScope(t *testing.T) {
	rootPath := t.TempDir()
	firstRepositoryPath := createTestRepository(t, filepath.Join(rootPath, "alpha"))
	secondRepositoryPath := createTestRepository(t, filepath.Join(rootPath, "nested", "beta"))
	createTestBranch(t, secondRepositoryPath, "feature/demo")
	firstCanonicalRepositoryPath := canonicalPath(t, firstRepositoryPath)
	secondCanonicalRepositoryPath := canonicalPath(t, secondRepositoryPath)

	httpServer, repositoryCatalog := newBrowserTestServer(t, rootPath)
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(remotesTaskButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(scopeAllButtonSelectorConstant, chromedp.ByQuery),
		chromedp.Click(remotesTaskButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(remoteLoadButtonSelectorConstant, chromedp.ByQuery),
		setControlValue(remoteOwnerInputSelectorConstant, remoteOwnerValueConstant),
		chromedp.Click(remoteLoadButtonSelectorConstant, chromedp.ByQuery),
	))

	assertSelectedCommand(t, browserContext, remoteCanonicalCommandPathConstant)
	assertRunnerArguments(t, browserContext, []string{
		"remote",
		"update-to-canonical",
		"--owner",
		remoteOwnerValueConstant,
		"--roots",
		firstCanonicalRepositoryPath,
		"--roots",
		secondCanonicalRepositoryPath,
	})

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(workflowsTaskButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(workflowLoadButtonSelectorConstant, chromedp.ByQuery),
		setControlValue(workflowTargetInputSelectorConstant, workflowTargetValueConstant),
		setControlValue(workflowVarsInputSelectorConstant, workflowVariableAssignmentConstant),
		setControlValue(workflowVarFilesInputSelectorConstant, workflowVariableFileConstant),
		setControlValue(workflowWorkersInputSelectorConstant, workflowWorkersValueConstant),
		setCheckboxValue(workflowRequireCleanSelectorConstant, true),
		chromedp.Click(workflowLoadButtonSelectorConstant, chromedp.ByQuery),
	))

	assertSelectedCommand(t, browserContext, workflowCommandPathConstant)
	assertRunnerArguments(t, browserContext, []string{
		"workflow",
		workflowTargetValueConstant,
		"--require-clean",
		"--var",
		workflowVariableAssignmentConstant,
		"--var-file",
		workflowVariableFileConstant,
		"--workflow-workers",
		workflowWorkersValueConstant,
		"--roots",
		firstCanonicalRepositoryPath,
		"--roots",
		secondCanonicalRepositoryPath,
	})
}

func TestWebInterfaceBrowserRendersRepositoryTreeAndPreservesCheckedScopeAcrossFilter(t *testing.T) {
	rootPath := t.TempDir()
	firstRepositoryPath := createTestRepository(t, filepath.Join(rootPath, "alpha"))
	secondRepositoryPath := createTestRepository(t, filepath.Join(rootPath, "nested", "beta"))
	secondCanonicalRepositoryPath := canonicalPath(t, secondRepositoryPath)

	httpServer, repositoryCatalog := newBrowserTestServer(t, rootPath)
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(repoTreeSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	require.Eventually(t, func() bool {
		treeText, treeTextError := readTextContent(browserContext, repoTreeSelectorConstant)
		if treeTextError != nil {
			return false
		}
		return strings.Contains(treeText, "alpha") && strings.Contains(treeText, "nested")
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)

	require.NoError(t, chromedp.Run(browserContext,
		doubleClickRepositoryTreeTitle("nested"),
		clickRepositoryTreeTitle("beta"),
	))

	require.Eventually(t, func() bool {
		repositoryTitle, repositoryTitleError := readTextContent(browserContext, "#repo-title")
		if repositoryTitleError != nil {
			return false
		}
		return repositoryTitle == "beta"
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)

	require.NoError(t, chromedp.Run(browserContext,
		clickRepositoryTreeCheckbox("alpha"),
		clickRepositoryTreeCheckbox("beta"),
		setControlValue(repoFilterSelectorConstant, "alpha"),
	))

	filteredTreeText, filteredTreeTextError := readTextContent(browserContext, repoTreeSelectorConstant)
	require.NoError(t, filteredTreeTextError)
	require.Contains(t, filteredTreeText, "alpha")
	require.NotContains(t, filteredTreeText, "beta")

	require.NoError(t, chromedp.Run(browserContext,
		setControlValue(repoFilterSelectorConstant, ""),
		chromedp.Click(scopeCheckedButtonSelectorConstant, chromedp.ByQuery),
		chromedp.Click(remotesTaskButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(remoteLoadButtonSelectorConstant, chromedp.ByQuery),
		setControlValue(remoteOwnerInputSelectorConstant, remoteOwnerValueConstant),
		chromedp.Click(remoteLoadButtonSelectorConstant, chromedp.ByQuery),
	))

	assertSelectedCommand(t, browserContext, remoteCanonicalCommandPathConstant)
	assertRunnerArguments(t, browserContext, []string{
		"remote",
		"update-to-canonical",
		"--owner",
		remoteOwnerValueConstant,
		"--roots",
		secondCanonicalRepositoryPath,
	})
	require.NotEqual(t, canonicalPath(t, firstRepositoryPath), secondCanonicalRepositoryPath)
}

func TestWebInterfaceBrowserDisplaysRepositoryTreeInLeftSidebar(t *testing.T) {
	rootPath := t.TempDir()
	createTestRepository(t, filepath.Join(rootPath, "alpha"))
	createTestRepository(t, filepath.Join(rootPath, "nested", "beta"))

	httpServer, repositoryCatalog := newBrowserTestServer(t, rootPath)
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(repoSidebarSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(repoTreeSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	require.Eventually(t, func() bool {
		treeText, treeTextError := readTextContent(browserContext, repoTreeSelectorConstant)
		if treeTextError != nil {
			return false
		}
		return strings.Contains(treeText, "alpha") && strings.Contains(treeText, "nested")
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)

	var layoutMetrics struct {
		SidebarWidthRatio float64 `json:"sidebarWidthRatio"`
		TreeHeight        float64 `json:"treeHeight"`
		SidebarLeft       float64 `json:"sidebarLeft"`
		MainLeft          float64 `json:"mainLeft"`
	}
	require.NoError(t, chromedp.Run(browserContext, chromedp.Evaluate(fmt.Sprintf(`(() => {
		const workspaceLayout = document.querySelector(%q);
		const repoSidebar = document.querySelector(%q);
		const workspaceMain = document.querySelector(%q);
		const repoTree = document.querySelector(%q);
		if (!workspaceLayout || !repoSidebar || !workspaceMain || !repoTree) {
			throw new Error("missing layout elements");
		}
		const workspaceLayoutRect = workspaceLayout.getBoundingClientRect();
		const repoSidebarRect = repoSidebar.getBoundingClientRect();
		const workspaceMainRect = workspaceMain.getBoundingClientRect();
		const repoTreeRect = repoTree.getBoundingClientRect();
		return {
			sidebarWidthRatio: repoSidebarRect.width / workspaceLayoutRect.width,
			treeHeight: repoTreeRect.height,
			sidebarLeft: repoSidebarRect.left,
			mainLeft: workspaceMainRect.left
		};
	})()`, workspaceLayoutSelectorConstant, repoSidebarSelectorConstant, workspaceMainSelectorConstant, repoTreeSelectorConstant), &layoutMetrics)))

	require.Greater(t, layoutMetrics.SidebarWidthRatio, 0.17)
	require.Less(t, layoutMetrics.SidebarWidthRatio, 0.23)
	require.Greater(t, layoutMetrics.TreeHeight, 180.0)
	require.Less(t, layoutMetrics.SidebarLeft, layoutMetrics.MainLeft)
}

func TestWebInterfaceBrowserCurrentRepoModeShowsParentFolderInTree(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "fleet", "workspace", "example"))

	httpServer, repositoryCatalog := newBrowserTestServer(t, repositoryPath)
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(repoTreeSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	grandparentFolderName := filepath.Base(filepath.Dir(filepath.Dir(repositoryPath)))
	parentFolderName := filepath.Base(filepath.Dir(repositoryPath))
	repositoryName := filepath.Base(repositoryPath)

	treeText, treeTextError := readTextContent(browserContext, repoTreeSelectorConstant)
	require.NoError(t, treeTextError)
	require.NotContains(t, treeText, grandparentFolderName)
	require.Contains(t, treeText, parentFolderName)
	require.Contains(t, treeText, repositoryName)

	require.NoError(t, chromedp.Run(browserContext,
		clickRepositoryTreeTitle(parentFolderName),
	))

	require.Eventually(t, func() bool {
		expandedTreeText, expandedTreeError := readTextContent(browserContext, repoTreeSelectorConstant)
		if expandedTreeError != nil {
			return false
		}
		return strings.Contains(expandedTreeText, grandparentFolderName)
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)
}

func TestWebInterfaceBrowserCurrentRepoLeafExpandsToSiblingRepositories(t *testing.T) {
	rootPath := t.TempDir()
	repositoryPath := createTestRepository(t, filepath.Join(rootPath, "workspace", "example"))
	createTestRepository(t, filepath.Join(rootPath, "workspace", "other"))

	httpServer, repositoryCatalog := newBrowserTestServer(t, repositoryPath)
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(repoTreeSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	initialTreeText, initialTreeError := readTextContent(browserContext, repoTreeSelectorConstant)
	require.NoError(t, initialTreeError)
	require.Contains(t, initialTreeText, "example")
	require.NotContains(t, initialTreeText, "other")

	require.NoError(t, chromedp.Run(browserContext,
		clickRepositoryTreeTitle("example"),
	))

	require.Eventually(t, func() bool {
		treeText, treeTextError := readTextContent(browserContext, repoTreeSelectorConstant)
		if treeTextError != nil {
			return false
		}
		return strings.Contains(treeText, "other")
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)
}

func TestWebInterfaceBrowserRepositoryTreeShowsOnlyTopLevelRepositories(t *testing.T) {
	rootPath := t.TempDir()
	topLevelRepositoryPath := createTestRepository(t, filepath.Join(rootPath, "alpha"))
	nestedRepositoryPath := createTestRepository(t, filepath.Join(topLevelRepositoryPath, "plugins", "child"))
	siblingRepositoryPath := createTestRepository(t, filepath.Join(rootPath, "gamma"))
	topLevelCanonicalRepositoryPath := canonicalPath(t, topLevelRepositoryPath)
	siblingCanonicalRepositoryPath := canonicalPath(t, siblingRepositoryPath)

	httpServer, repositoryCatalog := newBrowserTestServer(t, rootPath)
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(repoTreeSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	treeText, treeTextError := readTextContent(browserContext, repoTreeSelectorConstant)
	require.NoError(t, treeTextError)
	require.Contains(t, treeText, "alpha")
	require.Contains(t, treeText, "gamma")
	require.NotContains(t, treeText, "child")

	repoCountText, repoCountError := readTextContent(browserContext, "#repo-count")
	require.NoError(t, repoCountError)
	require.Equal(t, "2", repoCountText)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(scopeAllButtonSelectorConstant, chromedp.ByQuery),
		chromedp.Click(remotesTaskButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(remoteLoadButtonSelectorConstant, chromedp.ByQuery),
		setControlValue(remoteOwnerInputSelectorConstant, remoteOwnerValueConstant),
		chromedp.Click(remoteLoadButtonSelectorConstant, chromedp.ByQuery),
	))

	assertSelectedCommand(t, browserContext, remoteCanonicalCommandPathConstant)
	assertRunnerArguments(t, browserContext, []string{
		"remote",
		"update-to-canonical",
		"--owner",
		remoteOwnerValueConstant,
		"--roots",
		topLevelCanonicalRepositoryPath,
		"--roots",
		siblingCanonicalRepositoryPath,
	})
	require.NotEqual(t, topLevelCanonicalRepositoryPath, canonicalPath(t, nestedRepositoryPath))
}

func TestBrowserStartupErrorSkippable(t *testing.T) {
	startupError := errors.New(
		"chrome failed to start:\n" +
			"[0308/223413.260786:ERROR:third_party/crashpad/crashpad/util/file/file_io_posix.cc:145] open /sys/devices/system/cpu/cpu0/cpufreq/scaling_cur_freq: No such file or directory (2)\n",
	)

	require.True(t, browserStartupErrorSkippable(startupError))
	require.False(t, browserStartupErrorSkippable(errors.New("selector did not resolve")))
	require.False(t, browserStartupErrorSkippable(nil))
}

func TestWebInterfaceBrowserAdvancedHidesTaskOwnedCommands(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))

	httpServer, repositoryCatalog := newBrowserTestServer(t, repositoryPath)
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(workflowsTaskButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(workflowsTaskButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(workflowLoadButtonSelectorConstant, chromedp.ByQuery),
		chromedp.Click(workflowLoadButtonSelectorConstant, chromedp.ByQuery),
	))
	assertSelectedCommand(t, browserContext, workflowCommandPathConstant)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(advancedTaskButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(commandGroupsSelectorConstant, chromedp.ByQuery),
	))
	assertSelectedCommand(t, browserContext, versionCommandPathConstant)

	commandGroupsText, commandGroupsError := readTextContent(browserContext, commandGroupsSelectorConstant)
	require.NoError(t, commandGroupsError)
	assert.Contains(t, commandGroupsText, versionCommandPathConstant)
	assert.NotContains(t, commandGroupsText, auditCommandPathConstant)
	assert.NotContains(t, commandGroupsText, branchCommandPathConstant)
	assert.NotContains(t, commandGroupsText, filesReplaceCommandPathConstant)
	assert.NotContains(t, commandGroupsText, remoteCanonicalCommandPathConstant)
	assert.NotContains(t, commandGroupsText, workflowCommandPathConstant)
}

func newBrowserTestServer(testingInstance *testing.T, workingDirectory string) (*httptest.Server, web.RepositoryCatalog) {
	return newBrowserTestServerWithOptions(testingInstance, workingDirectory, nil, nil, nil)
}

func newBrowserTestServerWithInspector(testingInstance *testing.T, workingDirectory string, inspectAudit web.AuditInspector) (*httptest.Server, web.RepositoryCatalog) {
	return newBrowserTestServerWithOptions(testingInstance, workingDirectory, nil, inspectAudit, nil)
}

func newBrowserTestServerWithAuditHandlers(
	testingInstance *testing.T,
	workingDirectory string,
	inspectAudit web.AuditInspector,
	applyAuditChanges web.AuditChangeExecutor,
) (*httptest.Server, web.RepositoryCatalog) {
	return newBrowserTestServerWithOptions(testingInstance, workingDirectory, nil, inspectAudit, applyAuditChanges)
}

func newBrowserTestServerWithOptions(
	testingInstance *testing.T,
	workingDirectory string,
	execute web.CommandExecutor,
	inspectAudit web.AuditInspector,
	applyAuditChanges web.AuditChangeExecutor,
) (*httptest.Server, web.RepositoryCatalog) {
	testingInstance.Helper()

	var httpServer *httptest.Server
	var repositoryCatalog web.RepositoryCatalog

	withWorkingDirectory(testingInstance, workingDirectory, func() {
		application := NewApplication()
		repositoryCatalog = application.repositoryCatalog(context.Background())
		commandExecutor := execute
		if commandExecutor == nil {
			commandExecutor = application.newWebCommandExecutor()
		}
		auditInspector := inspectAudit
		if auditInspector == nil {
			auditInspector = application.newWebAuditInspector()
		}
		auditChangeExecutor := applyAuditChanges
		if auditChangeExecutor == nil {
			auditChangeExecutor = application.newWebAuditChangeExecutor()
		}

		server, serverError := web.NewServer(web.ServerOptions{
			Address:           testServerAddressConstant,
			Repositories:      repositoryCatalog,
			Catalog:           application.commandCatalog(),
			LoadBranches:      application.loadRepositoryBranches,
			Execute:           commandExecutor,
			InspectAudit:      auditInspector,
			ApplyAuditChanges: auditChangeExecutor,
		})
		require.NoError(testingInstance, serverError)

		httpServer = httptest.NewServer(server.Handler())
	})

	require.NotNil(testingInstance, httpServer)
	return httpServer, repositoryCatalog
}

func newBrowserTestContext(testingInstance *testing.T) context.Context {
	testingInstance.Helper()

	browserExecutable := locateBrowserExecutable()
	if len(browserExecutable) == 0 {
		testingInstance.Skip("skipping browser test: no Chrome or Chromium executable was found")
	}

	allocatorOptions := append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(browserExecutable),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-breakpad", true),
		chromedp.Flag("disable-crash-reporter", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("window-size", "1440,1100"),
	)

	allocatorContext, cancelAllocator := chromedp.NewExecAllocator(context.Background(), allocatorOptions...)
	browserContext, cancelBrowser := chromedp.NewContext(allocatorContext)
	timeoutContext, cancelTimeout := context.WithTimeout(browserContext, browserRunTimeoutConstant)

	testingInstance.Cleanup(func() {
		cancelTimeout()
		cancelBrowser()
		cancelAllocator()
	})

	startupError := chromedp.Run(timeoutContext, chromedp.Navigate("about:blank"))
	if browserStartupErrorSkippable(startupError) {
		testingInstance.Skipf("skipping browser test: Chrome failed to start in this environment: %v", startupError)
	}
	require.NoError(testingInstance, startupError)

	return timeoutContext
}

func browserStartupErrorSkippable(startupError error) bool {
	if startupError == nil {
		return false
	}

	startupErrorLower := strings.ToLower(startupError.Error())
	return strings.Contains(startupErrorLower, "chrome failed to start")
}

func locateBrowserExecutable() string {
	configuredBrowserExecutable := strings.TrimSpace(os.Getenv(testBrowserEnvironmentVariableConstant))
	if len(configuredBrowserExecutable) > 0 {
		if browserPathInfo, browserPathError := os.Stat(configuredBrowserExecutable); browserPathError == nil && !browserPathInfo.IsDir() {
			return configuredBrowserExecutable
		}
	}

	for _, candidate := range browserExecutableCandidates {
		if strings.Contains(candidate, string(filepath.Separator)) {
			if browserPathInfo, browserPathError := os.Stat(candidate); browserPathError == nil && !browserPathInfo.IsDir() {
				return candidate
			}
			continue
		}

		resolvedPath, resolvedPathError := exec.LookPath(candidate)
		if resolvedPathError == nil {
			return resolvedPath
		}
	}

	return ""
}

func selectedRepositoryDescriptor(testingInstance *testing.T, repositoryCatalog web.RepositoryCatalog) web.RepositoryDescriptor {
	testingInstance.Helper()

	selectedRepositoryID := repositoryCatalog.SelectedRepositoryID
	for _, repository := range repositoryCatalog.Repositories {
		if repository.ID == selectedRepositoryID {
			return repository
		}
	}

	require.NotEmpty(testingInstance, repositoryCatalog.Repositories)
	return repositoryCatalog.Repositories[0]
}

func waitForControlSurfaceReady(testingInstance *testing.T, browserContext context.Context, expectedRepositoryName string) {
	testingInstance.Helper()

	require.Eventually(testingInstance, func() bool {
		repositoryTitle, repositoryTitleError := readTextContent(browserContext, "#repo-title")
		if repositoryTitleError != nil {
			return false
		}
		if repositoryTitle == repositoryTitleLoadingConstant || repositoryTitle != expectedRepositoryName {
			return false
		}

		commandPreview, commandPreviewError := readTextContent(browserContext, commandPreviewSelectorConstant)
		if commandPreviewError != nil {
			return false
		}
		return strings.TrimSpace(commandPreview) != ""
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)
}

func assertSelectedCommand(testingInstance *testing.T, browserContext context.Context, expectedCommandPath string) {
	testingInstance.Helper()

	require.Eventually(testingInstance, func() bool {
		selectedCommandPath, selectedCommandError := readTextContent(browserContext, selectedPathSelectorConstant)
		if selectedCommandError != nil {
			return false
		}
		return strings.TrimSpace(selectedCommandPath) == expectedCommandPath
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)
}

func assertRunnerArguments(testingInstance *testing.T, browserContext context.Context, expectedArguments []string) {
	testingInstance.Helper()

	lastObservedArguments := []string(nil)
	argumentsMatched := assert.Eventually(testingInstance, func() bool {
		argumentsValue, argumentsError := readValue(browserContext, argumentsInputSelectorConstant)
		if argumentsError != nil {
			return false
		}
		lastObservedArguments = splitArgumentLines(argumentsValue)
		return strings.Join(lastObservedArguments, "\n") == strings.Join(expectedArguments, "\n")
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)
	if !argumentsMatched {
		require.Equal(testingInstance, expectedArguments, lastObservedArguments)
	}

	argumentsValue, argumentsError := readValue(browserContext, argumentsInputSelectorConstant)
	require.NoError(testingInstance, argumentsError)
	require.Equal(testingInstance, expectedArguments, splitArgumentLines(argumentsValue))
}

func splitArgumentLines(argumentsValue string) []string {
	trimmedArguments := strings.TrimSpace(argumentsValue)
	if len(trimmedArguments) == 0 {
		return nil
	}

	return strings.Split(trimmedArguments, "\n")
}

func readTextContent(browserContext context.Context, selector string) (string, error) {
	var textContent string
	script := fmt.Sprintf(`(() => {
		const element = document.querySelector(%q);
		return element ? element.textContent || "" : "";
	})()`, selector)
	actionError := chromedp.Run(browserContext, chromedp.Evaluate(script, &textContent))
	return strings.TrimSpace(textContent), actionError
}

func readValue(browserContext context.Context, selector string) (string, error) {
	var controlValue string
	actionError := chromedp.Run(browserContext, chromedp.Value(selector, &controlValue, chromedp.ByQuery))
	return controlValue, actionError
}

func readDisabledState(browserContext context.Context, selector string) (bool, error) {
	var disabled bool
	script := fmt.Sprintf(`(() => {
		const element = document.querySelector(%q);
		return element ? Boolean(element.disabled) : false;
	})()`, selector)
	actionError := chromedp.Run(browserContext, chromedp.Evaluate(script, &disabled))
	return disabled, actionError
}

func setControlValue(selector string, value string) chromedp.Action {
	script := fmt.Sprintf(`(() => {
		const element = document.querySelector(%q);
		if (!element) {
			throw new Error("missing control");
		}
		element.value = %q;
		element.dispatchEvent(new Event("input", { bubbles: true }));
		element.dispatchEvent(new Event("change", { bubbles: true }));
	})()`, selector, value)

	return chromedp.Evaluate(script, nil)
}

func setCheckboxValue(selector string, checked bool) chromedp.Action {
	script := fmt.Sprintf(`(() => {
		const element = document.querySelector(%q);
		if (!element) {
			throw new Error("missing checkbox");
		}
		element.checked = %t;
		element.dispatchEvent(new Event("input", { bubbles: true }));
		element.dispatchEvent(new Event("change", { bubbles: true }));
	})()`, selector, checked)

	return chromedp.Evaluate(script, nil)
}

func clickRepositoryTreeTitle(title string) chromedp.Action {
	return chromedp.Evaluate(fmt.Sprintf(`(() => {
		const rows = Array.from(document.querySelectorAll(%q + " .wb-row"));
		const match = rows.find((row) => {
			const titleElement = row.querySelector(".wb-title");
			return titleElement && (titleElement.textContent || "").trim() === %q;
		});
		if (!match) {
			throw new Error("missing tree node title");
		}
		const titleElement = match.querySelector(".wb-title");
		titleElement.dispatchEvent(new MouseEvent("click", { bubbles: true }));
	})()`, repoTreeSelectorConstant, title), nil)
}

func doubleClickRepositoryTreeTitle(title string) chromedp.Action {
	return chromedp.Evaluate(fmt.Sprintf(`(() => {
		const rows = Array.from(document.querySelectorAll(%q + " .wb-row"));
		const match = rows.find((row) => {
			const titleElement = row.querySelector(".wb-title");
			return titleElement && (titleElement.textContent || "").trim() === %q;
		});
		if (!match) {
			throw new Error("missing tree node title");
		}
		const titleElement = match.querySelector(".wb-title");
		titleElement.dispatchEvent(new MouseEvent("dblclick", { bubbles: true }));
	})()`, repoTreeSelectorConstant, title), nil)
}

func clickRepositoryTreeCheckbox(title string) chromedp.Action {
	return chromedp.Evaluate(fmt.Sprintf(`(() => {
		const rows = Array.from(document.querySelectorAll(%q + " .wb-row"));
		const match = rows.find((row) => {
			const titleElement = row.querySelector(".wb-title");
			return titleElement && (titleElement.textContent || "").trim() === %q;
		});
		if (!match) {
			throw new Error("missing tree node title");
		}
		const checkboxElement = match.querySelector(".wb-checkbox");
		if (!checkboxElement) {
			throw new Error("missing tree checkbox");
		}
		checkboxElement.dispatchEvent(new MouseEvent("click", { bubbles: true }));
	})()`, repoTreeSelectorConstant, title), nil)
}
