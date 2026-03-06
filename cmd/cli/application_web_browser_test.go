package cli

import (
	"context"
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

	branchTaskButtonSelectorConstant      = "#task-branch"
	filesTaskButtonSelectorConstant       = "#task-files"
	remotesTaskButtonSelectorConstant     = "#task-remotes"
	workflowsTaskButtonSelectorConstant   = "#task-workflows"
	scopeAllButtonSelectorConstant        = "#scope-all"
	targetRefModeSelectorConstant         = "#target-ref-mode"
	targetRefSelectSelectorConstant       = "#target-ref-select"
	targetPathModeSelectorConstant        = "#target-path-mode"
	targetPathValueSelectorConstant       = "#target-path-value"
	fileTaskModeSelectorConstant          = "#file-task-mode"
	fileFindInputSelectorConstant         = "#file-find-input"
	fileReplaceInputSelectorConstant      = "#file-replace-input"
	fileLoadButtonSelectorConstant        = "#task-file-load"
	remoteOwnerInputSelectorConstant      = "#remote-owner-input"
	remoteLoadButtonSelectorConstant      = "#task-remote-load"
	workflowTargetInputSelectorConstant   = "#workflow-target-input"
	workflowVarsInputSelectorConstant     = "#workflow-vars-input"
	workflowVarFilesInputSelectorConstant = "#workflow-var-files-input"
	workflowWorkersInputSelectorConstant  = "#workflow-workers-input"
	workflowRequireCleanSelectorConstant  = "#workflow-require-clean"
	workflowLoadButtonSelectorConstant    = "#task-workflow-load"
	switchTargetButtonSelectorConstant    = "#action-switch-target"
	selectedPathSelectorConstant          = "#selected-path"
	argumentsInputSelectorConstant        = "#arguments-input"
	commandPreviewSelectorConstant        = "#command-preview"
	branchCommandPathConstant             = "gix cd"
	filesReplaceCommandPathConstant       = "gix files replace"
	remoteCanonicalCommandPathConstant    = "gix remote update-to-canonical"
	workflowCommandPathConstant           = "gix workflow"
	refModeNamedConstant                  = "named"
	pathModeRelativeConstant              = "relative"
	fileTaskModeReplaceConstant           = "replace"
	repositoryReadmePathConstant          = "README.md"
	replacementFindValueConstant          = "initial"
	replacementTextValueConstant          = "updated"
	remoteOwnerValueConstant              = "mprlab"
	workflowTargetValueConstant           = "configs/proprietary-licensing.yaml"
	workflowVariableAssignmentConstant    = "license_year=2026"
	workflowVariableFileConstant          = "./vars.yaml"
	workflowWorkersValueConstant          = "3"
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

func newBrowserTestServer(testingInstance *testing.T, workingDirectory string) (*httptest.Server, web.RepositoryCatalog) {
	testingInstance.Helper()

	var httpServer *httptest.Server
	var repositoryCatalog web.RepositoryCatalog

	withWorkingDirectory(testingInstance, workingDirectory, func() {
		application := NewApplication()
		repositoryCatalog = application.repositoryCatalog(context.Background())

		server, serverError := web.NewServer(web.ServerOptions{
			Address:      testServerAddressConstant,
			Repositories: repositoryCatalog,
			Catalog:      application.commandCatalog(),
			LoadBranches: application.loadRepositoryBranches,
			Execute:      application.newWebCommandExecutor(),
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

	return timeoutContext
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
	actionError := chromedp.Run(browserContext, chromedp.Text(selector, &textContent, chromedp.ByQuery))
	return strings.TrimSpace(textContent), actionError
}

func readValue(browserContext context.Context, selector string) (string, error) {
	var controlValue string
	actionError := chromedp.Run(browserContext, chromedp.Value(selector, &controlValue, chromedp.ByQuery))
	return controlValue, actionError
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
