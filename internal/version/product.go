package version

import (
	_ "embed"
	"strings"
)

var linkedProductVersion string

//go:embed product-version.txt
var embeddedProductVersion string

// ProductVersion returns the canonical product release stored in this source.
func ProductVersion() string {
	if linked := strings.TrimSpace(linkedProductVersion); linked != "" {
		return linked
	}
	return strings.TrimSpace(embeddedProductVersion)
}
