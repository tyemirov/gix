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
	repositoryTitleLoadingConstant         = "Loading..."

	auditSelectionBadgeSelectorConstant      = "#audit-selection-badge"
	auditSelectionSummarySelectorConstant    = "#audit-selection-summary"
	auditRootsInputSelectorConstant          = "#audit-roots-input"
	auditIncludeAllSelectorConstant          = "#audit-include-all"
	auditRunButtonSelectorConstant           = "#task-inspect-load"
	auditResultsPanelSelectorConstant        = "#audit-results-panel"
	auditResultsSummarySelectorConstant      = "#audit-results-summary"
	auditResultsBodySelectorConstant         = "#audit-results-body"
	auditNameMatchesFilterSelectorConstant   = "[data-audit-column-filter='name_matches']"
	auditQueuePanelSelectorConstant          = "#audit-queue-panel"
	auditQueueSummarySelectorConstant        = "#audit-queue-summary"
	auditQueueListSelectorConstant           = "#audit-queue-list"
	auditQueueApplySelectorConstant          = "#audit-queue-apply"
	auditQueueDeleteSelectorConstant         = "[data-audit-action='delete_folder']"
	auditQueueDeleteConfirmSelector          = "[data-queue-confirm-delete]"
	auditQueueProtocolSelectorConstant       = "[data-audit-action='convert_protocol']"
	auditQueueRenameSelectorConstant         = "[data-audit-action='rename_folder']"
	auditQueueSyncSelectorConstant           = "[data-audit-action='sync_with_remote']"
	auditQueueTargetProtocolSelector         = "[data-queue-target-protocol]"
	auditQueueSyncStrategySelector           = "[data-queue-sync-strategy]"
	repoFilterSelectorConstant               = "#repo-filter"
	repoFocusSelectorConstant                = ".repo-focus"
	repoLaunchSummarySelectorConstant        = "#repo-launch-summary"
	repoScopeSelectorConstant                = ".repo-scope"
	repoSidebarSelectorConstant              = "#repo-sidebar"
	repoTreeSelectorConstant                 = "#repo-tree"
	queueIntroPanelSelectorConstant          = ".secondary-brand-panel"
	workspaceLayoutSelectorConstant          = "#workspace-layout"
	workspaceMainSelectorConstant            = "#workspace-main"
	branchTaskButtonSelectorConstant         = "#task-branch"
	filesTaskButtonSelectorConstant          = "#task-files"
	remotesTaskButtonSelectorConstant        = "#task-remotes"
	workflowsTaskButtonSelectorConstant      = "#task-workflows"
	advancedTaskButtonSelectorConstant       = "#task-advanced"
	fileTaskModeSelectorConstant             = "#file-task-mode"
	fileFindInputSelectorConstant            = "#file-find-input"
	fileReplaceInputSelectorConstant         = "#file-replace-input"
	fileLoadButtonSelectorConstant           = "#task-file-load"
	remoteOwnerInputSelectorConstant         = "#remote-owner-input"
	remoteLoadButtonSelectorConstant         = "#task-remote-load"
	workflowTargetInputSelectorConstant      = "#workflow-target-input"
	workflowVarsInputSelectorConstant        = "#workflow-vars-input"
	workflowVarFilesInputSelectorConstant    = "#workflow-var-files-input"
	workflowWorkersInputSelectorConstant     = "#workflow-workers-input"
	workflowRequireCleanSelectorConstant     = "#workflow-require-clean"
	workflowLoadButtonSelectorConstant       = "#task-workflow-load"
	workflowActionRepoSelectorConstant       = "#workflow-action-repo"
	workflowPrimitiveSelectSelectorConstant  = "#workflow-primitive-select"
	workflowPrimitiveSummarySelectorConstant = "#workflow-primitive-summary"
	workflowPrimitiveFieldsSelectorConstant  = "#workflow-primitive-fields"
	workflowActionQueueSelectorConstant      = "#workflow-action-queue"
	workflowQueueSummarySelectorConstant     = "#workflow-queue-summary"
	workflowQueueListSelectorConstant        = "#workflow-queue-list"
	workflowQueueApplySelectorConstant       = "#workflow-queue-apply"
	workflowPatternsSelectorConstant         = "[data-workflow-param-key='patterns']"
	workflowFindSelectorConstant             = "[data-workflow-param-key='find']"
	workflowReplaceSelectorConstant          = "[data-workflow-param-key='replace']"
	switchTargetButtonSelectorConstant       = "#action-switch-target"
	selectedPathSelectorConstant             = "#selected-path"
	commandGroupsSelectorConstant            = "#command-groups"
	argumentsInputSelectorConstant           = "#arguments-input"
	commandPreviewSelectorConstant           = "#command-preview"
	runCommandSelectorConstant               = "#run-command"
	stdoutOutputSelectorConstant             = "#stdout-output"
	stderrOutputSelectorConstant             = "#stderr-output"
	auditCommandPathConstant                 = "gix audit"
	branchCommandPathConstant                = "gix cd"
	filesReplaceCommandPathConstant          = "gix files replace"
	remoteCanonicalCommandPathConstant       = "gix remote update-to-canonical"
	workflowCommandPathConstant              = "gix workflow"
	versionCommandPathConstant               = "gix version"
	refModeNamedConstant                     = "named"
	pathModeRelativeConstant                 = "relative"
	fileTaskModeReplaceConstant              = "replace"
	repositoryReadmePathConstant             = "README.md"
	replacementFindValueConstant             = "initial"
	replacementTextValueConstant             = "updated"
	remoteOwnerValueConstant                 = "mprlab"
	workflowTargetValueConstant              = "configs/proprietary-licensing.yaml"
	workflowVariableAssignmentConstant       = "license_year=2026"
	workflowVariableFileConstant             = "./vars.yaml"
	workflowWorkersValueConstant             = "3"
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

