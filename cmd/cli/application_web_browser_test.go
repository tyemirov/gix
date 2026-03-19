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

	cdpruntime "github.com/chromedp/cdproto/runtime"
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

	auditSelectionBadgeSelectorConstant   = "#audit-selection-badge"
	auditSelectionSummarySelectorConstant = "#audit-selection-summary"
	auditRootsInputSelectorConstant       = "#audit-roots-input"
	auditIncludeAllSelectorConstant       = "#audit-include-all"
	auditRunButtonSelectorConstant        = "#task-inspect-load"
	runErrorSelectorConstant              = "#run-error"
	runStatusSelectorConstant             = "#run-status"

	auditResultsPanelSelectorConstant      = "#audit-results-panel"
	auditResultsSummarySelectorConstant    = "#audit-results-summary"
	auditResultsBodySelectorConstant       = "#audit-results-body"
	auditNameMatchesFilterSelectorConstant = "[data-audit-column-filter='name_matches']"

	auditQueuePanelSelectorConstant        = "#audit-queue-panel"
	auditQueueSummarySelectorConstant      = "#audit-queue-summary"
	auditQueueListSelectorConstant         = "#audit-queue-list"
	auditQueueClearSelectorConstant        = "#audit-queue-clear"
	auditQueueApplySelectorConstant        = "#audit-queue-apply"
	auditQueueRenameSelectorConstant       = "[data-audit-action='rename_folder']"
	auditQueueCanonicalSelectorConstant    = "[data-audit-action='update_remote_canonical']"
	auditQueueSyncSelectorConstant         = "[data-audit-action='sync_with_remote']"
	auditQueueDeleteSelectorConstant       = "[data-audit-action='delete_folder']"
	auditQueueProtocolSelectorConstant     = "[data-audit-action='convert_protocol']"
	auditQueueIncludeOwnerSelectorConstant = "[data-queue-include-owner]"
	auditQueueRequireCleanSelectorConstant = "[data-queue-require-clean]"
	auditQueueSyncStrategySelectorConstant = "[data-queue-sync-strategy]"

	repoCountSelectorConstant   = "#repo-count"
	repoFilterSelectorConstant  = "#repo-filter"
	repoSidebarSelectorConstant = "#repo-sidebar"
	repoTreeSelectorConstant    = "#repo-tree"
	workspaceLayoutSelector     = "#workspace-layout"
	workspaceMainSelector       = "#workspace-main"

	stdoutOutputSelectorConstant = "#stdout-output"
	stderrOutputSelectorConstant = "#stderr-output"
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

func TestWebInterfaceBrowserShowsAuditWorkspaceOnly(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))

	httpServer, repositoryCatalog := newBrowserTestServer(t, repositoryPath)
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForAuditWorkspaceReady(t, browserContext, repositoryCatalog.ExplorerRoot)

	bodyText, bodyTextError := readTextContent(browserContext, "body")
	require.NoError(t, bodyTextError)
	require.Contains(t, bodyText, "Scope")
	require.Contains(t, bodyText, "Audit Workspace")
	require.Contains(t, bodyText, "Run Audit")
	require.Contains(t, bodyText, "Findings")
	require.Contains(t, bodyText, "Queued Actions")
	require.Contains(t, bodyText, "Apply Results")

	auditSelectionBadge, auditSelectionBadgeError := readTextContent(browserContext, auditSelectionBadgeSelectorConstant)
	require.NoError(t, auditSelectionBadgeError)
	require.Equal(t, "Selected folder", auditSelectionBadge)

	auditSelectionSummary, auditSelectionSummaryError := readTextContent(browserContext, auditSelectionSummarySelectorConstant)
	require.NoError(t, auditSelectionSummaryError)
	assertTextContainsPath(t, auditSelectionSummary, repositoryCatalog.ExplorerRoot)

	auditResultsHidden, auditResultsHiddenError := readHiddenState(browserContext, auditResultsPanelSelectorConstant)
	require.NoError(t, auditResultsHiddenError)
	require.True(t, auditResultsHidden)

	auditQueueHidden, auditQueueHiddenError := readHiddenState(browserContext, auditQueuePanelSelectorConstant)
	require.NoError(t, auditQueueHiddenError)
	require.True(t, auditQueueHidden)

	runStatusText, runStatusError := readTextContent(browserContext, runStatusSelectorConstant)
	require.NoError(t, runStatusError)
	require.Equal(t, "idle", runStatusText)

	missingSelectors := []string{
		"#command-groups",
		"#command-preview",
		"#run-command",
		"#selected-path",
		"#arguments-input",
		"#task-workflows",
		"#task-files",
		"#task-remotes",
		"#task-advanced",
		"#task-branch",
		"#workflow-primitive-select",
		"#workflow-action-queue",
		"#workflow-queue-list",
	}
	for _, selector := range missingSelectors {
		selectorExists, selectorExistsError := readSelectorExists(browserContext, selector)
		require.NoError(t, selectorExistsError)
		require.False(t, selectorExists, selector)
	}
}

