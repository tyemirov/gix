package tests

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLifecycleTargetsUseInstalledGateway(t *testing.T) {
	repositoryRoot := releaseRepositoryRoot(t)
	makefile, err := os.ReadFile(filepath.Join(repositoryRoot, "Makefile"))
	require.NoError(t, err)
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	application := filepath.Join(workspace, "application with spaces")
	installation := filepath.Join(workspace, "installed runtime")
	for _, directory := range []string{application, installation} {
		require.NoError(t, os.MkdirAll(directory, 0o755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(application, "Makefile"), makefile, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(application, "go.mod"), []byte("module example.invalid/application\n\ngo 1.25.0\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(application, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644))
	initialize := exec.Command("git", "init", "-q", application)
	output, err := initialize.CombinedOutput()
	require.NoError(t, err, string(output))
	probeSource := filepath.Join(workspace, "probe.go")
	require.NoError(t, os.WriteFile(probeSource, []byte(`package main
import ("encoding/json"; "fmt"; "os")
func main() {
 if err := json.NewEncoder(os.Stdout).Encode(os.Args[1:]); err != nil { panic(err) }
 fmt.Fprintln(os.Stderr, "gateway diagnostic")
 if os.Getenv("GATEWAY_TEST_FAIL") == "1" { os.Exit(19) }
}
`), 0o644))
	executable := filepath.Join(installation, "mprlab-gateway")
	build := exec.Command("go", "build", "-o", executable, probeSource)
	output, err = build.CombinedOutput()
	require.NoError(t, err, string(output))
	environment := []string{}
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if name != "MPRLAB_GATEWAY_EXECUTABLE" && name != "MAKEFLAGS" && name != "MFLAGS" && name != "PATH" && name != "GATEWAY_TEST_FAIL" {
			environment = append(environment, entry)
		}
	}
	environment = append(environment, "PATH="+installation+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, phase := range []string{"release", "publish", "deploy"} {
		t.Run(phase, func(t *testing.T) {
			for _, selection := range []string{"", executable} {
				for _, failure := range []string{"0", "1"} {
					args := []string{"--no-print-directory", phase}
					if selection != "" {
						args = append(args, "MPRLAB_GATEWAY_EXECUTABLE="+selection)
					}
					command := exec.Command("make", args...)
					command.Dir = application
					command.Env = append(append([]string{}, environment...), "GATEWAY_TEST_FAIL="+failure)
					var diagnostics bytes.Buffer
					command.Stderr = &diagnostics
					output, err := command.Output()
					if failure == "0" {
						require.NoError(t, err, diagnostics.String())
					} else {
						require.Error(t, err)
						require.Contains(t, diagnostics.String(), "Error 19")
					}
					var received []string
					require.NoError(t, json.Unmarshal(output, &received), string(output))
					require.Equal(t, []string{"app-" + phase, "--app-root", application}, received)
					require.Contains(t, diagnostics.String(), "gateway diagnostic")
				}
			}
			missing := filepath.Join(workspace, "missing-gateway")
			command := exec.Command("make", "--no-print-directory", phase, "MPRLAB_GATEWAY_EXECUTABLE="+missing)
			command.Dir = application
			command.Env = environment
			output, err := command.CombinedOutput()
			require.Error(t, err)
			require.Contains(t, string(output), "Gateway runtime is unavailable: "+missing)
			require.NotContains(t, string(output), "gateway diagnostic")
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