func TestWebInterfaceBrowserShowsWorkflowQueueByDefault(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))

	httpServer, repositoryCatalog := newBrowserTestServer(t, repositoryPath)
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(workflowPrimitiveSelectSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	workflowRepoText, workflowRepoError := readTextContent(browserContext, workflowActionRepoSelectorConstant)
	require.NoError(t, workflowRepoError)
	assertTextContainsPath(t, workflowRepoText, repositoryPath)

	workflowPrimitiveValue, workflowPrimitiveValueError := readValue(browserContext, workflowPrimitiveSelectSelectorConstant)
	require.NoError(t, workflowPrimitiveValueError)
	require.NotEmpty(t, workflowPrimitiveValue)

	queueDisabled, queueDisabledError := readDisabledState(browserContext, workflowActionQueueSelectorConstant)
	require.NoError(t, queueDisabledError)
	require.False(t, queueDisabled)

	runCommandHidden, runCommandHiddenError := readHiddenState(browserContext, runCommandSelectorConstant)
	require.NoError(t, runCommandHiddenError)
	require.True(t, runCommandHidden)
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
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	auditSelectionBadge, auditSelectionBadgeError := readTextContent(browserContext, auditSelectionBadgeSelectorConstant)
	require.NoError(t, auditSelectionBadgeError)
	require.Equal(t, "Selected folder", auditSelectionBadge)

	auditSelectionSummary, auditSelectionSummaryError := readTextContent(browserContext, auditSelectionSummarySelectorConstant)
	require.NoError(t, auditSelectionSummaryError)
	require.Contains(t, auditSelectionSummary, expectedRepository.Path)

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
	require.Contains(t, auditResultsText, expectedRepository.Path)

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

func TestWebInterfaceBrowserInspectsAuditRootsFromExplicitLaunchRoots(t *testing.T) {
	rootPath := t.TempDir()
	launchRootPath := filepath.Join(rootPath, "fleet", "workspace")
	repositoryPath := createTestRepository(t, filepath.Join(launchRootPath, "example"))

	httpServer, repositoryCatalog := newBrowserTestServerWithOptions(
		t,
		rootPath,
		[]string{launchRootPath},
		nil,
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
		},
		nil,
	)
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	auditSelectionSummary, auditSelectionSummaryError := readTextContent(browserContext, auditSelectionSummarySelectorConstant)
	require.NoError(t, auditSelectionSummaryError)
	assertTextContainsPath(t, auditSelectionSummary, launchRootPath)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditResultsPanelSelectorConstant, chromedp.ByQuery),
	))

	auditSummary, auditSummaryError := readTextContent(browserContext, auditResultsSummarySelectorConstant)
	require.NoError(t, auditSummaryError)
	require.Equal(t, "1 row", auditSummary)

	auditResultsText, auditResultsError := readTextContent(browserContext, auditResultsBodySelectorConstant)
	require.NoError(t, auditResultsError)
	require.Contains(t, auditResultsText, "canonical/example")
	assertTextContainsPath(t, auditResultsText, repositoryPath)

	runErrorText, runErrorError := readTextContent(browserContext, "#run-error")
	require.NoError(t, runErrorError)
	require.Empty(t, runErrorText)
}

func TestWebInterfaceBrowserInspectsAuditRootsFromMultipleExplicitLaunchRoots(t *testing.T) {
	rootPath := t.TempDir()
	firstLaunchRootPath := filepath.Join(rootPath, "fleet", "alpha")
	secondLaunchRootPath := filepath.Join(rootPath, "fleet", "beta")
	firstRepositoryPath := createTestRepository(t, filepath.Join(firstLaunchRootPath, "example"))
	createTestRepository(t, filepath.Join(secondLaunchRootPath, "other"))

	httpServer, repositoryCatalog := newBrowserTestServerWithOptions(
		t,
		rootPath,
		[]string{firstLaunchRootPath, secondLaunchRootPath},
		nil,
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
		},
		nil,
	)
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	auditSelectionSummary, auditSelectionSummaryError := readTextContent(browserContext, auditSelectionSummarySelectorConstant)
	require.NoError(t, auditSelectionSummaryError)
	assertTextContainsPath(t, auditSelectionSummary, firstLaunchRootPath)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditResultsPanelSelectorConstant, chromedp.ByQuery),
	))

	auditSummary, auditSummaryError := readTextContent(browserContext, auditResultsSummarySelectorConstant)
	require.NoError(t, auditSummaryError)
	require.Equal(t, "1 row", auditSummary)

	auditResultsText, auditResultsError := readTextContent(browserContext, auditResultsBodySelectorConstant)
	require.NoError(t, auditResultsError)
	require.Contains(t, auditResultsText, "canonical/example")
	assertTextContainsPath(t, auditResultsText, firstRepositoryPath)

	runErrorText, runErrorError := readTextContent(browserContext, "#run-error")
	require.NoError(t, runErrorError)
	require.Empty(t, runErrorText)
}

func TestWebInterfaceBrowserInspectsAuditRootsFromSelectedParentFolder(t *testing.T) {
	rootPath := t.TempDir()
	launchRootPath := filepath.Join(rootPath, "fleet", "workspace")
	repositoryPath := createTestRepository(t, filepath.Join(launchRootPath, "example"))

	httpServer, repositoryCatalog := newBrowserTestServerWithOptions(
		t,
		rootPath,
		[]string{launchRootPath},
		nil,
		func(_ context.Context, request web.AuditInspectionRequest) web.AuditInspectionResponse {
			return web.AuditInspectionResponse{
				Roots: request.Roots,
				Rows: []web.AuditInspectionRow{
					{
						Path:                   filepath.Join(request.Roots[0], "workspace", "example"),
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
		},
		nil,
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
		clickRepositoryTreeTitle("fleet"),
	))

	auditSelectionSummary, auditSelectionSummaryError := readTextContent(browserContext, auditSelectionSummarySelectorConstant)
	require.NoError(t, auditSelectionSummaryError)
	assertTextContainsPath(t, auditSelectionSummary, filepath.Join(rootPath, "fleet"))

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditResultsPanelSelectorConstant, chromedp.ByQuery),
	))

	auditSummary, auditSummaryError := readTextContent(browserContext, auditResultsSummarySelectorConstant)
	require.NoError(t, auditSummaryError)
	require.Equal(t, "1 row", auditSummary)

	auditResultsText, auditResultsError := readTextContent(browserContext, auditResultsBodySelectorConstant)
	require.NoError(t, auditResultsError)
	require.Contains(t, auditResultsText, "canonical/example")
	assertTextContainsPath(t, auditResultsText, repositoryPath)

	runErrorText, runErrorError := readTextContent(browserContext, "#run-error")
	require.NoError(t, runErrorError)
	require.Empty(t, runErrorText)
}

func TestWebInterfaceBrowserKeepsAuditActionButtonsLegible(t *testing.T) {
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
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditQueueRenameSelectorConstant, chromedp.ByQuery),
	))

	var presentation struct {
		Label       string  `json:"label"`
		Color       string  `json:"color"`
		BorderColor string  `json:"borderColor"`
		ButtonWidth float64 `json:"buttonWidth"`
		CellWidth   float64 `json:"cellWidth"`
	}
	require.NoError(t, chromedp.Run(browserContext, chromedp.Evaluate(fmt.Sprintf(`(() => {
		const button = document.querySelector(%q);
		if (!button) {
			throw new Error("missing audit action button");
		}
		const style = window.getComputedStyle(button);
		const cell = button.closest("td");
		return {
			label: (button.textContent || "").trim(),
			color: style.color,
			borderColor: style.borderTopColor,
			buttonWidth: button.getBoundingClientRect().width,
			cellWidth: cell ? cell.getBoundingClientRect().width : 0
		};
	})()`, auditQueueRenameSelectorConstant), &presentation)))

	require.Equal(t, "Queue rename", presentation.Label)
	require.NotEqual(t, "rgba(0, 0, 0, 0)", presentation.BorderColor)
	require.NotEqual(t, "rgba(0, 0, 0, 0)", presentation.Color)
	require.Greater(t, presentation.CellWidth, 0.0)
	require.Less(t, presentation.ButtonWidth, presentation.CellWidth)
}