func TestWebInterfaceBrowserInspectsAuditRootsAndDisplaysTable(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))

	httpServer, repositoryCatalog := newBrowserTestServerWithInspector(t, repositoryPath, func(_ context.Context, request web.AuditInspectionRequest) web.AuditInspectionResponse {
		return web.AuditInspectionResponse{
			Roots: request.Roots,
			Rows: []web.AuditInspectionRow{
				{
					Path:                   filepath.Join(request.Roots[0], "example"),
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
	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForAuditWorkspaceReady(t, browserContext, repositoryCatalog.ExplorerRoot)

	require.NoError(t, chromedp.Run(browserContext,
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
	assertTextContainsPath(t, auditResultsText, repositoryPath)

	runStatusText, runStatusError := readTextContent(browserContext, runStatusSelectorConstant)
	require.NoError(t, runStatusError)
	require.Equal(t, "succeeded", runStatusText)

	var auditLayout struct {
		DocumentScrollWidth float64 `json:"documentScrollWidth"`
		DocumentClientWidth float64 `json:"documentClientWidth"`
		TableScrollWidth    float64 `json:"tableScrollWidth"`
		TableClientWidth    float64 `json:"tableClientWidth"`
	}
	require.NoError(t, chromedp.Run(browserContext, chromedp.Evaluate(`(() => {
		const documentElement = document.documentElement;
		const tableShell = document.querySelector(".audit-table-shell");
		if (!tableShell) {
			throw new Error("missing audit table shell");
		}
		return {
			documentScrollWidth: documentElement.scrollWidth,
			documentClientWidth: documentElement.clientWidth,
			tableScrollWidth: tableShell.scrollWidth,
			tableClientWidth: tableShell.clientWidth
		};
	})()`, &auditLayout)))
	require.LessOrEqual(t, auditLayout.DocumentScrollWidth, auditLayout.DocumentClientWidth+2)
	require.LessOrEqual(t, auditLayout.TableScrollWidth, auditLayout.TableClientWidth+2)
}

func TestWebInterfaceBrowserFindingsOnlyRenderAuditDerivedActions(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))

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
					NameMatches:            "no",
					RemoteDefaultBranch:    "main",
					LocalBranch:            "feature/demo",
					InSync:                 "no",
					RemoteProtocol:         "https",
					OriginMatchesCanonical: "no",
					WorktreeDirty:          "no",
					DirtyFiles:             "",
				},
			},
		}
	})
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForAuditWorkspaceReady(t, browserContext, repositoryCatalog.ExplorerRoot)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditResultsPanelSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditQueueRenameSelectorConstant, chromedp.ByQuery),
	))

	auditResultsText, auditResultsError := readTextContent(browserContext, auditResultsBodySelectorConstant)
	require.NoError(t, auditResultsError)
	require.Contains(t, auditResultsText, "Queue rename folder")
	require.Contains(t, auditResultsText, "Queue fix canonical remote")
	require.Contains(t, auditResultsText, "Queue sync with remote")
	require.NotContains(t, auditResultsText, "Delete folder")
	require.NotContains(t, auditResultsText, "Fix protocol")
	require.NotContains(t, auditResultsText, "Promote default branch")
	require.NotContains(t, auditResultsText, "Create release tag")
	require.NotContains(t, auditResultsText, "Retag releases")
	require.NotContains(t, auditResultsText, "Audit report")
	require.NotContains(t, auditResultsText, "Purge history")
	require.NotContains(t, auditResultsText, "Replace in files")
	require.NotContains(t, auditResultsText, "Rewrite namespace")

	renameExists, renameExistsError := readSelectorExists(browserContext, auditQueueRenameSelectorConstant)
	require.NoError(t, renameExistsError)
	require.True(t, renameExists)

	canonicalExists, canonicalExistsError := readSelectorExists(browserContext, auditQueueCanonicalSelectorConstant)
	require.NoError(t, canonicalExistsError)
	require.True(t, canonicalExists)

	syncExists, syncExistsError := readSelectorExists(browserContext, auditQueueSyncSelectorConstant)
	require.NoError(t, syncExistsError)
	require.True(t, syncExists)

	deleteExists, deleteExistsError := readSelectorExists(browserContext, auditQueueDeleteSelectorConstant)
	require.NoError(t, deleteExistsError)
	require.False(t, deleteExists)

	protocolExists, protocolExistsError := readSelectorExists(browserContext, auditQueueProtocolSelectorConstant)
	require.NoError(t, protocolExistsError)
	require.False(t, protocolExists)
}

