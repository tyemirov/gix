package tests

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	releaseMakeCommandTimeout           = 30 * time.Second
	releaseToolDirectoryRelativePath    = "scripts/release"
	releaseArtifactDirectoryVariable    = "RELEASE_ARTIFACT_DIR"
	releaseFakeGoLogVariable            = "GIX_RELEASE_FAKE_GO_LOG"
	releaseFakeGoFailureTargetVariable  = "GIX_RELEASE_FAKE_GO_FAILURE_TARGET"
	releaseFakeGoMissingTargetVariable  = "GIX_RELEASE_FAKE_GO_MISSING_TARGET"
	releaseFakeManifestVariable         = "GIX_RELEASE_FAKE_MANIFEST"
	releaseFakePagesArchiveVariable     = "GIX_RELEASE_FAKE_PAGES_ARCHIVE"
	releaseExpectedFailureTarget        = "linux/arm64"
	releaseMissingArtifactErrorFragment = "missing release artifact"
	releaseManifestMismatchFragment     = "published release manifest does not match the locally prepared release"
	releaseFixtureVersion               = "v9.8.7"
)

var releaseDeployRequiredCommands = []string{
	"awk",
	"cp",
	"curl",
	"find",
	"gh",
	"git",
	"head",
	"mkdir",
	"mktemp",
	"python3",
	"rm",
	"shasum",
	"sleep",
	"tar",
}

var releaseRequiredHelperFiles = []string{
	"deploy_pages_artifact.sh",
	"prepare_pages_artifact.sh",
	"prepare_release.sh",
	"publish_release.sh",
	"release_helper.py",
}

func TestReleaseTargetsUseRepositoryOwnedHelpers(testInstance *testing.T) {
	repositoryRoot := releaseRepositoryRoot(testInstance)
	cleanCheckout := testInstance.TempDir()
	copyReleaseFile(testInstance, repositoryRoot, cleanCheckout, "Makefile")
	for _, helperFile := range releaseRequiredHelperFiles {
		copyReleaseFile(
			testInstance,
			repositoryRoot,
			cleanCheckout,
			filepath.Join(releaseToolDirectoryRelativePath, helperFile),
		)
	}

	testCases := []struct {
		name              string
		arguments         []string
		forbiddenHelpText string
	}{
		{name: "prepare", arguments: []string{"release", "RELEASE_ARGS=--help"}},
		{name: "publish", arguments: []string{"publish", "PUBLISH_RELEASE_ARGS=--help"}, forbiddenHelpText: "--remote"},
		{name: "deploy", arguments: []string{"deploy", "PAGES_DEPLOY_ARGS=--help"}},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(t *testing.T) {
			outputText, runError := runReleaseMakeCommand(t, cleanCheckout, nil, testCase.arguments...)
			require.NoError(t, runError, outputText)
			require.Contains(t, strings.ToLower(outputText), "usage:")
			if testCase.forbiddenHelpText != "" {
				require.NotContains(t, outputText, testCase.forbiddenHelpText)
			}
		})
	}
}

func TestReleaseArtifactsStopAtFailedPlatformBuild(testInstance *testing.T) {
	repositoryRoot := releaseRepositoryRoot(testInstance)
	buildLogPath := filepath.Join(testInstance.TempDir(), "go-builds.log")
	fakeGoPath := buildStubbedExecutablePath(testInstance, "go", releaseFakeGoScript)
	environment := map[string]string{
		"PATH":                             fakeGoPath,
		releaseFakeGoLogVariable:           buildLogPath,
		releaseFakeGoFailureTargetVariable: releaseExpectedFailureTarget,
		releaseFakeGoMissingTargetVariable: "",
	}

	outputText, runError := runReleaseMakeCommand(
		testInstance,
		repositoryRoot,
		environment,
		"release-artifacts",
		releaseArtifactDirectoryVariable+"="+testInstance.TempDir(),
	)
	require.Error(testInstance, runError, outputText)

	buildLog, readError := os.ReadFile(buildLogPath)
	require.NoError(testInstance, readError)
	require.Equal(testInstance, "linux/amd64\nlinux/arm64\n", string(buildLog))
}

