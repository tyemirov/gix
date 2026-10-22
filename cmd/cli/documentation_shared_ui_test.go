package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"github.com/stretchr/testify/require"
)

const documentationMenuTrigger = `mpr-footer [data-mpr-dropdown="trigger"]`
const documentationMenuPanel = `mpr-footer [data-mpr-dropdown="panel"]`
const documentationLicenseLink = `mpr-footer [data-mpr-footer="privacy-link"]`
const documentationStylesheetLoaded = `(() => {
	const stylesheet = document.querySelector('link[rel="stylesheet"][href="styles.css"]');
	return Boolean(stylesheet && stylesheet.sheet && !stylesheet.sheet.disabled && stylesheet.sheet.cssRules.length > 0);
})()`

func TestDocumentationSharedUI(t *testing.T) {
	require.NotEmpty(t, locateBrowserExecutable(), "Documentation qualification requires Chrome")
	server := httptest.NewServer(http.FileServer(http.Dir(filepath.Join("..", "..", "docs"))))
	defer server.Close()
	for _, width := range []int64{390, 1280} {
		t.Run(fmt.Sprintf("viewport-%d", width), func(t *testing.T) {
			browserContext := newBrowserTestContext(t)
			scenarioContext, cancel := context.WithTimeout(browserContext, 8*time.Second)
			defer cancel()
			dependencies := newDocumentationBrowserDependencies(t, scenarioContext, server.URL)
			var links [][]string
			var stylesheetLoaded, inViewport, focused bool
			var licenseContent string
			require.NoError(t, chromedp.Run(scenarioContext,
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
			require.GreaterOrEqual(t, dependencies.sharedRequests.Load(), int64(4))
			dependencies.assertNoErrors(t)
		})
	}
}