func TestWebInterfaceBrowserCleanRowsShowNoActions(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))

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
					RemoteProtocol:         "ssh",
					OriginMatchesCanonical: "yes",
					WorktreeDirty:          "no",
					DirtyFiles:             "",
				},
			},
		}
	})
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForAuditWorkspaceReady(t, browserContext, repositoryCatalog.ExplorerRoot)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditResultsPanelSelectorConstant, chromedp.ByQuery),
	))

	auditResultsText, auditResultsError := readTextContent(browserContext, auditResultsBodySelectorConstant)
	require.NoError(t, auditResultsError)
	require.Contains(t, auditResultsText, "No actions")
	require.NotContains(t, auditResultsText, "Queue rename folder")
	require.NotContains(t, auditResultsText, "Queue fix canonical remote")
	require.NotContains(t, auditResultsText, "Queue sync with remote")
}

func TestWebInterfaceBrowserQueueOptionsOnlyExistInQueuedItems(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))

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
					NameMatches:            "no",
					RemoteDefaultBranch:    "main",
					LocalBranch:            "feature/demo",
					InSync:                 "no",
					RemoteProtocol:         "ssh",
					OriginMatchesCanonical: "yes",
					WorktreeDirty:          "no",
					DirtyFiles:             "",
				},
			},
		}
	})
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForAuditWorkspaceReady(t, browserContext, repositoryCatalog.ExplorerRoot)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditResultsPanelSelectorConstant, chromedp.ByQuery),
	))

	includeOwnerExists, includeOwnerExistsError := readSelectorExists(browserContext, auditQueueIncludeOwnerSelectorConstant)
	require.NoError(t, includeOwnerExistsError)
	require.False(t, includeOwnerExists)

	requireCleanExists, requireCleanExistsError := readSelectorExists(browserContext, auditQueueRequireCleanSelectorConstant)
	require.NoError(t, requireCleanExistsError)
	require.False(t, requireCleanExists)

	syncStrategyExists, syncStrategyExistsError := readSelectorExists(browserContext, auditQueueSyncStrategySelectorConstant)
	require.NoError(t, syncStrategyExistsError)
	require.False(t, syncStrategyExists)

	auditResultsText, auditResultsError := readTextContent(browserContext, auditResultsBodySelectorConstant)
	require.NoError(t, auditResultsError)
	require.NotContains(t, auditResultsText, "Include the owner in the destination folder name")
	require.NotContains(t, auditResultsText, "Require a clean worktree before renaming")
	require.NotContains(t, auditResultsText, "Dirty-worktree policy")

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(auditQueueRenameSelectorConstant, chromedp.ByQuery),
		chromedp.Click(auditQueueSyncSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditQueuePanelSelectorConstant, chromedp.ByQuery),
		setCheckboxValue(auditQueueIncludeOwnerSelectorConstant, true),
		setCheckboxValue(auditQueueRequireCleanSelectorConstant, false),
		setControlValue(auditQueueSyncStrategySelectorConstant, web.AuditChangeSyncStrategyStashChanges),
	))

	queueSummary, queueSummaryError := readTextContent(browserContext, auditQueueSummarySelectorConstant)
	require.NoError(t, queueSummaryError)
	require.Equal(t, "2 queued actions", queueSummary)

	queueText, queueTextError := readTextContent(browserContext, auditQueueListSelectorConstant)
	require.NoError(t, queueTextError)
	require.Contains(t, queueText, "Rename folder")
	require.Contains(t, queueText, "Sync with remote")
	require.Contains(t, queueText, "Include the owner in the destination folder name")
	require.Contains(t, queueText, "Require a clean worktree before renaming")
	require.Contains(t, queueText, "Dirty-worktree policy")

	syncStrategyValue, syncStrategyValueError := readValue(browserContext, auditQueueSyncStrategySelectorConstant)
	require.NoError(t, syncStrategyValueError)
	require.Equal(t, web.AuditChangeSyncStrategyStashChanges, syncStrategyValue)
}