func TestReleaseArtifactsRejectMissingExpectedOutput(testInstance *testing.T) {
	repositoryRoot := releaseRepositoryRoot(testInstance)
	buildLogPath := filepath.Join(testInstance.TempDir(), "go-builds.log")
	fakeGoPath := buildStubbedExecutablePath(testInstance, "go", releaseFakeGoScript)
	environment := map[string]string{
		"PATH":                             fakeGoPath,
		releaseFakeGoLogVariable:           buildLogPath,
		releaseFakeGoFailureTargetVariable: "",
		releaseFakeGoMissingTargetVariable: releaseExpectedFailureTarget,
	}

	outputText, runError := runReleaseMakeCommand(
		testInstance,
		repositoryRoot,
		environment,
		"release-artifacts",
		releaseArtifactDirectoryVariable+"="+testInstance.TempDir(),
	)
	require.Error(testInstance, runError, outputText)
	require.Contains(testInstance, outputText, releaseMissingArtifactErrorFragment)

	buildLog, readError := os.ReadFile(buildLogPath)
	require.NoError(testInstance, readError)
	require.Equal(testInstance, "linux/amd64\nlinux/arm64\n", string(buildLog))
}

func TestPagesDeployRejectsPublishedManifestThatDiffersFromPreparedRelease(testInstance *testing.T) {
	repositoryRoot := releaseRepositoryRoot(testInstance)
	fixtureRepository := createGitRepository(testInstance, gitRepositoryOptions{InitialBranch: "master"})
	preparedManifestPath := filepath.Join(fixtureRepository, ".git", "mprlab-release", "manifest.json")
	writeReleaseFixtureFile(testInstance, preparedManifestPath, releasePreparedManifestFixture)

	replacementDirectory := testInstance.TempDir()
	replacementManifestPath := filepath.Join(replacementDirectory, "manifest.json")
	replacementManifest := strings.Replace(
		releasePreparedManifestFixture,
		releasePreparedPagesHash,
		releaseReplacementPagesHash,
		1,
	)
	writeReleaseFixtureFile(testInstance, replacementManifestPath, replacementManifest)
	replacementArchivePath := filepath.Join(replacementDirectory, "pages.tar.gz")
	writeReleaseFixtureFile(testInstance, replacementArchivePath, "replacement archive\n")

	pathVariable := buildStubbedExecutablePath(testInstance, "gh", releaseFakeGHDownloadScript)
	outputText, runError := runReleaseDeployScript(
		testInstance,
		repositoryRoot,
		fixtureRepository,
		integrationCommandOptions{
			PathVariable: pathVariable,
			EnvironmentOverrides: map[string]string{
				releaseFakeManifestVariable:     replacementManifestPath,
				releaseFakePagesArchiveVariable: replacementArchivePath,
			},
		},
		"--version", releaseFixtureVersion,
		"--skip-configure",
		"--skip-verify",
	)
	require.Error(testInstance, runError, outputText)
	require.Contains(testInstance, outputText, releaseManifestMismatchFragment)
}

func TestPagesDeployPreflightsIntegrityDependencies(testInstance *testing.T) {
	repositoryRoot := releaseRepositoryRoot(testInstance)
	for _, missingCommand := range []string{"curl", "shasum"} {
		testInstance.Run(missingCommand, func(t *testing.T) {
			pathVariable := buildReleaseDependencyPath(t, missingCommand)
			outputText, runError := runReleaseDeployScript(
				t,
				repositoryRoot,
				t.TempDir(),
				integrationCommandOptions{PathVariable: pathVariable},
				"--version", releaseFixtureVersion,
				"--skip-configure",
				"--skip-verify",
			)
			require.Error(t, runError, outputText)
			require.Contains(t, outputText, "error: "+missingCommand+" is required")
		})
	}
}

func releaseRepositoryRoot(testInstance *testing.T) string {
	testInstance.Helper()
	workingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	return filepath.Dir(workingDirectory)
}

func copyReleaseFile(testInstance *testing.T, sourceRoot string, destinationRoot string, relativePath string) {
	testInstance.Helper()
	sourcePath := filepath.Join(sourceRoot, relativePath)
	fileContents, readError := os.ReadFile(sourcePath)
	require.NoError(testInstance, readError, "repository-owned release file is unavailable: %s", relativePath)
	fileInfo, statError := os.Stat(sourcePath)
	require.NoError(testInstance, statError)

	destinationPath := filepath.Join(destinationRoot, relativePath)
	require.NoError(testInstance, os.MkdirAll(filepath.Dir(destinationPath), 0o755))
	require.NoError(testInstance, os.WriteFile(destinationPath, fileContents, fileInfo.Mode().Perm()))
}

