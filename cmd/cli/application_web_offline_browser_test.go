package cli

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/web"
)

func TestWebInterfaceBrowserAuditsWithoutExternalAssets(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "offline-repository"))
	httpServer, repositoryCatalog := newBrowserTestServerWithInspector(t, repositoryPath, func(_ context.Context, request web.AuditInspectionRequest) web.AuditInspectionResponse {
		return web.AuditInspectionResponse{
			Roots: request.Roots,
			Rows: []web.AuditInspectionRow{{
				Path:               repositoryPath,
				FolderName:         "offline-repository",
				IsGitRepository:    true,
				OriginRemoteStatus: "missing",
			}},
		}
	})
	defer httpServer.Close()

	browserContext := newBrowserTestContext(t)
	scenarioContext, cancel := context.WithTimeout(browserContext, 8*time.Second)
	defer cancel()
	var requestMutex sync.Mutex
	var externalRequests []string
	t.Cleanup(func() {
		requestMutex.Lock()
		defer requestMutex.Unlock()
		if len(externalRequests) > 0 {
			t.Logf("external browser requests: %v", externalRequests)
		}
	})
	chromedp.ListenTarget(scenarioContext, func(event interface{}) {
		if request, ok := event.(*network.EventRequestWillBeSent); ok {
			requestURL := request.Request.URL
			if requestURL == httpServer.URL || strings.HasPrefix(requestURL, httpServer.URL+"/") {
				return
			}
			if !strings.HasPrefix(requestURL, "http://") && !strings.HasPrefix(requestURL, "https://") {
				return
			}
			requestMutex.Lock()
			externalRequests = append(externalRequests, request.Request.URL)
			requestMutex.Unlock()
		}
	})
	require.NoError(t, chromedp.Run(scenarioContext,
		network.Enable(),
		network.SetBlockedURLs([]string{"https://*", "http://cdn.jsdelivr.net/*"}),
		chromedp.Navigate(httpServer.URL),
	), "navigate to the offline workspace")
	require.NoError(t, chromedp.Run(scenarioContext, chromedp.WaitVisible(repoTreeSelectorConstant+" .wb-node-list .wb-row", chromedp.ByQuery)), "show the offline tree row")
	require.NoError(t, chromedp.Run(scenarioContext, chromedp.WaitVisible(auditRunButtonSelectorConstant, chromedp.ByQuery)), "show the audit button")
	waitForAuditRoots(t, scenarioContext, []string{repositoryCatalog.ExplorerRoot})
	require.NoError(t, chromedp.Run(scenarioContext,
		chromedp.Click(auditRunButtonSelectorConstant, chromedp.ByQuery),
		chromedp.WaitVisible(auditResultsPanelSelectorConstant, chromedp.ByQuery),
	), "run the offline audit")
	results, resultsError := readTextContent(scenarioContext, auditResultsBodySelectorConstant)
	require.NoError(t, resultsError)
	require.Contains(t, results, "offline-repository")
	requestMutex.Lock()
	defer requestMutex.Unlock()
	require.Empty(t, externalRequests, "the embedded audit page must request only its own origin")
}