func TestWebInterfaceBrowserApplyRemovesSucceededActionsAndKeepsFailuresQueued(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))

	httpServer, repositoryCatalog := newBrowserTestServerWithAuditHandlers(
		t,
		repositoryPath,
		func(_ context.Context, request web.AuditInspectionRequest) web.AuditInspectionResponse {
			return web.AuditInspectionResponse{
				Roots: request.Roots,
				Rows: []web.AuditInspectionRow{
					{
						Path:                   filepath.Join(request.Roots[0], "example"),
						FolderName:             "example",
						IsGitRepository:        true,
						FinalGitHubRepository:  "canonical/example",
						OriginRemoteStatus:     "configured",
						NameMatches:            "no",
						RemoteDefaultBranch:    "main",
						LocalBranch:            "feature/demo",
						InSync:                 "no",
						RemoteProtocol:         "ssh",
						OriginMatchesCanonical: "yes",
						WorktreeDirty:          "no",
						DirtyFiles:             "",
					},
				},
			}
		},
		func(_ context.Context, request web.AuditChangeApplyRequest) web.AuditChangeApplyResponse {
			require.Len(t, request.Changes, 2)

			results := make([]web.AuditChangeApplyResult, 0, len(request.Changes))
			for _, change := range request.Changes {
				switch change.Kind {
				case web.AuditChangeKindSyncWithRemote:
					require.Equal(t, web.AuditChangeSyncStrategyStashChanges, change.SyncStrategy)
					results = append(results, web.AuditChangeApplyResult{
						ID:     change.ID,
						Kind:   change.Kind,
						Path:   change.Path,
						Status: "failed",
						Error:  "sync failed",
						Stderr: "remote rejected update",
					})
				case web.AuditChangeKindRenameFolder:
					results = append(results, web.AuditChangeApplyResult{
						ID:      change.ID,
						Kind:    change.Kind,
						Path:    change.Path,
						Status:  "succeeded",
						Message: "rename applied",
						Stdout:  "folder renamed",
					})
				default:
					t.Fatalf("unexpected audit change kind %q", change.Kind)
				}
			}

			return web.AuditChangeApplyResponse{Results: results}
		},
	)
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForAuditWorkspaceReady(t, browserContext, repositoryCatalog.ExplorerRoot)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditResultsPanelSelectorConstant, chromedp.ByQuery),
		chromedp.Click(auditQueueRenameSelectorConstant, chromedp.ByQuery),
		chromedp.Click(auditQueueSyncSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditQueuePanelSelectorConstant, chromedp.ByQuery),
		setControlValue(auditQueueSyncStrategySelectorConstant, web.AuditChangeSyncStrategyStashChanges),
		chromedp.Click(auditQueueApplySelectorConstant, chromedp.ByQuery),
	))

	require.Eventually(t, func() bool {
		queueSummary, queueSummaryError := readTextContent(browserContext, auditQueueSummarySelectorConstant)
		if queueSummaryError != nil {
			return false
		}
		return queueSummary == "1 queued action"
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)

	queueText, queueTextError := readTextContent(browserContext, auditQueueListSelectorConstant)
	require.NoError(t, queueTextError)
	require.Contains(t, queueText, "Sync with remote")
	require.NotContains(t, queueText, "Rename folder")

	runErrorText, runErrorError := readTextContent(browserContext, runErrorSelectorConstant)
	require.NoError(t, runErrorError)
	require.Contains(t, runErrorText, "sync failed")

	stdoutOutput, stdoutOutputError := readTextContent(browserContext, stdoutOutputSelectorConstant)
	require.NoError(t, stdoutOutputError)
	require.Contains(t, stdoutOutput, "rename applied")
	require.Contains(t, stdoutOutput, "folder renamed")

	stderrOutput, stderrOutputError := readTextContent(browserContext, stderrOutputSelectorConstant)
	require.NoError(t, stderrOutputError)
	require.Contains(t, stderrOutput, "sync failed")
	require.Contains(t, stderrOutput, "remote rejected update")

	renameButtonLabel, renameButtonLabelError := readTextContent(browserContext, auditQueueRenameSelectorConstant)
	require.NoError(t, renameButtonLabelError)
	require.Equal(t, "Queue rename folder", renameButtonLabel)

	syncButtonLabel, syncButtonLabelError := readTextContent(browserContext, auditQueueSyncSelectorConstant)
	require.NoError(t, syncButtonLabelError)
	require.Equal(t, "Dequeue sync with remote", syncButtonLabel)
}

