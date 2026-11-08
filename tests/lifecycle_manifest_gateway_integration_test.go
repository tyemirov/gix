//go:build gateway

package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLifecycleManifestUsesInstalledGateway(t *testing.T) {
	gateway := os.Getenv("MPRLAB_GATEWAY_EXECUTABLE")
	if gateway == "" {
		gateway = "mprlab-gateway"
	}
	executable, err := exec.LookPath(gateway)
	require.NoError(t, err, "install Gateway v9 or set MPRLAB_GATEWAY_EXECUTABLE")
	manifest, err := os.ReadFile(filepath.Join(releaseRepositoryRoot(t), ".mprlab", "deploy", "resources.yml"))
	require.NoError(t, err)
	workspace := t.TempDir()
	probeSource := filepath.Join(workspace, "runner.go")
	require.NoError(t, os.WriteFile(probeSource, []byte(`package main
import ("fmt"; "os")
func main() {
 if len(os.Args) > 1 && os.Args[1] == "--version" { fmt.Println("ansible-playbook [core 2.20.0]"); return }
 for _, arg := range os.Args[1:] { if arg == "--list" { fmt.Println(`+"`"+`{"_meta":{"hostvars":{"tutosh":{"ansible_host":"127.0.0.1","ansible_user":"fixture","mprlab_root":"/tmp/mprlab-test"}}},"all":{"children":["gateway"]},"gateway":{"hosts":["tutosh"]}}`+"`"+`); return } }
 fmt.Println("gateway manifest accepted")
 os.Exit(19)
}
`), 0o644))
	runner := filepath.Join(workspace, "ansible-probe")
	build := exec.Command("go", "build", "-o", runner, probeSource)
	output, err := build.CombinedOutput()
	require.NoError(t, err, string(output))
	application := createGitRepository(t, gitRepositoryOptions{InitialBranch: "master"})
	application, err = filepath.EvalSymlinks(application)
	require.NoError(t, err)
	configureGitIdentity(t, application)
	writeReleaseFixtureFile(t, filepath.Join(application, ".gitignore"), ".env\n")
	writeReleaseFixtureFile(t, filepath.Join(application, "go.mod"), "module github.com/tyemirov/gix\n\ngo 1.27.0\n")
	writeReleaseFixtureFile(t, filepath.Join(application, "main.go"), "package main\nfunc main() {}\n")
	writeReleaseFixtureFile(t, filepath.Join(application, "docs", "index.html"), "<html><title>Gateway fixture</title></html>\n")
	writeReleaseFixtureFile(t, filepath.Join(application, ".mprlab", "deploy", ".env"), "DEPLOY_SUDO_PASSWORD_TUTOSH=gateway-test-secret\n")
	manifestPath := filepath.Join(application, ".mprlab", "deploy", "resources.yml")
	writeReleaseFixtureFile(t, manifestPath, string(manifest))
	runGit(t, application, "add", ".")
	runGit(t, application, "commit", "-m", "Application manifest fixture")
	remote := filepath.Join(workspace, "origin.git")
	runGit(t, application, "init", "--bare", "--initial-branch=master", remote)
	runGit(t, application, "remote", "add", "origin", remote)
	runGit(t, application, "push", "--set-upstream", "origin", "master")
	inventory := filepath.Join(workspace, "inventory.yml")
	writeReleaseFixtureFile(t, inventory, "all:\n  children:\n    gateway:\n      hosts:\n        tutosh:\n          ansible_host: 127.0.0.1\n")
	for _, phase := range []string{"release", "publish", "deploy"} {
		t.Run(phase, func(t *testing.T) {
			command := exec.Command(executable, "app-plan-"+phase, "--app-root", application,
				"--operator-root", workspace, "--ansible-playbook", runner,
				"--ansible-inventory", inventory, "--ansible-inventory-bin", runner)
			output, err := command.CombinedOutput()
			var exitError *exec.ExitError
			require.ErrorAs(t, err, &exitError, string(output))
			require.Equal(t, 19, exitError.ExitCode(), string(output))
			require.Contains(t, string(output), "gateway manifest accepted")
			require.NotContains(t, string(output), "gateway-test-secret")
		})
	}
}