func runReleaseMakeCommand(
	testInstance *testing.T,
	workingDirectory string,
	environmentOverrides map[string]string,
	arguments ...string,
) (string, error) {
	testInstance.Helper()
	executionContext, cancelFunction := context.WithTimeout(context.Background(), releaseMakeCommandTimeout)
	defer cancelFunction()

	makeArguments := append([]string{"FAST_TEST_PACKAGES="}, arguments...)
	command := exec.CommandContext(executionContext, "make", makeArguments...)
	command.Dir = workingDirectory
	command.Env = buildCommandEnvironment(integrationCommandOptions{EnvironmentOverrides: environmentOverrides})
	outputBytes, runError := command.CombinedOutput()
	return string(outputBytes), runError
}

func runReleaseDeployScript(
	testInstance *testing.T,
	repositoryRoot string,
	workingDirectory string,
	commandOptions integrationCommandOptions,
	arguments ...string,
) (string, error) {
	testInstance.Helper()
	executionContext, cancelFunction := context.WithTimeout(context.Background(), releaseMakeCommandTimeout)
	defer cancelFunction()

	scriptPath := filepath.Join(repositoryRoot, releaseToolDirectoryRelativePath, "deploy_pages_artifact.sh")
	commandArguments := append([]string{scriptPath}, arguments...)
	command := exec.CommandContext(executionContext, "/bin/bash", commandArguments...)
	command.Dir = workingDirectory
	command.Env = buildCommandEnvironment(commandOptions)
	outputBytes, runError := command.CombinedOutput()
	return string(outputBytes), runError
}

func buildReleaseDependencyPath(testInstance *testing.T, missingCommand string) string {
	testInstance.Helper()
	stubDirectory := testInstance.TempDir()
	for _, commandName := range releaseDeployRequiredCommands {
		if commandName == missingCommand {
			continue
		}
		stubPath := filepath.Join(stubDirectory, commandName)
		require.NoError(testInstance, os.WriteFile(stubPath, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	}
	return stubDirectory
}

func writeReleaseFixtureFile(testInstance *testing.T, path string, contents string) {
	testInstance.Helper()
	require.NoError(testInstance, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(testInstance, os.WriteFile(path, []byte(contents), 0o644))
}

const releaseFakeGoScript = `#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" == "list" ]]; then
  exit 0
fi

target="${GOOS}/${GOARCH}"
printf '%s\n' "${target}" >>"${GIX_RELEASE_FAKE_GO_LOG}"
if [[ "${target}" == "${GIX_RELEASE_FAKE_GO_FAILURE_TARGET}" ]]; then
  exit 42
fi

output_path=""
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "-o" ]]; then
    output_path="$2"
    break
  fi
  shift
done
[[ -n "${output_path}" ]] || { echo "missing -o" >&2; exit 43; }
if [[ "${target}" != "${GIX_RELEASE_FAKE_GO_MISSING_TARGET}" ]]; then
  mkdir -p "$(dirname "${output_path}")"
  printf 'fixture\n' >"${output_path}"
fi
`

const (
	releasePreparedPagesHash       = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	releaseReplacementPagesHash    = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	releasePreparedManifestFixture = `{
  "artifact_kind": "mprlab.release",
  "default_branch": "master",
  "notes_sha256": "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
  "payloads": [
    {
      "path": "payloads/release-assets/pages.tar.gz",
      "sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "size": 20
    }
  ],
  "release_commit": "1111111111111111111111111111111111111111",
  "release_timestamp": "2026-07-09T12:00:00-07:00",
  "schema_version": 2,
  "source_commit": "2222222222222222222222222222222222222222",
  "version": "v9.8.7"
}
`
	releaseFakeGHDownloadScript = `#!/usr/bin/env bash
set -euo pipefail

[[ "${1:-}" == "release" && "${2:-}" == "download" ]] || { echo "unexpected gh invocation: $*" >&2; exit 41; }
shift 2
download_directory=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dir) download_directory="$2"; shift 2 ;;
    *) shift ;;
  esac
done
[[ -n "${download_directory}" ]] || { echo "missing download directory" >&2; exit 42; }
cp "${GIX_RELEASE_FAKE_MANIFEST}" "${download_directory}/manifest.json"
cp "${GIX_RELEASE_FAKE_PAGES_ARCHIVE}" "${download_directory}/pages.tar.gz"
`
)