func TestWebInterfaceBrowserReauditClearsQueuedActions(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))

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
					NameMatches:            "no",
					RemoteDefaultBranch:    "main",
					LocalBranch:            "main",
					InSync:                 "yes",
					RemoteProtocol:         "ssh",
					OriginMatchesCanonical: "yes",
					WorktreeDirty:          "no",
					DirtyFiles:             "",
				},
			},
		}
	})
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForAuditWorkspaceReady(t, browserContext, repositoryCatalog.ExplorerRoot)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditResultsPanelSelectorConstant, chromedp.ByQuery),
		chromedp.Click(auditQueueRenameSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditQueuePanelSelectorConstant, chromedp.ByQuery),
	))

	queueSummary, queueSummaryError := readTextContent(browserContext, auditQueueSummarySelectorConstant)
	require.NoError(t, queueSummaryError)
	require.Equal(t, "1 queued action", queueSummary)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))

	require.Eventually(t, func() bool {
		nextQueueSummary, nextQueueSummaryError := readTextContent(browserContext, auditQueueSummarySelectorConstant)
		if nextQueueSummaryError != nil {
			return false
		}
		return nextQueueSummary == "0 queued actions"
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)

	queueText, queueTextError := readTextContent(browserContext, auditQueueListSelectorConstant)
	require.NoError(t, queueTextError)
	require.Contains(t, queueText, "No actions queued from the latest audit snapshot.")

	runErrorText, runErrorError := readTextContent(browserContext, runErrorSelectorConstant)
	require.NoError(t, runErrorError)
	require.Contains(t, runErrorText, "Cleared 1 queued action")

	renameButtonLabel, renameButtonLabelError := readTextContent(browserContext, auditQueueRenameSelectorConstant)
	require.NoError(t, renameButtonLabelError)
	require.Equal(t, "Queue rename folder", renameButtonLabel)
}

func TestWebInterfaceBrowserSupportsMultiRootAuditsFromCheckedRepos(t *testing.T) {
	rootPath := t.TempDir()
	firstRepositoryPath := createTestRepository(t, filepath.Join(rootPath, "alpha"))
	secondRepositoryPath := createTestRepository(t, filepath.Join(rootPath, "beta"))

	httpServer, repositoryCatalog := newBrowserTestServerWithInspector(t, rootPath, func(_ context.Context, request web.AuditInspectionRequest) web.AuditInspectionResponse {
		rows := make([]web.AuditInspectionRow, 0, len(request.Roots))
		for _, root := range request.Roots {
			rows = append(rows, web.AuditInspectionRow{
				Path:                   root,
				FolderName:             filepath.Base(root),
				IsGitRepository:        true,
				FinalGitHubRepository:  "canonical/" + filepath.Base(root),
				OriginRemoteStatus:     "configured",
				NameMatches:            "yes",
				RemoteDefaultBranch:    "main",
				LocalBranch:            "main",
				InSync:                 "yes",
				RemoteProtocol:         "ssh",
				OriginMatchesCanonical: "yes",
				WorktreeDirty:          "no",
				DirtyFiles:             "",
			})
		}
		return web.AuditInspectionResponse{
			Roots: request.Roots,
			Rows:  rows,
		}
	})
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(repoTreeSelectorConstant, chromedp.ByQuery),
	))
	waitForAuditWorkspaceReady(t, browserContext, repositoryCatalog.ExplorerRoot)

	require.NoError(t, chromedp.Run(browserContext,
		clickRepositoryTreeCheckbox("alpha"),
		clickRepositoryTreeCheckbox("beta"),
	))

	waitForAuditRoots(t, browserContext, []string{firstRepositoryPath, secondRepositoryPath})

	auditSelectionBadge, auditSelectionBadgeError := readTextContent(browserContext, auditSelectionBadgeSelectorConstant)
	require.NoError(t, auditSelectionBadgeError)
	require.Equal(t, "2 checked repos", auditSelectionBadge)

	auditSelectionSummary, auditSelectionSummaryError := readTextContent(browserContext, auditSelectionSummarySelectorConstant)
	require.NoError(t, auditSelectionSummaryError)
	assertTextContainsPath(t, auditSelectionSummary, firstRepositoryPath)
	assertTextContainsPath(t, auditSelectionSummary, secondRepositoryPath)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditResultsPanelSelectorConstant, chromedp.ByQuery),
	))

	auditSummary, auditSummaryError := readTextContent(browserContext, auditResultsSummarySelectorConstant)
	require.NoError(t, auditSummaryError)
	require.Equal(t, "2 rows", auditSummary)

	auditResultsText, auditResultsError := readTextContent(browserContext, auditResultsBodySelectorConstant)
	require.NoError(t, auditResultsError)
	assertTextContainsPath(t, auditResultsText, firstRepositoryPath)
	assertTextContainsPath(t, auditResultsText, secondRepositoryPath)
}

