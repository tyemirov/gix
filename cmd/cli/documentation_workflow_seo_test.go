package cli

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const documentationCanonicalURL = "https://gix.mprlab.com/"
const documentationSocialImageURL = documentationCanonicalURL + "assets/gix-social.png"

func TestDocumentationSharedUIWorkflowAndSEO(t *testing.T) {
	require.NotEmpty(t, locateBrowserExecutable(), "Documentation qualification requires Chrome")
	server := httptest.NewServer(http.FileServer(http.Dir(filepath.Join("..", "..", "docs"))))
	defer server.Close()
	for _, width := range []int64{390, 1280} {
		t.Run(fmt.Sprintf("viewport-%d", width), func(t *testing.T) {
			browserContext := newBrowserTestContext(t)
			scenarioContext, cancel := context.WithTimeout(browserContext, 8*time.Second)
			defer cancel()
			chromedp.ListenTarget(scenarioContext, func(event interface{}) {
				if request, ok := event.(*fetch.EventRequestPaused); ok {
					go func() {
						contentType := "application/javascript"
						if strings.HasSuffix(request.Request.URL, ".css") {
							contentType = "text/css"
						}
						executorContext := cdp.WithExecutor(scenarioContext, chromedp.FromContext(scenarioContext).Target)
						requestError := fetch.FulfillRequest(request.RequestID, http.StatusOK).
							WithResponseHeaders([]*fetch.HeaderEntry{{Name: "Content-Type", Value: contentType}}).
							WithBody("").Do(executorContext)
						if requestError != nil && scenarioContext.Err() == nil {
							t.Errorf("fulfill documentation dependency: %v", requestError)
						}
					}()
				}
			})
			var hero, article, install string
			var metadata map[string]string
			var links, commands []string
			var structuredData []json.RawMessage
			var stylesheetLoaded, noOverflow bool
			require.NoError(t, chromedp.Run(scenarioContext,
				fetch.Enable().WithPatterns([]*fetch.RequestPattern{{URLPattern: "https://*"}}),
				chromedp.EmulateViewport(width, 900),
				chromedp.Navigate(server.URL),
				chromedp.Evaluate(documentationStylesheetLoaded, &stylesheetLoaded),
				chromedp.Evaluate(`document.querySelector('.hero')?.innerText || ''`, &hero),
				chromedp.Evaluate(`document.querySelector('main').innerText`, &article),
				chromedp.Evaluate(`Array.from(document.querySelectorAll('main section'), n=>n.innerText).find(text=>text.includes('go install')) || ''`, &install),
				chromedp.Evaluate(`Object.fromEntries([['title',document.title],['canonical',document.querySelector('link[rel="canonical"]')?.href || ''],...['description','twitter:card','twitter:title','twitter:description','twitter:image','og:type','og:title','og:description','og:url','og:image','og:image:alt'].map(name=>[name,document.querySelector('meta[name="'+name+'"],meta[property="'+name+'"]')?.content || ''])])`, &metadata),
				chromedp.Evaluate(`Array.from(document.querySelectorAll('a[href]'),n=>n.getAttribute('href'))`, &links),
				chromedp.Evaluate(`Array.from(document.querySelectorAll('main code'),n=>n.textContent.trim())`, &commands),
				chromedp.Evaluate(`Array.from(document.querySelectorAll('script[type="application/ld+json"]'),n=>JSON.parse(n.textContent))`, &structuredData),
				chromedp.Evaluate(`document.documentElement.scrollWidth <= innerWidth`, &noOverflow),
			))
			require.True(t, stylesheetLoaded)
			assert.Regexp(t, `(?i)gix sync`, hero, "hero must introduce daily sync")
			assert.Regexp(t, `(?i)(daily|everyday|day.to.day)`, hero)
			require.NotEmpty(t, article, "readable workflow content must accompany the command examples")
			assert.Contains(t, commands, "gix sync", "show the plain sync invocation")
			for label, expression := range map[string]string{
				"remote example":      `gix sync https://github\.com/[^\s]+`,
				"branch example":      `gix sync (?:feature/[^\s]+|qqq)`,
				"discard example":     `gix discard(?:\s|$)`,
				"explicit target":     `(?i)explicit`,
				"default branch":      `(?i)default branch`,
				"pending work":        `(?i)(uncommitted|dirty)`,
				"local commits":       `(?i)(unpublished|local) commits`,
				"empty review":        `(?i)(file changes|empty pull request|no pull request)`,
				"stacked review":      `(?i)(stack|parent branch)`,
				"conflict bounds":     `(?i)(bounded|limit)`,
				"conflict safety":     `(?i)(unresolved|reject|failure)`,
				"publication failure": `(?i)nonzero`,
			} {
				assert.True(t, regexp.MustCompile(expression).MatchString(article), "workflow must explain %s (%s)", label, expression)
			}
			assert.Regexp(t, `(?i)source`, install, "installation must distinguish source from an installed release")
			assert.Regexp(t, `(?i)(installed|release)`, install)
			for _, link := range links {
				if strings.Contains(link, "README.md") || strings.Contains(link, "ARCHITECTURE.md") || strings.Contains(link, "AGENTS.md") {
					assert.True(t, strings.HasPrefix(link, "https://github.com/tyemirov/gix/blob/master/"), "documentation destination: %s", link)
				}
			}
			assert.True(t, noOverflow, "page must fit the viewport")
			assert.NotEmpty(t, metadata["title"])
			assert.LessOrEqual(t, len([]rune(metadata["title"])), 70)
			assert.Regexp(t, `(?i)(sync|git)`, metadata["title"])
			assert.NotEmpty(t, metadata["description"])
			assert.LessOrEqual(t, len([]rune(metadata["description"])), 180)
			assert.Equal(t, documentationCanonicalURL, metadata["canonical"])
			assert.Equal(t, documentationCanonicalURL, metadata["og:url"])
			assert.Equal(t, "website", metadata["og:type"])
			assert.Equal(t, "summary_large_image", metadata["twitter:card"])
			for _, name := range []string{"og:title", "twitter:title"} {
				assert.Equal(t, metadata["title"], metadata[name])
			}
			for _, name := range []string{"og:description", "twitter:description"} {
				assert.Equal(t, metadata["description"], metadata[name])
			}
			for _, name := range []string{"og:image", "twitter:image"} {
				assert.Equal(t, documentationSocialImageURL, metadata[name])
			}
			assert.NotEmpty(t, metadata["og:image:alt"])
			require.Len(t, structuredData, 1)
			var application map[string]interface{}
			require.NoError(t, json.Unmarshal(structuredData[0], &application))
			assert.Equal(t, "https://schema.org", application["@context"])
			assert.Equal(t, "SoftwareApplication", application["@type"])
			assert.Equal(t, "gix", application["name"])
			assert.Equal(t, documentationCanonicalURL, application["url"])
			assert.NotEmpty(t, application["description"])
			assert.NotContains(t, application, "offers")
			assert.NotContains(t, application, "aggregateRating")
			assert.NotContains(t, application, "review")

			var navigation []string
			require.NoError(t, chromedp.Run(scenarioContext, chromedp.Evaluate(`Array.from(document.querySelectorAll('nav a[href^="#"]'),n=>n.getAttribute('href'))`, &navigation)))
			require.NotEmpty(t, navigation)
			for _, destination := range navigation {
				selector := fmt.Sprintf("nav a[href=%q]", destination)
				var focused, targetVisible bool
				require.NoError(t, chromedp.Run(scenarioContext,
					chromedp.Focus(selector, chromedp.ByQuery),
					chromedp.KeyEvent(kb.Tab),
					chromedp.KeyEvent(kb.Tab, chromedp.KeyModifiers(input.ModifierShift)),
					chromedp.Evaluate(`(()=>{const n=document.activeElement,s=getComputedStyle(n);return n.matches(`+fmt.Sprintf("%q", selector)+`) && n.matches(':focus-visible') && ((s.outlineStyle!=='none' && parseFloat(s.outlineWidth)>0)||s.boxShadow!=='none');})()`, &focused),
					chromedp.KeyEvent(kb.Enter),
					chromedp.Evaluate(`(()=>{const n=document.getElementById(`+fmt.Sprintf("%q", strings.TrimPrefix(destination, "#"))+`);if(!n)return false;const b=n.getBoundingClientRect(),nav=document.querySelector('body>header').getBoundingClientRect();return location.hash===`+fmt.Sprintf("%q", destination)+` && b.top>=nav.bottom-1 && b.top<innerHeight;})()`, &targetVisible),
				))
				assert.True(t, focused, "keyboard focus must be visible on %s", destination)
				assert.True(t, targetVisible, "anchor target must clear the sticky navigation: %s", destination)
			}
		})
	}
}