func TestWebInterfaceBrowserQueuedAuditActionBecomesDequeueControl(t *testing.T) {
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
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditQueueRenameSelectorConstant, chromedp.ByQuery),
		chromedp.Click(auditQueueRenameSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditQueuePanelSelectorConstant, chromedp.ByQuery),
	))

	require.Eventually(t, func() bool {
		buttonLabel, buttonLabelError := readTextContent(browserContext, auditQueueRenameSelectorConstant)
		if buttonLabelError != nil {
			return false
		}
		queueSummary, queueSummaryError := readTextContent(browserContext, auditQueueSummarySelectorConstant)
		if queueSummaryError != nil {
			return false
		}
		return buttonLabel == "Dequeue rename" && queueSummary == "1 pending change"
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(auditQueueRenameSelectorConstant, chromedp.ByQuery),
	))

	require.Eventually(t, func() bool {
		buttonLabel, buttonLabelError := readTextContent(browserContext, auditQueueRenameSelectorConstant)
		if buttonLabelError != nil {
			return false
		}
		queueSummary, queueSummaryError := readTextContent(browserContext, auditQueueSummarySelectorConstant)
		if queueSummaryError != nil {
			return false
		}
		return buttonLabel == "Queue rename" && queueSummary == "0 pending changes"
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)
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
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	require.NoError(t, chromedp.Run(browserContext,
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

func TestWebInterfaceBrowserUsesInspectButtonWithoutCommandRunner(t *testing.T) {
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
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditResultsPanelSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditQueueRenameSelectorConstant, chromedp.ByQuery),
	))

	inspectButtonLabel, inspectButtonLabelError := readTextContent(browserContext, auditRunButtonSelectorConstant)
	require.NoError(t, inspectButtonLabelError)
	require.Equal(t, "Inspect or refresh audit table", inspectButtonLabel)

	runCommandHidden, runCommandHiddenError := readHiddenState(browserContext, runCommandSelectorConstant)
	require.NoError(t, runCommandHiddenError)
	require.True(t, runCommandHidden)
}

func TestWebInterfaceBrowserInspectionIgnoresEditedAuditRootArguments(t *testing.T) {
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
	require.Contains(t, auditResultsText, canonicalPath(t, repositoryPath))
	require.NotContains(t, auditResultsText, alternateRoot)
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
						Path:                   filepath.Join(request.Roots[0], "example"),
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

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))

	waitForAuditResultsBody(t, browserContext, []string{"yes"}, nil)
}

func TestWebInterfaceBrowserAppliesQueueUsingLastInspectedScope(t *testing.T) {
	rootPath := t.TempDir()
	workspaceRepositoryPath := createTestRepository(t, filepath.Join(rootPath, "workspace", "example"))
	alternateRepositoryPath := createTestRepository(t, filepath.Join(rootPath, "lab", "other"))
	workspacePath := canonicalPath(t, filepath.Dir(workspaceRepositoryPath))
	alternateRoot := canonicalPath(t, filepath.Dir(alternateRepositoryPath))

	renameQueued := false
	httpServer, repositoryCatalog := newBrowserTestServerWithAuditHandlers(
		t,
		rootPath,
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
						Path:                   filepath.Join(request.Roots[0], "example"),
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
		clickRepositoryTreeTitle("workspace"),
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditResultsPanelSelectorConstant, chromedp.ByQuery),
		chromedp.Click(auditQueueRenameSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditQueuePanelSelectorConstant, chromedp.ByQuery),
		clickRepositoryTreeTitle("lab"),
		chromedp.Click(auditQueueApplySelectorConstant, chromedp.ByQuery),
	))

	require.Eventually(t, func() bool {
		summaryText, summaryError := readTextContent(browserContext, auditQueueSummarySelectorConstant)
		if summaryError != nil {
			return false
		}
		return summaryText == "0 pending changes"
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)

	require.Eventually(t, func() bool {
		stdoutOutput, stdoutError := readTextContent(browserContext, stdoutOutputSelectorConstant)
		if stdoutError != nil {
			return false
		}
		return strings.Contains(stdoutOutput, workspaceRepositoryPath) && strings.Contains(stdoutOutput, "rename applied")
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))

	waitForAuditResultsBody(t, browserContext, []string{alternateRoot, "other"}, []string{workspacePath})
}

func TestWebInterfaceBrowserQueuesDeleteChangeAndRequiresConfirmation(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))
	workspacePath := canonicalPath(t, filepath.Dir(repositoryPath))

	deleteApplied := false
	httpServer, repositoryCatalog := newBrowserTestServerWithAuditHandlers(
		t,
		repositoryPath,
		func(_ context.Context, request web.AuditInspectionRequest) web.AuditInspectionResponse {
			rows := []web.AuditInspectionRow{
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
	require.Contains(t, queueText, workspacePath)

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

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))

	waitForAuditResultsSummary(t, browserContext, "0 rows")
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
						Path:                   filepath.Join(request.Roots[0], "example"),
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

	var protocolOptions []string
	require.NoError(t, chromedp.Run(browserContext, chromedp.Evaluate(`(() => {
		const select = document.querySelector("[data-queue-target-protocol]");
		if (!select) {
			throw new Error("missing protocol target select");
		}
		return Array.from(select.options).map((option) => option.value);
	})()`, &protocolOptions)))
	require.Equal(t, []string{"ssh", "https"}, protocolOptions)

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

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))

	waitForAuditResultsBody(t, browserContext, []string{"ssh", "yes"}, nil)
}

