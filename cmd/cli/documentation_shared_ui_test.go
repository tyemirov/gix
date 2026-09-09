package cli

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"github.com/stretchr/testify/require"
)

const documentationCandidateRevision = "768f25936497c5aabd426197d21c2100b6e5d9a1"
const documentationMenuTrigger = `mpr-footer [data-mpr-dropdown="trigger"]`
const documentationMenuPanel = `mpr-footer [data-mpr-dropdown="panel"]`
const documentationLicenseLink = `mpr-footer [data-mpr-footer="privacy-link"]`
const documentationStylesheetLoaded = `(() => {
	const stylesheet = document.querySelector('link[rel="stylesheet"][href="styles.css"]');
	return Boolean(stylesheet && stylesheet.sheet && !stylesheet.sheet.disabled && stylesheet.sheet.cssRules.length > 0);
})()`

func TestDocumentationSharedUI(t *testing.T) {
	require.NotEmpty(t, locateBrowserExecutable(), "Documentation qualification requires Chrome")
	assets := documentationCandidateAssets(t)
	server := httptest.NewServer(http.FileServer(http.Dir(filepath.Join("..", "..", "docs"))))
	defer server.Close()
	for _, width := range []int64{390, 1280} {
		t.Run(fmt.Sprintf("viewport-%d", width), func(t *testing.T) {
			browserContext := newBrowserTestContext(t)
			scenarioContext, cancel := context.WithTimeout(browserContext, 8*time.Second)
			defer cancel()
			var boundaryErrors atomic.Int64
			var sharedRequests atomic.Int64
			chromedp.ListenTarget(scenarioContext, func(event interface{}) {
				switch current := event.(type) {
				case *cdpruntime.EventExceptionThrown:
					boundaryErrors.Add(1)
				case *fetch.EventRequestPaused:
					go func() {
						body := []byte{}
						contentType := "application/javascript"
						if asset, exists := assets[current.Request.URL]; exists {
							body = asset
							sharedRequests.Add(1)
							if filepath.Ext(current.Request.URL) == ".css" {
								contentType = "text/css"
							}
						}
						executorContext := cdp.WithExecutor(scenarioContext, chromedp.FromContext(scenarioContext).Target)
						responseError := fetch.FulfillRequest(current.RequestID, http.StatusOK).
							WithResponseHeaders([]*fetch.HeaderEntry{{Name: "Content-Type", Value: contentType}}).
							WithBody(base64.StdEncoding.EncodeToString(body)).Do(executorContext)
						if responseError != nil && scenarioContext.Err() == nil {
							boundaryErrors.Add(1)
						}
					}()
				}
			})
			var links [][]string
			var stylesheetLoaded, inViewport, focused bool
			var licenseContent string
			require.NoError(t, chromedp.Run(scenarioContext,
				fetch.Enable().WithPatterns([]*fetch.RequestPattern{{URLPattern: "https://*"}}),
				chromedp.EmulateViewport(width, 900),
				chromedp.Navigate(server.URL),
				chromedp.Evaluate(documentationStylesheetLoaded, &stylesheetLoaded),
			))
			require.True(t, stylesheetLoaded, "Documentation stylesheet must load before geometry checks")
			require.NoError(t, chromedp.Run(scenarioContext,
				chromedp.WaitVisible(documentationMenuTrigger, chromedp.ByQuery),
				chromedp.Focus(documentationMenuTrigger, chromedp.ByQuery),
				chromedp.KeyEvent(kb.Enter),
				chromedp.WaitVisible(documentationMenuPanel, chromedp.ByQuery),
				chromedp.Evaluate(`Array.from(document.querySelectorAll('mpr-footer [data-mpr-dropdown="panel"] a'), node => [node.textContent.trim(),node.getAttribute('href')])`, &links),
				chromedp.Evaluate(`(()=>{const bounds=document.querySelector('mpr-footer [data-mpr-dropdown="panel"]').getBoundingClientRect();return bounds.left>=0 && bounds.right<=innerWidth;})()`, &inViewport),
				chromedp.KeyEvent(kb.Escape),
				chromedp.Evaluate(`(()=>{const trigger=document.querySelector('mpr-footer [data-mpr-dropdown="trigger"]');return trigger===document.activeElement && trigger.getAttribute('aria-expanded')==='false';})()`, &focused),
				chromedp.Click(documentationLicenseLink, chromedp.ByQuery),
				chromedp.WaitVisible(`[role="dialog"]`, chromedp.ByQuery),
				chromedp.Text(`[data-mpr-footer="privacy-modal-content"]`, &licenseContent, chromedp.ByQuery),
				chromedp.KeyEvent(kb.Escape),
				chromedp.Reload(),
				chromedp.Evaluate(documentationStylesheetLoaded, &stylesheetLoaded),
				chromedp.WaitVisible(documentationMenuTrigger, chromedp.ByQuery),
			))
			require.True(t, stylesheetLoaded, "Documentation stylesheet must load after reload")
			require.Equal(t, [][]string{
				{"Marco Polo Research Lab", "https://mprlab.com"},
				{"Gravity Notes", "https://gravity.mprlab.com"},
				{"LoopAware", "https://loopaware.mprlab.com"},
				{"Allergy Wheel", "https://allergy.mprlab.com"},
				{"Social Threader", "https://threader.mprlab.com"},
				{"RSVP", "https://rsvp.mprlab.com"},
				{"Countdown Calendar", "https://countdown.mprlab.com"},
				{"LLM Crossword", "https://llm-crossword.mprlab.com"},
				{"Prompt Bubbles", "https://prompts.mprlab.com"},
				{"Wallpapers", "https://wallpapers.mprlab.com"},
				{"CTX", "https://ctx.mprlab.com"},
			}, links)
			require.True(t, inViewport)
			require.True(t, focused)
			require.Contains(t, licenseContent, "MIT License")
			require.Contains(t, licenseContent, "Copyright (c) 2025")
			require.GreaterOrEqual(t, sharedRequests.Load(), int64(4))
			require.Zero(t, boundaryErrors.Load())
		})
	}
}

func documentationCandidateAssets(t *testing.T) map[string][]byte {
	t.Helper()
	client := http.Client{Timeout: 15 * time.Second}
	assets := map[string][]byte{}
	for name, digest := range map[string]string{
		"mpr-ui.js":  "3e725dbe911470ca934cb46456369479b6ac232eee5ccba2582bf8d939259ae8",
		"mpr-ui.css": "351bbf6c15054528a651571d8c8bd85536eea76c3e574f9335e6cd413878923f",
	} {
		response, requestError := client.Get("https://raw.githubusercontent.com/MarcoPoloResearchLab/mpr-ui/" + documentationCandidateRevision + "/" + name)
		require.NoError(t, requestError)
		require.Equal(t, http.StatusOK, response.StatusCode)
		body, readError := io.ReadAll(response.Body)
		closeError := response.Body.Close()
		require.NoError(t, readError)
		require.NoError(t, closeError)
		require.Equal(t, digest, fmt.Sprintf("%x", sha256.Sum256(body)))
		assets["https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/"+name] = body
	}
	return assets
}