func TestDocumentationSharedUIDiscoveryResources(t *testing.T) {
	server := httptest.NewServer(http.FileServer(http.Dir(filepath.Join("..", "..", "docs"))))
	defer server.Close()
	t.Run("robots", func(t *testing.T) {
		response, requestError := server.Client().Get(server.URL + "/robots.txt")
		require.NoError(t, requestError)
		defer response.Body.Close()
		require.Equal(t, http.StatusOK, response.StatusCode)
		body, readError := io.ReadAll(response.Body)
		require.NoError(t, readError)
		assert.Regexp(t, `(?m)^User-agent:\s*\*\s*$`, string(body))
		assert.Contains(t, string(body), "Sitemap: "+documentationCanonicalURL+"sitemap.xml")
		assert.NotRegexp(t, `(?m)^Disallow:\s*/\s*$`, string(body))
	})
	t.Run("sitemap", func(t *testing.T) {
		response, requestError := server.Client().Get(server.URL + "/sitemap.xml")
		require.NoError(t, requestError)
		defer response.Body.Close()
		require.Equal(t, http.StatusOK, response.StatusCode)
		var sitemap struct {
			XMLName   xml.Name `xml:"urlset"`
			Locations []string `xml:"url>loc"`
		}
		require.NoError(t, xml.NewDecoder(response.Body).Decode(&sitemap))
		assert.Equal(t, "http://www.sitemaps.org/schemas/sitemap/0.9", sitemap.XMLName.Space)
		assert.Contains(t, sitemap.Locations, documentationCanonicalURL)
		for _, location := range sitemap.Locations {
			assert.True(t, strings.HasPrefix(location, documentationCanonicalURL), "canonical sitemap location: %s", location)
		}
	})
	t.Run("share image", func(t *testing.T) {
		response, requestError := server.Client().Get(server.URL + "/assets/gix-social.png")
		require.NoError(t, requestError)
		defer response.Body.Close()
		require.Equal(t, http.StatusOK, response.StatusCode)
		require.Equal(t, "image/png", response.Header.Get("Content-Type"))
		image, decodeError := png.Decode(response.Body)
		require.NoError(t, decodeError)
		assert.Equal(t, 1200, image.Bounds().Dx())
		assert.Equal(t, 630, image.Bounds().Dy())
	})
}