func TestWebInterfaceBrowserAuditScopeChangesDoNotChangeWorkflowActionTarget(t *testing.T) {
	rootPath := t.TempDir()
	firstRepositoryPath := createTestRepository(t, filepath.Join(rootPath, "alpha"))
	createTestRepository(t, filepath.Join(rootPath, "nested", "beta"))

	httpServer, repositoryCatalog := newBrowserTestServer(t, rootPath)
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(workflowPrimitiveSelectSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	require.NoError(t, chromedp.Run(browserContext,
		clickRepositoryTreeTitle("alpha"),
	))

	require.Eventually(t, func() bool {
		repositoryTitle, repositoryTitleError := readTextContent(browserContext, "#repo-title")
		if repositoryTitleError != nil {
			return false
		}
		return repositoryTitle == "alpha"
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)

	require.NoError(t, chromedp.Run(browserContext,
		clickRepositoryTreeExpander("nested"),
	))
	waitForRepositoryTreeState(t, browserContext, []string{"alpha", "nested", "beta"}, nil, "")

	require.NoError(t, chromedp.Run(browserContext,
		clickRepositoryTreeCheckbox("beta"),
	))

	waitForAuditRoots(t, browserContext, []string{
		firstRepositoryPath,
		filepath.Join(rootPath, "nested", "beta"),
	})

	workflowRepoText, workflowRepoError := readTextContent(browserContext, workflowActionRepoSelectorConstant)
	require.NoError(t, workflowRepoError)
	assertTextContainsPath(t, workflowRepoText, firstRepositoryPath)
}

func TestWebInterfaceBrowserQueuesAndAppliesWorkflowPrimitiveActions(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))

	httpServer, repositoryCatalog := newBrowserTestServer(t, repositoryPath)
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(workflowPrimitiveSelectSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	require.NoError(t, chromedp.Run(browserContext,
		setControlValue(workflowPrimitiveSelectSelectorConstant, webWorkflowPrimitiveFileReplaceConstant),
		chromedp.WaitVisible(workflowPrimitiveFieldsSelectorConstant, chromedp.ByQuery),
		setControlValue(workflowPatternsSelectorConstant, "README.md"),
		setControlValue(workflowFindSelectorConstant, "initial"),
		setControlValue(workflowReplaceSelectorConstant, "updated"),
		chromedp.Click(workflowActionQueueSelectorConstant, chromedp.ByQuery),
	))

	queuedRepositoryText, queuedRepositoryError := readTextContent(browserContext, workflowActionRepoSelectorConstant)
	require.NoError(t, queuedRepositoryError)
	assertTextContainsPath(t, queuedRepositoryText, repositoryPath)

	queuedSummary, queuedSummaryError := readTextContent(browserContext, workflowQueueSummarySelectorConstant)
	require.NoError(t, queuedSummaryError)
	require.Equal(t, "1 queued action", queuedSummary)

	queuedListText, queuedListError := readTextContent(browserContext, workflowQueueListSelectorConstant)
	require.NoError(t, queuedListError)
	require.Contains(t, queuedListText, "Replace in files")
	require.Contains(t, queuedListText, "README.md")

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(workflowQueueApplySelectorConstant, chromedp.ByQuery),
	))

	require.Eventually(t, func() bool {
		updatedContent, readError := os.ReadFile(filepath.Join(repositoryPath, "README.md"))
		if readError != nil {
			return false
		}
		return string(updatedContent) == "updated\n"
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)

	stdoutOutput, stdoutError := readTextContent(browserContext, stdoutOutputSelectorConstant)
	require.NoError(t, stdoutError)
	require.Contains(t, stdoutOutput, "Replace in files")
	require.Contains(t, stdoutOutput, "REPLACE-APPLY")

	stderrOutput, stderrError := readTextContent(browserContext, stderrOutputSelectorConstant)
	require.NoError(t, stderrError)
	require.Empty(t, strings.TrimSpace(stderrOutput))
}

func TestWebInterfaceBrowserAuditTableListsWorkflowPrimitiveRowActions(t *testing.T) {
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
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(auditRunButtonSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditResultsPanelSelectorConstant, chromedp.ByQuery),
	))

	auditResultsText, auditResultsError := readTextContent(browserContext, auditResultsBodySelectorConstant)
	require.NoError(t, auditResultsError)
	require.Contains(t, auditResultsText, "Promote default branch")
	require.Contains(t, auditResultsText, "Create release tag")
	require.Contains(t, auditResultsText, "Retag releases")
	require.Contains(t, auditResultsText, "Audit report")
	require.Contains(t, auditResultsText, "Purge history")
	require.Contains(t, auditResultsText, "Replace in files")
	require.Contains(t, auditResultsText, "Rewrite namespace")
}

func TestWebInterfaceBrowserQueuesWorkflowPrimitiveFromAuditTableUsingCurrentDraft(t *testing.T) {
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
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(workflowPrimitiveSelectSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	require.NoError(t, chromedp.Run(browserContext,
		setControlValue(workflowPrimitiveSelectSelectorConstant, webWorkflowPrimitiveFileReplaceConstant),
		chromedp.WaitVisible(workflowPrimitiveFieldsSelectorConstant, chromedp.ByQuery),
		setControlValue(workflowPatternsSelectorConstant, "README.md"),
		setControlValue(workflowFindSelectorConstant, "initial"),
		setControlValue(workflowReplaceSelectorConstant, "updated"),
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditResultsPanelSelectorConstant, chromedp.ByQuery),
		clickAuditActionButton("Queue Replace in files"),
	))

	workflowQueueSummary, workflowQueueSummaryError := readTextContent(browserContext, workflowQueueSummarySelectorConstant)
	require.NoError(t, workflowQueueSummaryError)
	require.Equal(t, "1 queued action", workflowQueueSummary)

	workflowQueueText, workflowQueueTextError := readTextContent(browserContext, workflowQueueListSelectorConstant)
	require.NoError(t, workflowQueueTextError)
	require.Contains(t, workflowQueueText, "Replace in files")
	require.Contains(t, workflowQueueText, "README.md")
	assertTextContainsPath(t, workflowQueueText, repositoryPath)
}

