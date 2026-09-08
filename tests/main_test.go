package tests

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	_ = os.Setenv("GH_TOKEN", "test-token")
	_ = os.Setenv("GITHUB_TOKEN", "test-token")
	result := m.Run()
	if sharedIntegrationBinary.path != "" {
		_ = os.RemoveAll(filepath.Dir(sharedIntegrationBinary.path))
	}
	os.Exit(result)
}
