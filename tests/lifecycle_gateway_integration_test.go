package tests

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLifecycleTargetsDelegateToSiblingGateway(t *testing.T) {
	makefile, err := os.ReadFile(filepath.Join(releaseRepositoryRoot(t), "Makefile"))
	require.NoError(t, err)
	for _, phase := range []string{"release", "publish", "deploy"} {
		t.Run(phase, func(t *testing.T) {
			workspace, pathError := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, pathError)
			application := filepath.Join(workspace, "application with spaces")
			gateway := filepath.Join(workspace, "mprlab-gateway")
			writeReleaseFixtureFile(t, filepath.Join(application, "Makefile"), string(makefile))
			writeReleaseFixtureFile(t, filepath.Join(application, "go.mod"), "module example.invalid/application\n\ngo 1.25.0\n")
			writeReleaseFixtureFile(t, filepath.Join(application, "main.go"), "package main\nfunc main() {}\n")
			runGit(t, application, "init")
			writeReleaseFixtureFile(t, filepath.Join(gateway, "Makefile"), `.PHONY: app-release app-publish app-deploy
app-release app-publish app-deploy:
	@printf '%s\n' '$@' '$(MPRLAB_APP_ROOT)' > received.txt
	@exit $(RESULT)
`)
			for _, exitStatus := range []string{"0", "19"} {
				command := exec.Command("make", "--no-print-directory", phase, "RESULT="+exitStatus)
				command.Dir = application
				output, runError := command.CombinedOutput()
				if exitStatus == "0" {
					require.NoError(t, runError, string(output))
				} else {
					require.Error(t, runError, string(output))
				}
				received, readError := os.ReadFile(filepath.Join(gateway, "received.txt"))
				require.NoError(t, readError, string(output))
				require.Equal(t, "app-"+phase+"\n"+application+"\n", string(received))
			}
			require.NoError(t, os.RemoveAll(gateway))
			command := exec.Command("make", "--no-print-directory", phase)
			command.Dir = application
			output, runError := command.CombinedOutput()
			require.Error(t, runError, string(output))
			require.Contains(t, string(output), "mprlab-gateway")
			require.False(t, strings.Contains(string(output), "scripts/release"), string(output))
		})
	}
}

func TestLifecyclePagesSourceArchive(t *testing.T) {
	repositoryRoot := releaseRepositoryRoot(t)
	fixture := createGitRepository(t, gitRepositoryOptions{InitialBranch: "master"})
	runGit(t, fixture, "config", "user.useConfigOnly", "true")
	configureGitIdentity(t, fixture)
	source := filepath.Join(repositoryRoot, "docs")
	require.NoError(t, filepath.WalkDir(source, func(path string, entry os.DirEntry, walkError error) error {
		if walkError != nil {
			return walkError
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		writeReleaseFixtureFile(t, filepath.Join(fixture, "docs", relative), string(data))
		return nil
	}))
	runGit(t, fixture, "add", "docs")
	runGit(t, fixture, "commit", "-m", "Pages source fixture")
	command := exec.Command("git", "archive", "--format=tar", "HEAD:docs")
	command.Dir = fixture
	archive, err := command.Output()
	require.NoError(t, err)
	files := map[string][]byte{}
	reader := tar.NewReader(bytes.NewReader(archive))
	for {
		header, readError := reader.Next()
		if readError == io.EOF {
			break
		}
		require.NoError(t, readError)
		if header.Typeflag == tar.TypeDir {
			continue
		}
		contents, readError := io.ReadAll(reader)
		require.NoError(t, readError)
		files[header.Name] = contents
	}
	for _, name := range []string{"index.html", "styles.css"} {
		expected, readError := os.ReadFile(filepath.Join(source, name))
		require.NoError(t, readError)
		require.Equal(t, expected, files[name], name)
	}
	for _, name := range []string{"CNAME", ".nojekyll", ".mprlab-release.json", ".gitattributes", "GX-412-refactor-plan.md", "policy_refactor_plan.md", "readme_config_test.go"} {
		require.NotContains(t, files, name)
	}
}