func TestWebInterfaceBrowserRendersRepositoryTreeAndPreservesCheckedScopeAcrossFilter(t *testing.T) {
	rootPath := t.TempDir()
	createTestRepository(t, filepath.Join(rootPath, "alpha"))
	createTestRepository(t, filepath.Join(rootPath, "nested", "beta"))

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
		clickRepositoryTreeTitle("nested"),
	))
	waitForRepositoryTreeState(t, browserContext, []string{"alpha", "nested", "beta"}, nil, "nested")

	require.NoError(t, chromedp.Run(browserContext,
		clickRepositoryTreeCheckbox("alpha"),
		clickRepositoryTreeCheckbox("beta"),
		clickRepositoryTreeTitle("beta"),
		setControlValue(repoFilterSelectorConstant, "alpha"),
	))

	require.Eventually(t, func() bool {
		repositoryTitle, repositoryTitleError := readTextContent(browserContext, "#repo-title")
		if repositoryTitleError != nil {
			return false
		}
		return repositoryTitle == "beta"
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)

	visibleTreeTitles, visibleTreeTitlesError := readVisibleRepositoryTreeTitles(browserContext)
	require.NoError(t, visibleTreeTitlesError)
	require.Contains(t, visibleTreeTitles, "alpha")

	require.NoError(t, chromedp.Run(browserContext,
		setControlValue(repoFilterSelectorConstant, ""),
	))

	waitForAuditRoots(t, browserContext, []string{
		filepath.Join(rootPath, "alpha"),
		filepath.Join(rootPath, "nested", "beta"),
	})
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

func TestWebInterfaceBrowserCurrentRepoModeStartsAtRepositoryRoot(t *testing.T) {
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
	require.Contains(t, treeText, repositoryName)
	require.NotContains(t, treeText, parentFolderName)
}

func TestWebInterfaceBrowserCurrentRepoTreeDoesNotRevealSiblingRepositories(t *testing.T) {
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

	treeText, treeTextError := readTextContent(browserContext, repoTreeSelectorConstant)
	require.NoError(t, treeTextError)
	require.NotContains(t, treeText, "other")
}

func TestWebInterfaceBrowserCurrentRepoTreeStartsAtCurrentRepositoryRoot(t *testing.T) {
	rootPath := t.TempDir()
	repositoryPath := createTestRepository(t, filepath.Join(rootPath, "fleet", "workspace", "example"))
	createTestRepository(t, filepath.Join(repositoryPath, "plugins", "other"))

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
	require.Contains(t, initialTreeText, "plugins")
	require.NotContains(t, initialTreeText, "workspace")
	require.NotContains(t, initialTreeText, "fleet")

	require.NoError(t, chromedp.Run(browserContext,
		clickRepositoryTreeTitle("plugins"),
	))

	require.Eventually(t, func() bool {
		treeText, treeTextError := readTextContent(browserContext, repoTreeSelectorConstant)
		if treeTextError != nil {
			return false
		}
		return strings.Contains(treeText, "other")
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)
}

func TestWebInterfaceBrowserTraversesFolderTreeScenarios(t *testing.T) {
	type traversalStep struct {
		clickTitle        string
		expectedContains  []string
		expectedExcludes  []string
		expectedActiveRow string
		expectedAuditRoot string
	}

	type traversalScenario struct {
		name               string
		setup              func(*testing.T) (*httptest.Server, web.RepositoryCatalog)
		initialContains    []string
		initialExcludes    []string
		initialActiveRow   string
		initialAuditRoot   string
		expectedRepository string
		steps              []traversalStep
	}

	rootNameConfiguredRoot := "hangar"
	scenarios := []traversalScenario{
		{
			name: "current repo selection traverses downward into nested repository folders",
			setup: func(testingInstance *testing.T) (*httptest.Server, web.RepositoryCatalog) {
				rootPath := testingInstance.TempDir()
				repositoryPath := createTestRepository(testingInstance, filepath.Join(rootPath, "workspace", "example"))
				createTestRepository(testingInstance, filepath.Join(repositoryPath, "plugins", "other"))
				return newBrowserTestServer(testingInstance, repositoryPath)
			},
			initialContains:    []string{"example", "plugins"},
			initialExcludes:    []string{"other", "workspace", "sandbox"},
			initialActiveRow:   "example",
			initialAuditRoot:   "example",
			expectedRepository: "example",
			steps: []traversalStep{
				{
					clickTitle:        "plugins",
					expectedContains:  []string{"other"},
					expectedExcludes:  []string{"workspace"},
					expectedActiveRow: "plugins",
					expectedAuditRoot: filepath.Join("example", "plugins"),
				},
				{
					clickTitle:        "plugins",
					expectedContains:  []string{"example", "plugins"},
					expectedExcludes:  []string{"other"},
					expectedActiveRow: "plugins",
					expectedAuditRoot: filepath.Join("example", "plugins"),
				},
			},
		},
		{
			name: "single explicit root selection traverses downward and reopens nested folders",
			setup: func(testingInstance *testing.T) (*httptest.Server, web.RepositoryCatalog) {
				rootPath := testingInstance.TempDir()
				launchRootPath := filepath.Join(rootPath, rootNameConfiguredRoot, "fleet", "workspace")
				createTestRepository(testingInstance, filepath.Join(launchRootPath, "example"))
				createTestRepository(testingInstance, filepath.Join(launchRootPath, "nested", "other"))
				return newBrowserTestServerWithLaunchRoots(testingInstance, rootPath, []string{launchRootPath})
			},
			initialContains:    []string{"fleet", "workspace", "example", "nested"},
			initialExcludes:    []string{rootNameConfiguredRoot, "other"},
			initialActiveRow:   "workspace",
			initialAuditRoot:   filepath.Join(rootNameConfiguredRoot, "fleet", "workspace"),
			expectedRepository: "",
			steps: []traversalStep{
				{
					clickTitle:        "nested",
					expectedContains:  []string{"fleet", "workspace", "nested", "other", "example"},
					expectedExcludes:  []string{rootNameConfiguredRoot},
					expectedActiveRow: "nested",
					expectedAuditRoot: filepath.Join(rootNameConfiguredRoot, "fleet", "workspace", "nested"),
				},
				{
					clickTitle:        "nested",
					expectedContains:  []string{"fleet", "workspace", "nested", "example"},
					expectedExcludes:  []string{"other"},
					expectedActiveRow: "nested",
					expectedAuditRoot: filepath.Join(rootNameConfiguredRoot, "fleet", "workspace", "nested"),
				},
				{
					clickTitle:        "example",
					expectedContains:  []string{"fleet", "workspace", "nested", "example"},
					expectedExcludes:  []string{"other"},
					expectedActiveRow: "example",
					expectedAuditRoot: filepath.Join(rootNameConfiguredRoot, "fleet", "workspace", "example"),
				},
			},
		},
		{
			name: "single explicit root selection browses upward across repository-bearing branches only",
			setup: func(testingInstance *testing.T) (*httptest.Server, web.RepositoryCatalog) {
				rootPath := testingInstance.TempDir()
				profilePath := filepath.Join(rootPath, "profile")
				developmentPath := filepath.Join(profilePath, "Development")
				launchRootPath := filepath.Join(developmentPath, "tyemirov")
				createTestRepository(testingInstance, filepath.Join(launchRootPath, "gix"))
				createTestRepository(testingInstance, filepath.Join(profilePath, "Documents", "notes"))
				require.NoError(testingInstance, os.MkdirAll(filepath.Join(developmentPath, "shared"), 0o755))
				return newBrowserTestServerWithLaunchRoots(testingInstance, rootPath, []string{launchRootPath})
			},
			initialContains:    []string{"Development", "tyemirov", "gix"},
			initialExcludes:    []string{"shared", "Documents", "notes"},
			initialActiveRow:   "tyemirov",
			initialAuditRoot:   filepath.Join("profile", "Development", "tyemirov"),
			expectedRepository: "",
			steps: []traversalStep{
				{
					clickTitle:        "Development",
					expectedContains:  []string{"profile", "Development", "Documents", "tyemirov", "gix"},
					expectedExcludes:  []string{"shared"},
					expectedActiveRow: "Development",
					expectedAuditRoot: filepath.Join("profile", "Development"),
				},
				{
					clickTitle:        "Documents",
					expectedContains:  []string{"profile", "Development", "Documents", "notes"},
					expectedExcludes:  []string{"shared"},
					expectedActiveRow: "Documents",
					expectedAuditRoot: filepath.Join("profile", "Documents"),
				},
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			httpServer, repositoryCatalog := scenario.setup(t)
			defer httpServer.Close()

			browserContext := newBrowserTestContext(t)
			require.NoError(t, chromedp.Run(browserContext,
				chromedp.Navigate(httpServer.URL),
				chromedp.WaitVisible(repoTreeSelectorConstant, chromedp.ByQuery),
			))
			waitForControlSurfaceReady(t, browserContext, scenario.expectedRepository)
			waitForRepositoryTreeState(t, browserContext, scenario.initialContains, scenario.initialExcludes, scenario.initialActiveRow)
			if scenario.initialAuditRoot != "" {
				waitForAuditRootSuffix(t, browserContext, scenario.initialAuditRoot)
			}

			for _, step := range scenario.steps {
				require.NoError(t, chromedp.Run(browserContext, clickRepositoryTreeTitle(step.clickTitle)))
				waitForRepositoryTreeState(t, browserContext, step.expectedContains, step.expectedExcludes, step.expectedActiveRow)
				if step.expectedAuditRoot != "" {
					waitForAuditRootSuffix(t, browserContext, step.expectedAuditRoot)
				}
			}

			require.NotEmpty(t, repositoryCatalog.Repositories)
		})
	}
}

func TestWebInterfaceBrowserDiscoversRepositoryOutsideInitialLaunchCatalog(t *testing.T) {
	rootPath := t.TempDir()
	profilePath := filepath.Join(rootPath, "profile")
	launchRootPath := filepath.Join(profilePath, "Development", "tyemirov")
	createTestRepository(t, filepath.Join(launchRootPath, "gix"))
	discoveredRepositoryPath := createTestRepository(t, filepath.Join(profilePath, "Documents", "notes"))
	createTestBranch(t, discoveredRepositoryPath, "feature/dynamic")
	require.NoError(t, os.MkdirAll(filepath.Join(profilePath, "Development", "shared"), 0o755))

	httpServer, repositoryCatalog := newBrowserTestServerWithLaunchRoots(t, rootPath, []string{launchRootPath})
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(repoTreeSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, "")
	waitForRepositoryTreeState(t, browserContext, []string{"Development", "tyemirov", "gix"}, []string{"shared", "Documents", "notes"}, "tyemirov")

	require.NoError(t, chromedp.Run(browserContext,
		clickRepositoryTreeTitle("Development"),
	))
	waitForRepositoryTreeState(t, browserContext, []string{"profile", "Development", "Documents", "tyemirov", "gix"}, []string{"shared"}, "Development")

	require.NoError(t, chromedp.Run(browserContext,
		clickRepositoryTreeTitle("Documents"),
	))
	waitForRepositoryTreeState(t, browserContext, []string{"profile", "Development", "Documents", "notes"}, []string{"shared"}, "Documents")

	require.NoError(t, chromedp.Run(browserContext,
		clickRepositoryTreeTitle("notes"),
	))

	require.Eventually(t, func() bool {
		repositoryTitle, repositoryTitleError := readTextContent(browserContext, "#repo-title")
		if repositoryTitleError != nil {
			return false
		}
		return repositoryTitle == "notes"
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)

	workflowRepoText, workflowRepoError := readTextContent(browserContext, workflowActionRepoSelectorConstant)
	require.NoError(t, workflowRepoError)
	assertTextContainsPath(t, workflowRepoText, discoveredRepositoryPath)

	require.NotEmpty(t, repositoryCatalog.Repositories)
}

func TestWebInterfaceBrowserRepositoryTreeShowsNestedRepositoriesAsFolderNodes(t *testing.T) {
	rootPath := t.TempDir()
	topLevelRepositoryPath := createTestRepository(t, filepath.Join(rootPath, "alpha"))
	createTestRepository(t, filepath.Join(topLevelRepositoryPath, "plugins", "child"))
	createTestRepository(t, filepath.Join(rootPath, "gamma"))

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

	repoCountText, repoCountError := readTextContent(browserContext, "#repo-count")
	require.NoError(t, repoCountError)
	require.Equal(t, "3", repoCountText)

	require.NoError(t, chromedp.Run(browserContext,
		clickRepositoryTreeTitle("alpha"),
		clickRepositoryTreeTitle("plugins"),
	))

	waitForRepositoryTreeState(t, browserContext, []string{"alpha", "plugins", "child", "gamma"}, nil, "plugins")
}

func TestWebInterfaceBrowserRepositoryTreeOrdersSiblingsAlphabeticallyWithSharedIndent(t *testing.T) {
	rootPath := t.TempDir()
	launchRootPath := filepath.Join(rootPath, "fleet", "workspace")
	createTestRepository(t, filepath.Join(launchRootPath, "aardvark"))
	createTestRepository(t, filepath.Join(launchRootPath, "zeta", "project"))

	httpServer, _ := newBrowserTestServerWithLaunchRoots(t, rootPath, []string{launchRootPath})
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(repoTreeSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, "")

	require.Eventually(t, func() bool {
		treeText, treeTextError := readTextContent(browserContext, repoTreeSelectorConstant)
		if treeTextError != nil {
			return false
		}
		return strings.Contains(treeText, "aardvark") && strings.Contains(treeText, "zeta")
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)

	var siblingMetrics struct {
		Titles []string  `json:"titles"`
		Lefts  []float64 `json:"lefts"`
	}
	require.NoError(t, chromedp.Run(browserContext, chromedp.Evaluate(fmt.Sprintf(`(() => {
		const rows = Array.from(document.querySelectorAll(%q + " .wb-row"));
		const titles = rows
			.map((row) => row.querySelector(".wb-title"))
			.filter((title) => title && ["aardvark", "zeta"].includes((title.textContent || "").trim()));
		return {
			titles: titles.map((title) => (title.textContent || "").trim()),
			lefts: titles.map((title) => title.getBoundingClientRect().left)
		};
	})()`, repoTreeSelectorConstant), &siblingMetrics)))

	require.Equal(t, []string{"aardvark", "zeta"}, siblingMetrics.Titles)
	require.Len(t, siblingMetrics.Lefts, 2)
	require.InDelta(t, siblingMetrics.Lefts[0], siblingMetrics.Lefts[1], 1.0)
}

func TestWebInterfaceBrowserRepositoryTreeHonorsLaunchRoots(t *testing.T) {
	rootPath := t.TempDir()
	firstRootPath := filepath.Join(rootPath, "fleet", "alpha")
	secondRootPath := filepath.Join(rootPath, "fleet", "beta")
	createTestRepository(t, filepath.Join(firstRootPath, "example"))
	createTestRepository(t, filepath.Join(secondRootPath, "other"))
	createTestRepository(t, filepath.Join(rootPath, "ignored", "skip"))

	httpServer, repositoryCatalog := newBrowserTestServerWithLaunchRoots(t, rootPath, []string{firstRootPath, secondRootPath})
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(repoTreeSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	repoLaunchSummary, repoLaunchSummaryError := readTextContent(browserContext, repoLaunchSummarySelectorConstant)
	require.NoError(t, repoLaunchSummaryError)
	require.Contains(t, repoLaunchSummary, "Explicit roots mode")
	require.True(
		t,
		strings.Contains(repoLaunchSummary, filepath.Join(rootPath, "fleet")) ||
			strings.Contains(repoLaunchSummary, canonicalPath(t, filepath.Join(rootPath, "fleet"))),
	)

	repoCountText, repoCountError := readTextContent(browserContext, "#repo-count")
	require.NoError(t, repoCountError)
	require.Equal(t, "2", repoCountText)

	auditSelectionBadge, auditSelectionBadgeError := readTextContent(browserContext, auditSelectionBadgeSelectorConstant)
	require.NoError(t, auditSelectionBadgeError)
	require.Equal(t, "Selected folder", auditSelectionBadge)

	auditSelectionSummary, auditSelectionSummaryError := readTextContent(browserContext, auditSelectionSummarySelectorConstant)
	require.NoError(t, auditSelectionSummaryError)
	assertTextContainsPath(t, auditSelectionSummary, firstRootPath)
	require.False(
		t,
		strings.Contains(auditSelectionSummary, secondRootPath) ||
			strings.Contains(auditSelectionSummary, canonicalPath(t, secondRootPath)),
	)

	auditRootsValue, auditRootsError := readValue(browserContext, auditRootsInputSelectorConstant)
	require.NoError(t, auditRootsError)
	assertCanonicalPathEqual(t, firstRootPath, auditRootsValue)

	treeText, treeTextError := readTextContent(browserContext, repoTreeSelectorConstant)
	require.NoError(t, treeTextError)
	require.Contains(t, treeText, "fleet")
	require.Contains(t, treeText, filepath.Base(firstRootPath))
	require.Contains(t, treeText, filepath.Base(secondRootPath))
	require.NotContains(t, treeText, "skip")

}

func TestWebInterfaceBrowserSingleLaunchRootShowsConfiguredRootUnderParentFolder(t *testing.T) {
	rootPath := t.TempDir()
	launchRootPath := filepath.Join(rootPath, "fleet")
	createTestRepository(t, filepath.Join(launchRootPath, "workspace", "example"))

	httpServer, repositoryCatalog := newBrowserTestServerWithLaunchRoots(t, rootPath, []string{launchRootPath})
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
	require.Contains(t, treeText, filepath.Base(rootPath))
	require.Contains(t, treeText, filepath.Base(launchRootPath))
	require.Contains(t, treeText, "workspace")

	require.Eventually(t, func() bool {
		activeTitle, activeTitleError := readActiveRepositoryTreeTitle(browserContext)
		if activeTitleError != nil {
			return false
		}
		return activeTitle == filepath.Base(launchRootPath)
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)

	auditSelectionBadge, auditSelectionBadgeError := readTextContent(browserContext, auditSelectionBadgeSelectorConstant)
	require.NoError(t, auditSelectionBadgeError)
	require.Equal(t, "Selected folder", auditSelectionBadge)

	auditSelectionSummary, auditSelectionSummaryError := readTextContent(browserContext, auditSelectionSummarySelectorConstant)
	require.NoError(t, auditSelectionSummaryError)
	require.True(
		t,
		strings.Contains(auditSelectionSummary, launchRootPath) ||
			strings.Contains(auditSelectionSummary, canonicalPath(t, launchRootPath)),
	)

	auditRootsValue, auditRootsError := readValue(browserContext, auditRootsInputSelectorConstant)
	require.NoError(t, auditRootsError)
	assertCanonicalPathEqual(t, launchRootPath, auditRootsValue)

	require.NoError(t, chromedp.Run(browserContext,
		clickRepositoryTreeTitle("workspace"),
	))

	waitForRepositoryTreeState(t, browserContext, []string{filepath.Base(rootPath), filepath.Base(launchRootPath), "workspace", "example"}, nil, "workspace")

	auditRootsValue, auditRootsError = readValue(browserContext, auditRootsInputSelectorConstant)
	require.NoError(t, auditRootsError)
	assertCanonicalPathEqual(t, filepath.Join(launchRootPath, "workspace"), auditRootsValue)
}

func TestWebInterfaceBrowserFolderClickSetsAuditRoot(t *testing.T) {
	rootPath := t.TempDir()
	firstFolderPath := filepath.Join(rootPath, "scratch")
	createTestRepository(t, filepath.Join(firstFolderPath, "aardvark"))
	createTestRepository(t, filepath.Join(rootPath, "lab", "beta"))

	httpServer, repositoryCatalog := newBrowserTestServer(t, rootPath)
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(repoTreeSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	require.NoError(t, chromedp.Run(browserContext,
		clickRepositoryTreeTitle("scratch"),
	))

	auditRootsValue, auditRootsError := readValue(browserContext, auditRootsInputSelectorConstant)
	require.NoError(t, auditRootsError)
	assertCanonicalPathEqual(t, firstFolderPath, auditRootsValue)

	assertRunnerArguments(t, browserContext, []string{
		"audit",
		"--roots",
		canonicalPath(t, firstFolderPath),
	})
}

func TestWebInterfaceBrowserLatestFolderClickReplacesAuditRoot(t *testing.T) {
	rootPath := t.TempDir()
	firstFolderPath := filepath.Join(rootPath, "scratch")
	secondFolderPath := filepath.Join(rootPath, "lab")
	createTestRepository(t, filepath.Join(firstFolderPath, "aardvark"))
	createTestRepository(t, filepath.Join(secondFolderPath, "beta"))

	httpServer, repositoryCatalog := newBrowserTestServer(t, rootPath)
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(repoTreeSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	require.NoError(t, chromedp.Run(browserContext,
		clickRepositoryTreeTitle("scratch"),
		clickRepositoryTreeTitle("lab"),
	))

	auditRootsValue, auditRootsError := readValue(browserContext, auditRootsInputSelectorConstant)
	require.NoError(t, auditRootsError)
	assertCanonicalPathEqual(t, secondFolderPath, auditRootsValue)

	assertRunnerArguments(t, browserContext, []string{
		"audit",
		"--roots",
		canonicalPath(t, secondFolderPath),
	})
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

func TestWebInterfaceBrowserHidesCommandOrientedControls(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))

	httpServer, repositoryCatalog := newBrowserTestServer(t, repositoryPath)
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	expectedRepository := selectedRepositoryDescriptor(t, repositoryCatalog)

	require.NoError(t, chromedp.Run(browserContext,
		chromedp.Navigate(httpServer.URL),
		chromedp.WaitVisible(workflowPrimitiveSelectSelectorConstant, chromedp.ByQuery),
	))
	waitForControlSurfaceReady(t, browserContext, expectedRepository.Name)

	hiddenSelectors := []string{
		commandGroupsSelectorConstant,
		runCommandSelectorConstant,
		repoFocusSelectorConstant,
		repoScopeSelectorConstant,
		repoLaunchSummarySelectorConstant,
		queueIntroPanelSelectorConstant,
	}
	for _, selector := range hiddenSelectors {
		hiddenState, hiddenStateError := readHiddenState(browserContext, selector)
		require.NoError(t, hiddenStateError)
		require.True(t, hiddenState, selector)
	}

	missingSelectors := []string{
		"#target-ref-mode",
		"#target-ref-select",
		"#target-path-mode",
		"#target-path-value",
	}
	for _, selector := range missingSelectors {
		selectorExists, selectorExistsError := readSelectorExists(browserContext, selector)
		require.NoError(t, selectorExistsError)
		require.False(t, selectorExists, selector)
	}

	workflowSummaryText, workflowSummaryError := readTextContent(browserContext, workflowPrimitiveSummarySelectorConstant)
	require.NoError(t, workflowSummaryError)
	require.NotEmpty(t, workflowSummaryText)
}

func newBrowserTestServer(testingInstance *testing.T, workingDirectory string) (*httptest.Server, web.RepositoryCatalog) {
	return newBrowserTestServerWithOptions(testingInstance, workingDirectory, nil, nil, nil, nil)
}

func newBrowserTestServerWithLaunchRoots(
	testingInstance *testing.T,
	workingDirectory string,
	launchRoots []string,
) (*httptest.Server, web.RepositoryCatalog) {
	return newBrowserTestServerWithOptions(testingInstance, workingDirectory, launchRoots, nil, nil, nil)
}

func newBrowserTestServerWithInspector(testingInstance *testing.T, workingDirectory string, inspectAudit web.AuditInspector) (*httptest.Server, web.RepositoryCatalog) {
	return newBrowserTestServerWithOptions(testingInstance, workingDirectory, nil, nil, inspectAudit, nil)
}

func newBrowserTestServerWithAuditHandlers(
	testingInstance *testing.T,
	workingDirectory string,
	inspectAudit web.AuditInspector,
	applyAuditChanges web.AuditChangeExecutor,
) (*httptest.Server, web.RepositoryCatalog) {
	return newBrowserTestServerWithOptions(testingInstance, workingDirectory, nil, nil, inspectAudit, applyAuditChanges)
}

func newBrowserTestServerWithOptions(
	testingInstance *testing.T,
	workingDirectory string,
	launchRoots []string,
	execute web.CommandExecutor,
	inspectAudit web.AuditInspector,
	applyAuditChanges web.AuditChangeExecutor,
) (*httptest.Server, web.RepositoryCatalog) {
	testingInstance.Helper()

	var httpServer *httptest.Server
	var repositoryCatalog web.RepositoryCatalog

	withWorkingDirectory(testingInstance, workingDirectory, func() {
		application := NewApplication()
		repositoryCatalog = application.repositoryCatalog(context.Background(), launchRoots)
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
			Address:                testServerAddressConstant,
			Repositories:           repositoryCatalog,
			Catalog:                application.commandCatalog(),
			LoadBranches:           application.loadRepositoryBranches,
			BrowseDirectories:      application.newWebDirectoryBrowser(),
			Execute:                commandExecutor,
			InspectAudit:           auditInspector,
			ApplyAuditChanges:      auditChangeExecutor,
			LoadWorkflowPrimitives: application.newWebWorkflowPrimitiveCatalogLoader(),
			ApplyWorkflowActions:   application.newWebWorkflowPrimitiveExecutor(),
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

	ready := assert.Eventually(testingInstance, func() bool {
		repositoryTitle, repositoryTitleError := readTextContent(browserContext, "#repo-title")
		if repositoryTitleError != nil {
			return false
		}
		if repositoryTitle == repositoryTitleLoadingConstant {
			return false
		}
		if strings.TrimSpace(expectedRepositoryName) != "" &&
			repositoryTitle != expectedRepositoryName &&
			repositoryTitle != "No repository selected" {
			return false
		}

		commandPreview, commandPreviewError := readTextContent(browserContext, commandPreviewSelectorConstant)
		if commandPreviewError != nil {
			return false
		}
		return strings.TrimSpace(commandPreview) != ""
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)
	if ready {
		return
	}

	repositoryTitle, _ := readTextContent(browserContext, "#repo-title")
	commandPreview, _ := readTextContent(browserContext, commandPreviewSelectorConstant)
	runError, _ := readTextContent(browserContext, "#run-error")
	testingInstance.Fatalf(
		"control surface did not become ready: repo title=%q command preview=%q run error=%q expected repository=%q",
		repositoryTitle,
		commandPreview,
		runError,
		expectedRepositoryName,
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

func waitForAuditResultsBody(testingInstance *testing.T, browserContext context.Context, expectedContains []string, expectedExcludes []string) {
	testingInstance.Helper()

	require.Eventually(testingInstance, func() bool {
		auditResultsText, auditResultsError := readTextContent(browserContext, auditResultsBodySelectorConstant)
		if auditResultsError != nil {
			return false
		}
		for _, expectedFragment := range expectedContains {
			if !strings.Contains(auditResultsText, expectedFragment) {
				return false
			}
		}
		for _, excludedFragment := range expectedExcludes {
			if strings.Contains(auditResultsText, excludedFragment) {
				return false
			}
		}
		return true
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)
}

func waitForAuditResultsSummary(testingInstance *testing.T, browserContext context.Context, expectedSummary string) {
	testingInstance.Helper()

	require.Eventually(testingInstance, func() bool {
		auditSummary, auditSummaryError := readTextContent(browserContext, auditResultsSummarySelectorConstant)
		if auditSummaryError != nil {
			return false
		}
		return auditSummary == expectedSummary
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)
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

func waitForAuditRootSuffix(testingInstance *testing.T, browserContext context.Context, expectedSuffix string) {
	testingInstance.Helper()

	normalizedSuffix := strings.ReplaceAll(filepath.Clean(expectedSuffix), "\\", "/")
	require.Eventually(testingInstance, func() bool {
		auditRootsValue, auditRootsError := readValue(browserContext, auditRootsInputSelectorConstant)
		if auditRootsError != nil {
			return false
		}

		normalizedValue := strings.ReplaceAll(strings.TrimSpace(auditRootsValue), "\\", "/")
		return strings.HasSuffix(normalizedValue, normalizedSuffix)
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

	normalizedExpectedArguments := canonicalizeArgumentPaths(testingInstance, expectedArguments)
	lastObservedArguments := []string(nil)
	lastNormalizedObservedArguments := []string(nil)
	argumentsMatched := assert.Eventually(testingInstance, func() bool {
		argumentsValue, argumentsError := readValue(browserContext, argumentsInputSelectorConstant)
		if argumentsError != nil {
			return false
		}
		lastObservedArguments = splitArgumentLines(argumentsValue)
		lastNormalizedObservedArguments = canonicalizeArgumentPaths(testingInstance, lastObservedArguments)
		return strings.Join(lastNormalizedObservedArguments, "\n") == strings.Join(normalizedExpectedArguments, "\n")
	}, browserReadyTimeoutConstant, browserReadyPollIntervalConstant)
	if !argumentsMatched {
		require.Equal(testingInstance, normalizedExpectedArguments, lastNormalizedObservedArguments)
	}

	argumentsValue, argumentsError := readValue(browserContext, argumentsInputSelectorConstant)
	require.NoError(testingInstance, argumentsError)
	require.Equal(testingInstance, normalizedExpectedArguments, canonicalizeArgumentPaths(testingInstance, splitArgumentLines(argumentsValue)))
}

func splitArgumentLines(argumentsValue string) []string {
	trimmedArguments := strings.TrimSpace(argumentsValue)
	if len(trimmedArguments) == 0 {
		return nil
	}

	return strings.Split(trimmedArguments, "\n")
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

func assertCanonicalPathEqual(testingInstance *testing.T, expectedPath string, actualPath string) {
	testingInstance.Helper()
	require.Equal(testingInstance, canonicalPath(testingInstance, expectedPath), canonicalPath(testingInstance, strings.TrimSpace(actualPath)))
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

func readDisabledState(browserContext context.Context, selector string) (bool, error) {
	var disabled bool
	script := fmt.Sprintf(`(() => {
		const element = document.querySelector(%q);
		return element ? Boolean(element.disabled) : false;
	})()`, selector)
	actionError := chromedp.Run(browserContext, chromedp.Evaluate(script, &disabled))
	return disabled, actionError
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

func clickAuditActionButton(label string) chromedp.Action {
	return chromedp.Evaluate(fmt.Sprintf(`(() => {
		const buttons = Array.from(document.querySelectorAll(%q + " button"));
		const match = buttons.find((button) => (button.textContent || "").trim() === %q);
		if (!match) {
			throw new Error("missing audit action button");
		}
		match.dispatchEvent(new MouseEvent("click", { bubbles: true }));
	})()`, auditResultsBodySelectorConstant, label), nil)
}
