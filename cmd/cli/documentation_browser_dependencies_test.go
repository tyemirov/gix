package cli

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/require"
)

const documentationCandidateRevision = "768f25936497c5aabd426197d21c2100b6e5d9a1"

//go:embed testdata/mpr-ui/mpr-ui.js testdata/mpr-ui/mpr-ui.css
var documentationSharedUIFixtures embed.FS

type documentationBrowserDependencies struct {
	sharedRequests atomic.Int64
	mutex          sync.Mutex
	errors         []string
}

func newDocumentationBrowserDependencies(t *testing.T, browserContext context.Context, origin string) *documentationBrowserDependencies {
	t.Helper()
	assets := documentationCandidateAssets(t)
	dependencies := &documentationBrowserDependencies{}
	chromedp.ListenTarget(browserContext, func(event interface{}) {
		switch current := event.(type) {
		case *cdpruntime.EventExceptionThrown:
			dependencies.recordError(fmt.Sprintf("browser exception: %+v", current.ExceptionDetails))
		case *fetch.EventRequestPaused:
			go func() {
				executorContext := cdp.WithExecutor(browserContext, chromedp.FromContext(browserContext).Target)
				requestURL, parseError := url.Parse(current.Request.URL)
				var responseError error
				switch {
				case parseError == nil && requestURL.Scheme+"://"+requestURL.Host == origin:
					responseError = fetch.ContinueRequest(current.RequestID).Do(executorContext)
				case assets[current.Request.URL] != nil:
					contentType := "application/javascript"
					if requestURL.Path == "/gh/MarcoPoloResearchLab/mpr-ui@latest/mpr-ui.css" {
						contentType = "text/css"
					}
					dependencies.sharedRequests.Add(1)
					responseError = fetch.FulfillRequest(current.RequestID, http.StatusOK).
						WithResponseHeaders([]*fetch.HeaderEntry{{Name: "Content-Type", Value: contentType}}).
						WithBody(base64.StdEncoding.EncodeToString(assets[current.Request.URL])).Do(executorContext)
				case current.Request.URL == "https://www.googletagmanager.com/gtag/js?id=G-JNXM3KYJQ9",
					current.Request.URL == "https://www.loopaware.com/pixel.js?site_id=64026286-27e2-4b89-9ec5-956878d731d2":
					responseError = fetch.FulfillRequest(current.RequestID, http.StatusOK).
						WithResponseHeaders([]*fetch.HeaderEntry{{Name: "Content-Type", Value: "application/javascript"}}).
						WithBody(base64.StdEncoding.EncodeToString([]byte("/* Local analytics boundary. */"))).Do(executorContext)
				default:
					dependencies.recordError("unexpected external request: " + current.Request.URL)
					responseError = fetch.FailRequest(current.RequestID, network.ErrorReasonBlockedByClient).Do(executorContext)
				}
				if responseError != nil && browserContext.Err() == nil {
					dependencies.recordError(fmt.Sprintf("handle request %s: %v", current.Request.URL, responseError))
				}
			}()
		}
	})
	require.NoError(t, chromedp.Run(browserContext,
		fetch.Enable().WithPatterns([]*fetch.RequestPattern{{URLPattern: "http://*"}, {URLPattern: "https://*"}}),
	))
	return dependencies
}

func (dependencies *documentationBrowserDependencies) recordError(message string) {
	dependencies.mutex.Lock()
	defer dependencies.mutex.Unlock()
	dependencies.errors = append(dependencies.errors, message)
}

func (dependencies *documentationBrowserDependencies) assertNoErrors(t *testing.T) {
	t.Helper()
	dependencies.mutex.Lock()
	defer dependencies.mutex.Unlock()
	require.Empty(t, dependencies.errors)
}

func documentationCandidateAssets(t *testing.T) map[string][]byte {
	t.Helper()
	assets := map[string][]byte{}
	for name, digest := range map[string]string{
		"mpr-ui.js":  "3e725dbe911470ca934cb46456369479b6ac232eee5ccba2582bf8d939259ae8",
		"mpr-ui.css": "351bbf6c15054528a651571d8c8bd85536eea76c3e574f9335e6cd413878923f",
	} {
		body, readError := documentationSharedUIFixtures.ReadFile("testdata/mpr-ui/" + name)
		require.NoError(t, readError)
		require.Equal(t, digest, fmt.Sprintf("%x", sha256.Sum256(body)))
		assets["https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/"+name] = body
	}
	return assets
}

func TestDocumentationSharedUIRejectsUnknownExternalRequest(t *testing.T) {
	const unknownURL = "https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/unexpected.js"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		_, writeError := fmt.Fprintf(writer, `<script src="%s"></script>`, unknownURL)
		if writeError != nil {
			t.Errorf("write browser page: %v", writeError)
		}
	}))
	defer server.Close()
	browserContext := newBrowserTestContext(t)
	scenarioContext, cancel := context.WithTimeout(browserContext, 8*time.Second)
	defer cancel()
	dependencies := newDocumentationBrowserDependencies(t, scenarioContext, server.URL)
	require.NoError(t, chromedp.Run(scenarioContext, chromedp.Navigate(server.URL)))
	dependencies.mutex.Lock()
	defer dependencies.mutex.Unlock()
	require.Equal(t, []string{"unexpected external request: " + unknownURL}, dependencies.errors)
	require.Zero(t, dependencies.sharedRequests.Load())
}