func TestWebInterfaceBrowserFiltersAuditRowsByColumnValue(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))

	httpServer, repositoryCatalog := newBrowserTestServerWithInspector(t, repositoryPath, func(_ context.Context, request web.AuditInspectionRequest) web.AuditInspectionResponse {
		return web.AuditInspectionResponse{
			Roots: request.Roots,
			Rows: []web.AuditInspectionRow{
				{
					Path:                   filepath.Join(request.Roots[0], "alpha"),
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
					Path:                   filepath.Join(request.Roots[0], "beta"),
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
	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForAuditWorkspaceReady(t, browserContext, repositoryCatalog.ExplorerRoot)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditResultsPanelSelectorConstant, chromedp.ByQuery),
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

func TestWebInterfaceBrowserRendersRepositoryTreeAndPreservesCheckedScopeAcrossFilter(t *testing.T) {
	rootPath := t.TempDir()
	firstRepositoryPath := createTestRepository(t, filepath.Join(rootPath, "alpha"))
	secondRepositoryPath := createTestRepository(t, filepath.Join(rootPath, "nested", "beta"))

	httpServer, repositoryCatalog := newBrowserTestServer(t, rootPath)
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(repoTreeSelectorConstant, chromedp.ByQuery),
	))
	waitForAuditWorkspaceReady(t, browserContext, repositoryCatalog.ExplorerRoot)

	require.Eventually(t, func() bool {
		treeText, treeTextError := readTextContent(browserContext, repoTreeSelectorConstant)
		if treeTextError != nil {
			return false
		}
		return strings.Contains(treeText, "alpha") && strings.Contains(treeText, "nested")
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)

	require.NoError(t, chromedp.Run(browserContext,
		clickRepositoryTreeExpander("nested"),
	))
	waitForRepositoryTreeState(t, browserContext, []string{"alpha", "nested", "beta"}, nil, "")

	require.NoError(t, chromedp.Run(browserContext,
		clickRepositoryTreeCheckbox("alpha"),
		clickRepositoryTreeCheckbox("beta"),
		setControlValue(repoFilterSelectorConstant, "alpha"),
	))

	visibleTreeTitles, visibleTreeTitlesError := readVisibleRepositoryTreeTitles(browserContext)
	require.NoError(t, visibleTreeTitlesError)
	require.Contains(t, visibleTreeTitles, "alpha")

	require.NoError(t, chromedp.Run(browserContext,
		setControlValue(repoFilterSelectorConstant, ""),
	))

	waitForAuditRoots(t, browserContext, []string{firstRepositoryPath, secondRepositoryPath})
}

func TestWebInterfaceBrowserDisplaysRepositoryTreeInLeftSidebar(t *testing.T) {
	rootPath := t.TempDir()
	createTestRepository(t, filepath.Join(rootPath, "alpha"))
	createTestRepository(t, filepath.Join(rootPath, "nested", "beta"))

	httpServer, repositoryCatalog := newBrowserTestServer(t, rootPath)
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(repoSidebarSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(repoTreeSelectorConstant, chromedp.ByQuery),
	))
	waitForAuditWorkspaceReady(t, browserContext, repositoryCatalog.ExplorerRoot)

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
	})()`, workspaceLayoutSelector, repoSidebarSelectorConstant, workspaceMainSelector, repoTreeSelectorConstant), &layoutMetrics)))

	require.Greater(t, layoutMetrics.SidebarWidthRatio, 0.17)
	require.Less(t, layoutMetrics.SidebarWidthRatio, 0.23)
	require.Greater(t, layoutMetrics.TreeHeight, 180.0)
	require.Less(t, layoutMetrics.SidebarLeft, layoutMetrics.MainLeft)
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

func newBrowserTestServer(testingInstance *testing.T, workingDirectory string) (*httptest.Server, web.RepositoryCatalog) {
	return newBrowserTestServerWithOptions(testingInstance, workingDirectory, nil, nil, nil)
}

func newBrowserTestServerWithInspector(
	testingInstance *testing.T,
	workingDirectory string,
	inspectAudit web.AuditInspector,
) (*httptest.Server, web.RepositoryCatalog) {
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
	launchRoots []string,
	inspectAudit web.AuditInspector,
	applyAuditChanges web.AuditChangeExecutor,
) (*httptest.Server, web.RepositoryCatalog) {
	testingInstance.Helper()

	var httpServer *httptest.Server
	var repositoryCatalog web.RepositoryCatalog

	withWorkingDirectory(testingInstance, workingDirectory, func() {
		application := NewApplication()
		repositoryCatalog = application.repositoryCatalog(context.Background(), launchRoots)

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
			BrowseDirectories: application.newWebDirectoryBrowser(),
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

	chromedp.ListenTarget(timeoutContext, func(event interface{}) {
		switch typedEvent := event.(type) {
		case *cdpruntime.EventConsoleAPICalled:
			parts := make([]string, 0, len(typedEvent.Args))
			for _, argument := range typedEvent.Args {
				if len(strings.TrimSpace(argument.Value.String())) > 0 {
					parts = append(parts, strings.Trim(argument.Value.String(), `"`))
					continue
				}
				if len(strings.TrimSpace(argument.Description)) > 0 {
					parts = append(parts, argument.Description)
				}
			}
			testingInstance.Logf("browser console %s: %s", typedEvent.Type.String(), strings.Join(parts, " "))
		case *cdpruntime.EventExceptionThrown:
			testingInstance.Logf("browser exception: %s", typedEvent.ExceptionDetails.Error())
		}
	})

	startupError := chromedp.Run(
		timeoutContext,
		chromedp.ActionFunc(func(executionContext context.Context) error {
			return cdpruntime.Enable().Do(executionContext)
		}),
		chromedp.Navigate("about:blank"),
	)
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

func waitForAuditWorkspaceReady(testingInstance *testing.T, browserContext context.Context, expectedAuditRoot string) {
	testingInstance.Helper()

	ready := assert.Eventually(testingInstance, func() bool {
		repoCountText, repoCountError := readTextContent(browserContext, repoCountSelectorConstant)
		if repoCountError != nil || strings.TrimSpace(repoCountText) == "" {
			return false
		}

		treeText, treeTextError := readTextContent(browserContext, repoTreeSelectorConstant)
		if treeTextError != nil || strings.TrimSpace(treeText) == "" {
			return false
		}

		auditRootsValue, auditRootsError := readValue(browserContext, auditRootsInputSelectorConstant)
		if auditRootsError != nil || strings.TrimSpace(auditRootsValue) == "" {
			return false
		}

		if strings.TrimSpace(expectedAuditRoot) != "" {
			normalizedExpectedRoot := canonicalPath(testingInstance, expectedAuditRoot)
			normalizedObservedRoots := canonicalizeArgumentPaths(testingInstance, splitAuditRootsValue(auditRootsValue))
			foundExpectedRoot := false
			for _, observedRoot := range normalizedObservedRoots {
				if observedRoot == normalizedExpectedRoot {
					foundExpectedRoot = true
					break
				}
			}
			if !foundExpectedRoot {
				return false
			}
		}

		runStatusText, runStatusError := readTextContent(browserContext, runStatusSelectorConstant)
		if runStatusError != nil {
			return false
		}
		return runStatusText == "idle"
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)
	if ready {
		return
	}

	repoCountText, _ := readTextContent(browserContext, repoCountSelectorConstant)
	auditRootsValue, _ := readValue(browserContext, auditRootsInputSelectorConstant)
	runStatusText, _ := readTextContent(browserContext, runStatusSelectorConstant)
	runErrorText, _ := readTextContent(browserContext, runErrorSelectorConstant)
	testingInstance.Fatalf(
		"audit workspace did not become ready: repo count=%q audit roots=%q status=%q error=%q expected root=%q",
		repoCountText,
		auditRootsValue,
		runStatusText,
		runErrorText,
		expectedAuditRoot,
	)
}

func waitForRepositoryTreeState(testingInstance *testing.T, browserContext context.Context, expectedContains []string, expectedExcludes []string, expectedActiveRow string) {
	testingInstance.Helper()

	require.Eventually(testingInstance, func() bool {
		visibleTitles, visibleTitlesError := readVisibleRepositoryTreeTitles(browserContext)
		if visibleTitlesError != nil {
			return false
		}

		treeText := strings.Join(visibleTitles, " ")
		for _, expectedFragment := range expectedContains {
			if !strings.Contains(treeText, expectedFragment) {
				return false
			}
		}
		for _, excludedFragment := range expectedExcludes {
			if strings.Contains(treeText, excludedFragment) {
				return false
			}
		}
		if expectedActiveRow == "" {
			return true
		}

		activeTitle, activeTitleError := readActiveRepositoryTreeTitle(browserContext)
		if activeTitleError != nil {
			return false
		}
		return activeTitle == expectedActiveRow
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)
}

func waitForAuditRoots(testingInstance *testing.T, browserContext context.Context, expectedPaths []string) {
	testingInstance.Helper()

	normalizedExpectedPaths := canonicalizeArgumentPaths(testingInstance, expectedPaths)
	lastObservedPaths := []string(nil)
	rootsMatched := assert.Eventually(testingInstance, func() bool {
		auditRootsValue, auditRootsError := readValue(browserContext, auditRootsInputSelectorConstant)
		if auditRootsError != nil {
			return false
		}

		lastObservedPaths = canonicalizeArgumentPaths(testingInstance, splitAuditRootsValue(auditRootsValue))
		if len(lastObservedPaths) != len(normalizedExpectedPaths) {
			return false
		}

		for _, expectedPath := range normalizedExpectedPaths {
			foundMatch := false
			for _, observedPath := range lastObservedPaths {
				if observedPath == expectedPath {
					foundMatch = true
					break
				}
			}
			if !foundMatch {
				return false
			}
		}

		return true
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)
	if !rootsMatched {
		require.Equal(testingInstance, normalizedExpectedPaths, lastObservedPaths)
	}
}

func splitAuditRootsValue(auditRootsValue string) []string {
	return strings.FieldsFunc(strings.TrimSpace(auditRootsValue), func(r rune) bool {
		return r == ','
	})
}

func canonicalizeArgumentPaths(testingInstance *testing.T, arguments []string) []string {
	testingInstance.Helper()

	normalizedArguments := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		trimmedArgument := strings.TrimSpace(argument)
		if filepath.IsAbs(trimmedArgument) {
			normalizedArguments = append(normalizedArguments, canonicalPath(testingInstance, trimmedArgument))
			continue
		}
		normalizedArguments = append(normalizedArguments, trimmedArgument)
	}

	return normalizedArguments
}

func readVisibleRepositoryTreeTitles(browserContext context.Context) ([]string, error) {
	var titles []string
	readError := chromedp.Run(browserContext, chromedp.Evaluate(fmt.Sprintf(`(() => {
		return Array.from(document.querySelectorAll(%q + " .wb-row"))
			.filter((row) => row instanceof HTMLElement && row.offsetParent !== null && row.getClientRects().length > 0)
			.map((row) => {
				const title = row.querySelector(".wb-title");
				return title instanceof HTMLElement ? (title.textContent || "").trim() : "";
			})
			.filter(Boolean);
	})()`, repoTreeSelectorConstant), &titles))
	return titles, readError
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

func readActiveRepositoryTreeTitle(browserContext context.Context) (string, error) {
	var textContent string
	script := fmt.Sprintf(`(() => {
		const element = document.querySelector(%q + " .wb-row.wb-active .wb-title");
		return element ? element.textContent || "" : "";
	})()`, repoTreeSelectorConstant)
	actionError := chromedp.Run(browserContext, chromedp.Evaluate(script, &textContent))
	return strings.TrimSpace(textContent), actionError
}

func readValue(browserContext context.Context, selector string) (string, error) {
	var controlValue string
	actionError := chromedp.Run(browserContext, chromedp.Value(selector, &controlValue, chromedp.ByQuery))
	return controlValue, actionError
}

func assertTextContainsPath(testingInstance *testing.T, text string, expectedPath string) {
	testingInstance.Helper()

	trimmedExpectedPath := strings.TrimSpace(expectedPath)
	require.True(
		testingInstance,
		strings.Contains(text, trimmedExpectedPath) ||
			strings.Contains(text, canonicalPath(testingInstance, trimmedExpectedPath)),
	)
}

func readHiddenState(browserContext context.Context, selector string) (bool, error) {
	var hidden bool
	script := fmt.Sprintf(`(() => {
		const element = document.querySelector(%q);
		if (!(element instanceof HTMLElement)) {
			return false;
		}
		return Boolean(element.hidden || element.closest("[hidden]") || getComputedStyle(element).display === "none");
	})()`, selector)
	actionError := chromedp.Run(browserContext, chromedp.Evaluate(script, &hidden))
	return hidden, actionError
}

func readSelectorExists(browserContext context.Context, selector string) (bool, error) {
	var selectorExists bool
	script := fmt.Sprintf(`(() => Boolean(document.querySelector(%q)))()`, selector)
	actionError := chromedp.Run(browserContext, chromedp.Evaluate(script, &selectorExists))
	return selectorExists, actionError
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

func clickRepositoryTreeExpander(title string) chromedp.Action {
	return chromedp.Evaluate(fmt.Sprintf(`(() => {
		const rows = Array.from(document.querySelectorAll(%q + " .wb-row"));
		const match = rows.find((row) => {
			const titleElement = row.querySelector(".wb-title");
			return titleElement && (titleElement.textContent || "").trim() === %q;
		});
		if (!match) {
			throw new Error("missing tree node title");
		}
		const expanderElement = match.querySelector(".wb-expander");
		if (!expanderElement) {
			throw new Error("missing tree expander");
		}
		expanderElement.dispatchEvent(new MouseEvent("click", { bubbles: true }));
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
