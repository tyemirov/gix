package tests

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	releaseMakeCommandTimeout             = 30 * time.Second
	releaseToolDirectoryRelativePath      = "scripts/release"
	releaseArtifactDirectoryVariable      = "RELEASE_ARTIFACT_DIR"
	releaseFakeGoLogVariable              = "GIX_RELEASE_FAKE_GO_LOG"
	releaseFakeGoFailureTargetVariable    = "GIX_RELEASE_FAKE_GO_FAILURE_TARGET"
	releaseFakeGoMissingTargetVariable    = "GIX_RELEASE_FAKE_GO_MISSING_TARGET"
	releaseFakeManifestVariable           = "GIX_RELEASE_FAKE_MANIFEST"
	releaseFakePagesArchiveVariable       = "GIX_RELEASE_FAKE_PAGES_ARCHIVE"
	releaseFakePagesMarkerVariable        = "GIX_RELEASE_FAKE_PAGES_MARKER"
	releaseFakeGHLogVariable              = "GIX_RELEASE_FAKE_GH_LOG"
	releaseFakePublishedDirectoryVariable = "GIX_RELEASE_FAKE_PUBLISHED_DIRECTORY"
	releaseFakeReleaseViewVariable        = "GIX_RELEASE_FAKE_RELEASE_VIEW"
	releaseFakeMakeLogVariable            = "GIX_RELEASE_FAKE_MAKE_LOG"
	releaseFakeMakeFailureTargetVariable  = "GIX_RELEASE_FAKE_MAKE_FAILURE_TARGET"
	releaseFakeMakeUnsafePayloadVariable  = "GIX_RELEASE_FAKE_MAKE_UNSAFE_PAYLOAD"
	releaseFakePagesBuildCounterVariable  = "GIX_RELEASE_FAKE_PAGES_BUILD_COUNTER"
	releaseFakePagesBuildStatesVariable   = "GIX_RELEASE_FAKE_PAGES_BUILD_STATES"
	releaseFakePagesConfigStateVariable   = "GIX_RELEASE_FAKE_PAGES_CONFIG_STATE"
	releaseFakeReleaseCommitVariable      = "GIX_RELEASE_FAKE_RELEASE_COMMIT"
	releaseFakeRemoteVariable             = "GIX_RELEASE_FAKE_REMOTE"
	releaseRealGitVariable                = "GIX_RELEASE_REAL_GIT"
	releaseFakeVersionVariable            = "GIX_RELEASE_FIXTURE_VERSION"
	releaseExpectedFailureTarget          = "linux/arm64"
	releaseMissingArtifactErrorFragment   = "missing release artifact"
	releaseManifestMismatchFragment       = "published release manifest does not match the locally prepared release"
	releaseFixtureVersion                 = "v9.8.7"
	releaseFixtureURL                     = "https://pages.example.invalid"
	releaseFixtureReleaseCommit           = "1111111111111111111111111111111111111111"
	releaseFixtureSourceCommit            = "2222222222222222222222222222222222222222"
)

var releaseDeployRequiredCommands = []string{
	"awk",
	"cat",
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

func TestReleaseExactTagReusesVerifiedLocalReceipt(testInstance *testing.T) {
	fixture := newExactReleaseFixture(testInstance)
	before := readReleaseArtifactTree(testInstance, fixture.artifactDirectory)
	makeLogPath := filepath.Join(testInstance.TempDir(), "make.log")
	ghLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	pathVariable := buildReleaseStubbedExecutablePath(testInstance, map[string]string{
		"gh":   releaseFakeGHRecoveryScript,
		"make": releaseFakeMakeScript,
	})

	outputText, runError := runReleasePrepareReleaseScript(
		testInstance,
		fixture.repositoryRoot,
		fixture.repositoryPath,
		integrationCommandOptions{
			PathVariable: pathVariable,
			EnvironmentOverrides: map[string]string{
				releaseFakeGHLogVariable:             ghLogPath,
				releaseFakeMakeLogVariable:           makeLogPath,
				releaseFakeMakeFailureTargetVariable: "ci",
			},
		},
	)
	require.NoError(testInstance, runError, outputText)
	require.Contains(testInstance, outputText, "Reused sealed release "+fixture.version)
	require.Equal(testInstance, before, readReleaseArtifactTree(testInstance, fixture.artifactDirectory))
	require.NoFileExists(testInstance, makeLogPath)
	require.NoFileExists(testInstance, ghLogPath)
}

func TestReleaseExactTagRecoversPublishedReceiptAndRejectsConflict(testInstance *testing.T) {
	testCases := []struct {
		name           string
		localState     string
		mutateManifest func(string) string
		expectError    bool
		expectedOutput string
	}{
		{
			name:           "matching-published-release-from-staging",
			localState:     "staging",
			expectedOutput: "Recovered and reused sealed release " + releaseFixtureVersion,
		},
		{
			name:           "matching-published-release-with-missing-payload",
			localState:     "missing-payload",
			expectedOutput: "Recovered and reused sealed release " + releaseFixtureVersion,
		},
		{
			name:       "conflicting-published-manifest",
			localState: "staging",
			mutateManifest: func(manifest string) string {
				return strings.Replace(manifest, `"version": "`+releaseFixtureVersion+`"`, `"version": "v9.8.6"`, 1)
			},
			expectError:    true,
			expectedOutput: "published release manifest does not match exact tag",
		},
		{
			name:       "published-manifest-without-timestamp",
			localState: "staging",
			mutateManifest: func(manifest string) string {
				return strings.Replace(manifest, `  "release_timestamp": "2026-08-06T14:23:35-07:00",`+"\n", "", 1)
			},
			expectError:    true,
			expectedOutput: "published release manifest has no release timestamp",
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(t *testing.T) {
			fixture := newExactReleaseFixture(t)
			expectedReceipt := readReleaseArtifactTree(t, fixture.artifactDirectory)
			manifest := fixture.manifest
			if testCase.mutateManifest != nil {
				manifest = testCase.mutateManifest(manifest)
			}
			publishedDirectory, releaseViewPath := fixture.writePublishedRelease(t, manifest)
			switch testCase.localState {
			case "staging":
				require.NoError(t, os.RemoveAll(fixture.artifactDirectory))
				writeReleaseFixtureFile(t, filepath.Join(fixture.artifactDirectory, "staging.json"), `{"artifact_kind":"mprlab.release.staging","version":"v9.8.8"}`+"\n")
			case "missing-payload":
				require.NoError(t, os.Remove(filepath.Join(fixture.artifactDirectory, "payloads", "release-assets", "pages.tar.gz")))
			default:
				t.Fatalf("unknown local state %q", testCase.localState)
			}
			incompleteReceipt := readReleaseArtifactTree(t, fixture.artifactDirectory)
			makeLogPath := filepath.Join(t.TempDir(), "make.log")
			ghLogPath := filepath.Join(t.TempDir(), "gh.log")
			pathVariable := buildReleaseStubbedExecutablePath(t, map[string]string{
				"gh":   releaseFakeGHRecoveryScript,
				"make": releaseFakeMakeScript,
			})

			outputText, runError := runReleasePrepareReleaseScript(
				t,
				fixture.repositoryRoot,
				fixture.repositoryPath,
				integrationCommandOptions{
					PathVariable: pathVariable,
					EnvironmentOverrides: map[string]string{
						releaseFakeGHLogVariable:              ghLogPath,
						releaseFakeMakeLogVariable:            makeLogPath,
						releaseFakeMakeFailureTargetVariable:  "ci",
						releaseFakePublishedDirectoryVariable: publishedDirectory,
						releaseFakeReleaseViewVariable:        releaseViewPath,
					},
				},
			)
			if testCase.expectError {
				require.Error(t, runError, outputText)
				require.Equal(t, incompleteReceipt, readReleaseArtifactTree(t, fixture.artifactDirectory))
			} else {
				require.NoError(t, runError, outputText)
				require.Equal(t, expectedReceipt, readReleaseArtifactTree(t, fixture.artifactDirectory))
			}
			require.Contains(t, outputText, testCase.expectedOutput)
			require.NoFileExists(t, makeLogPath)
		})
	}
}

func TestReleaseExactTagRejectsConflictingLocalReceipt(testInstance *testing.T) {
	fixture := newExactReleaseFixture(testInstance)
	payloadPath := filepath.Join(fixture.artifactDirectory, "payloads", "release-assets", "pages.tar.gz")
	writeReleaseFixtureFile(testInstance, payloadPath, "tampered pages\n")
	before := readReleaseArtifactTree(testInstance, fixture.artifactDirectory)
	makeLogPath := filepath.Join(testInstance.TempDir(), "make.log")
	ghLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	pathVariable := buildReleaseStubbedExecutablePath(testInstance, map[string]string{
		"gh":   releaseFakeGHRecoveryScript,
		"make": releaseFakeMakeScript,
	})

	outputText, runError := runReleasePrepareReleaseScript(
		testInstance,
		fixture.repositoryRoot,
		fixture.repositoryPath,
		integrationCommandOptions{
			PathVariable: pathVariable,
			EnvironmentOverrides: map[string]string{
				releaseFakeGHLogVariable:             ghLogPath,
				releaseFakeMakeLogVariable:           makeLogPath,
				releaseFakeMakeFailureTargetVariable: "ci",
			},
		},
	)
	require.Error(testInstance, runError, outputText)
	require.Contains(testInstance, outputText, "local sealed release conflicts with exact tag")
	require.Equal(testInstance, before, readReleaseArtifactTree(testInstance, fixture.artifactDirectory))
	require.NoFileExists(testInstance, makeLogPath)
	require.NoFileExists(testInstance, ghLogPath)
}

func TestReleaseCandidateFailurePreservesCanonicalReceipt(testInstance *testing.T) {
	fixture := newExactReleaseFixture(testInstance)
	before := readReleaseArtifactTree(testInstance, fixture.artifactDirectory)
	writeReleaseFixtureFile(testInstance, filepath.Join(fixture.repositoryPath, "README.md"), "next release\n")
	runGit(testInstance, fixture.repositoryPath, "add", "README.md")
	runGit(testInstance, fixture.repositoryPath, "commit", "-m", "feat: next release")
	makeLogPath := filepath.Join(testInstance.TempDir(), "make.log")
	pathVariable := buildReleaseStubbedExecutablePath(testInstance, map[string]string{"make": releaseFakeMakeScript})

	outputText, runError := runReleasePrepareReleaseScript(
		testInstance,
		fixture.repositoryRoot,
		fixture.repositoryPath,
		integrationCommandOptions{
			PathVariable: pathVariable,
			EnvironmentOverrides: map[string]string{
				"RELEASE_ARTIFACT_TARGETS":           "fixture-artifact",
				releaseFakeMakeLogVariable:           makeLogPath,
				releaseFakeMakeFailureTargetVariable: "fixture-artifact",
			},
		},
	)
	require.Error(testInstance, runError, outputText)
	require.Contains(testInstance, outputText, "fixture artifact failure")
	require.Equal(testInstance, before, readReleaseArtifactTree(testInstance, fixture.artifactDirectory))
	candidatePaths, candidateGlobError := filepath.Glob(filepath.Join(filepath.Dir(fixture.artifactDirectory), "mprlab-release-candidate.*"))
	require.NoError(testInstance, candidateGlobError)
	require.Empty(testInstance, candidatePaths)
}

func TestReleaseSealingFailureRestoresSourceState(testInstance *testing.T) {
	fixture := newExactReleaseFixture(testInstance)
	before := readReleaseArtifactTree(testInstance, fixture.artifactDirectory)
	writeReleaseFixtureFile(testInstance, filepath.Join(fixture.repositoryPath, "README.md"), "next release\n")
	runGit(testInstance, fixture.repositoryPath, "add", "README.md")
	runGit(testInstance, fixture.repositoryPath, "commit", "-m", "feat: next release")
	sourceHead := strings.TrimSpace(runGit(testInstance, fixture.repositoryPath, "rev-parse", "HEAD"))
	pathVariable := buildReleaseStubbedExecutablePath(testInstance, map[string]string{"make": releaseFakeMakeScript})

	outputText, runError := runReleasePrepareReleaseScript(
		testInstance,
		fixture.repositoryRoot,
		fixture.repositoryPath,
		integrationCommandOptions{
			PathVariable: pathVariable,
			EnvironmentOverrides: map[string]string{
				"RELEASE_ARTIFACT_TARGETS":           "fixture-artifact",
				releaseFakeMakeUnsafePayloadVariable: "true",
			},
		},
	)
	require.Error(testInstance, runError, outputText)
	require.Contains(testInstance, outputText, "prepared release payloads must not contain symlinks")
	require.Contains(testInstance, outputText, "Restored master to "+sourceHead)
	require.Equal(testInstance, sourceHead, strings.TrimSpace(runGit(testInstance, fixture.repositoryPath, "rev-parse", "HEAD")))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, fixture.repositoryPath, "status", "--short")))
	require.Equal(testInstance, before, readReleaseArtifactTree(testInstance, fixture.artifactDirectory))
	tagCommand := exec.Command("git", "rev-parse", "--verify", "refs/tags/v9.8.8")
	tagCommand.Dir = fixture.repositoryPath
	tagOutput, tagError := tagCommand.CombinedOutput()
	require.Error(testInstance, tagError, string(tagOutput))
	candidatePaths, candidateGlobError := filepath.Glob(filepath.Join(filepath.Dir(fixture.artifactDirectory), "mprlab-release-candidate.*"))
	require.NoError(testInstance, candidateGlobError)
	require.Empty(testInstance, candidatePaths)
}

type exactReleaseFixture struct {
	repositoryRoot    string
	repositoryPath    string
	artifactDirectory string
	remoteDirectory   string
	version           string
	releaseCommit     string
	sourceCommit      string
	notes             string
	manifest          string
}

func newExactReleaseFixture(testInstance *testing.T) exactReleaseFixture {
	testInstance.Helper()
	repositoryRoot := releaseRepositoryRoot(testInstance)
	remoteDirectory := filepath.Join(testInstance.TempDir(), "remote.git")
	initRemoteCommand := exec.Command("git", "init", "--bare", remoteDirectory)
	initRemoteOutput, initRemoteError := initRemoteCommand.CombinedOutput()
	require.NoError(testInstance, initRemoteError, string(initRemoteOutput))
	repositoryPath := createGitRepository(testInstance, gitRepositoryOptions{InitialBranch: "master", RemoteURL: remoteDirectory})
	configureGitIdentity(testInstance, repositoryPath)
	writeReleaseFixtureFile(testInstance, filepath.Join(repositoryPath, "README.md"), "fixture\n")
	writeReleaseFixtureFile(testInstance, filepath.Join(repositoryPath, "CHANGELOG.md"), "# Changelog\n\n")
	runGit(testInstance, repositoryPath, "add", "README.md", "CHANGELOG.md")
	runGit(testInstance, repositoryPath, "commit", "-m", "feat: fixture source")
	sourceCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))
	notes := "## [" + releaseFixtureVersion + "] - 2026-08-06\n\n- feat: fixture source\n"
	writeReleaseFixtureFile(testInstance, filepath.Join(repositoryPath, "CHANGELOG.md"), "# Changelog\n\n"+notes)
	runGit(testInstance, repositoryPath, "add", "CHANGELOG.md")
	runGit(testInstance, repositoryPath, "commit", "-m", "Release "+releaseFixtureVersion)
	releaseCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))
	runGit(testInstance, repositoryPath, "tag", "-a", releaseFixtureVersion, "-m", "Release "+releaseFixtureVersion)
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")
	runGit(testInstance, repositoryPath, "push", "origin", "refs/tags/"+releaseFixtureVersion)
	runGit(testInstance, repositoryPath, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/master")

	artifactDirectory := filepath.Join(repositoryPath, ".git", "mprlab-release")
	payloadContents := []byte("published pages\n")
	payloadRelativePath := "payloads/release-assets/pages.tar.gz"
	writeReleaseFixtureFile(testInstance, filepath.Join(artifactDirectory, payloadRelativePath), string(payloadContents))
	writeReleaseFixtureFile(testInstance, filepath.Join(artifactDirectory, "notes.md"), notes)
	notesHash := fmt.Sprintf("%x", sha256.Sum256([]byte(notes)))
	payloadHash := fmt.Sprintf("%x", sha256.Sum256(payloadContents))
	manifestValue := map[string]any{
		"schema_version":    2,
		"artifact_kind":     "mprlab.release",
		"version":           releaseFixtureVersion,
		"source_commit":     sourceCommit,
		"release_commit":    releaseCommit,
		"default_branch":    "master",
		"release_timestamp": "2026-08-06T14:23:35-07:00",
		"notes_sha256":      notesHash,
		"payloads": []map[string]any{{
			"path":   payloadRelativePath,
			"size":   len(payloadContents),
			"sha256": payloadHash,
		}},
	}
	manifestBytes, manifestMarshalError := json.MarshalIndent(manifestValue, "", "  ")
	require.NoError(testInstance, manifestMarshalError)
	manifest := string(manifestBytes) + "\n"
	writeReleaseFixtureFile(testInstance, filepath.Join(artifactDirectory, "manifest.json"), manifest)

	return exactReleaseFixture{
		repositoryRoot:    repositoryRoot,
		repositoryPath:    repositoryPath,
		artifactDirectory: artifactDirectory,
		remoteDirectory:   remoteDirectory,
		version:           releaseFixtureVersion,
		releaseCommit:     releaseCommit,
		sourceCommit:      sourceCommit,
		notes:             notes,
		manifest:          manifest,
	}
}

func (fixture exactReleaseFixture) writePublishedRelease(testInstance *testing.T, manifest string) (string, string) {
	testInstance.Helper()
	publishedDirectory := testInstance.TempDir()
	writeReleaseFixtureFile(testInstance, filepath.Join(publishedDirectory, "manifest.json"), manifest)
	payloadPath := filepath.Join(fixture.artifactDirectory, "payloads", "release-assets", "pages.tar.gz")
	payloadContents, payloadReadError := os.ReadFile(payloadPath)
	require.NoError(testInstance, payloadReadError)
	writeReleaseFixtureFile(testInstance, filepath.Join(publishedDirectory, "pages.tar.gz"), string(payloadContents))
	releaseView := map[string]any{
		"tagName":         fixture.version,
		"body":            fixture.notes,
		"publishedAt":     "2026-08-06T21:26:53Z",
		"isDraft":         false,
		"isPrerelease":    false,
		"targetCommitish": "master",
		"url":             "https://example.invalid/releases/" + fixture.version,
		"assets": []map[string]any{
			{"name": "manifest.json"},
			{"name": "pages.tar.gz"},
		},
	}
	releaseViewBytes, releaseViewMarshalError := json.Marshal(releaseView)
	require.NoError(testInstance, releaseViewMarshalError)
	releaseViewPath := filepath.Join(testInstance.TempDir(), "release-view.json")
	writeReleaseFixtureFile(testInstance, releaseViewPath, string(releaseViewBytes))
	return publishedDirectory, releaseViewPath
}

func readReleaseArtifactTree(testInstance *testing.T, artifactDirectory string) map[string]string {
	testInstance.Helper()
	files := make(map[string]string)
	walkError := filepath.WalkDir(artifactDirectory, func(path string, directoryEntry os.DirEntry, walkError error) error {
		if walkError != nil {
			return walkError
		}
		if directoryEntry.IsDir() {
			return nil
		}
		relativePath, relativePathError := filepath.Rel(artifactDirectory, path)
		if relativePathError != nil {
			return relativePathError
		}
		contents, readError := os.ReadFile(path)
		if readError != nil {
			return readError
		}
		files[filepath.ToSlash(relativePath)] = string(contents)
		return nil
	})
	require.NoError(testInstance, walkError)
	return files
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

func TestPagesReleasePreservesDistinctCommitRolesAndNoJekyll(testInstance *testing.T) {
	repositoryRoot := releaseRepositoryRoot(testInstance)
	fixtureRepository := createGitRepository(testInstance, gitRepositoryOptions{InitialBranch: "master"})
	artifactDirectory := filepath.Join(testInstance.TempDir(), "artifact")
	writeReleaseFixtureFile(
		testInstance,
		filepath.Join(artifactDirectory, "staging.json"),
		releaseStagingManifestFixture,
	)
	siteDirectory := filepath.Join(testInstance.TempDir(), "site")
	writeReleaseFixtureFile(testInstance, filepath.Join(siteDirectory, "index.html"), "<!doctype html><title>Fixture</title>\n")

	preparePath := buildReleaseStubbedExecutablePath(testInstance, map[string]string{"rsync": releaseFakeRsyncScript})
	prepareOutput, prepareError := runReleasePrepareScript(
		testInstance,
		repositoryRoot,
		fixtureRepository,
		integrationCommandOptions{
			PathVariable: preparePath,
			EnvironmentOverrides: map[string]string{
				releaseArtifactDirectoryVariable: artifactDirectory,
				"RELEASE_VERSION":                releaseFixtureVersion,
			},
		},
		"--source", siteDirectory,
	)
	require.NoError(testInstance, prepareError, prepareOutput)

	archivePath := filepath.Join(artifactDirectory, "payloads", "release-assets", "pages.tar.gz")
	extractedSite := testInstance.TempDir()
	extractCommand := exec.Command("tar", "-xzf", archivePath, "-C", extractedSite)
	extractOutput, extractError := extractCommand.CombinedOutput()
	require.NoError(testInstance, extractError, string(extractOutput))
	noJekyllInfo, noJekyllError := os.Stat(filepath.Join(extractedSite, ".nojekyll"))
	require.NoError(testInstance, noJekyllError)
	require.Zero(testInstance, noJekyllInfo.Size())
	markerPath := filepath.Join(extractedSite, ".mprlab-release.json")
	markerContents, markerReadError := os.ReadFile(markerPath)
	require.NoError(testInstance, markerReadError)
	var marker map[string]any
	require.NoError(testInstance, json.Unmarshal(markerContents, &marker))
	require.Equal(testInstance, float64(1), marker["schema_version"])
	require.Equal(testInstance, releaseFixtureVersion, marker["release_version"])
	require.Equal(testInstance, releaseFixtureSourceCommit, marker["source_commit"])

	archiveContents, archiveReadError := os.ReadFile(archivePath)
	require.NoError(testInstance, archiveReadError)
	archiveHash := fmt.Sprintf("%x", sha256.Sum256(archiveContents))
	preparedManifest := strings.Replace(releasePreparedManifestFixture, releasePreparedPagesHash, archiveHash, 1)
	preparedManifestPath := filepath.Join(fixtureRepository, ".git", "mprlab-release", "manifest.json")
	writeReleaseFixtureFile(testInstance, preparedManifestPath, preparedManifest)

	remoteDirectory := filepath.Join(testInstance.TempDir(), "remote.git")
	initRemoteCommand := exec.Command("git", "init", "--bare", remoteDirectory)
	initRemoteOutput, initRemoteError := initRemoteCommand.CombinedOutput()
	require.NoError(testInstance, initRemoteError, string(initRemoteOutput))
	realGitPath, realGitError := exec.LookPath("git")
	require.NoError(testInstance, realGitError)
	publicMarkerPath := filepath.Join(testInstance.TempDir(), ".mprlab-release.json")
	writeReleaseFixtureFile(testInstance, publicMarkerPath, string(markerContents))
	ghLogPath := filepath.Join(testInstance.TempDir(), "gh-api.log")
	buildCounterPath := filepath.Join(testInstance.TempDir(), "pages-build-counter")
	deployPath := buildReleaseStubbedExecutablePath(testInstance, map[string]string{
		"curl": releaseFakeCurlScript,
		"gh":   releaseFakeGHPagesScript,
		"git":  releaseFakeGitDeployScript,
	})
	deployEnvironment := map[string]string{
		releaseFakeManifestVariable:          preparedManifestPath,
		releaseFakePagesArchiveVariable:      archivePath,
		releaseFakePagesMarkerVariable:       publicMarkerPath,
		releaseFakeGHLogVariable:             ghLogPath,
		releaseFakePagesBuildCounterVariable: buildCounterPath,
		releaseFakeReleaseCommitVariable:     releaseFixtureReleaseCommit,
		releaseFakeRemoteVariable:            remoteDirectory,
		releaseRealGitVariable:               realGitPath,
		releaseFakeVersionVariable:           releaseFixtureVersion,
		"PAGES_VERIFY_ATTEMPTS":              "1",
		"PAGES_VERIFY_DELAY_SECONDS":         "0",
	}
	deployOutput, deployError := runReleaseDeployScript(
		testInstance,
		repositoryRoot,
		fixtureRepository,
		integrationCommandOptions{PathVariable: deployPath, EnvironmentOverrides: deployEnvironment},
		"--version", releaseFixtureVersion,
		"--url", releaseFixtureURL,
	)
	require.NoError(testInstance, deployError, deployOutput)
	require.Contains(testInstance, deployOutput, "Verified "+releaseFixtureURL+" at source "+releaseFixtureSourceCommit+".")
	require.NotContains(testInstance, deployOutput, "at source "+releaseFixtureReleaseCommit+".")
	ghLogContents, ghLogReadError := os.ReadFile(ghLogPath)
	require.NoError(testInstance, ghLogReadError)
	require.Equal(
		testInstance,
		"GET repos/{owner}/{repo}/pages\nGET repos/{owner}/{repo}/pages/builds?per_page=100\n",
		string(ghLogContents),
	)

	deployedNoJekyllCommand := exec.Command(realGitPath, "--git-dir", remoteDirectory, "cat-file", "-e", "refs/heads/gh-pages:.nojekyll")
	deployedNoJekyllOutput, deployedNoJekyllError := deployedNoJekyllCommand.CombinedOutput()
	require.NoError(testInstance, deployedNoJekyllError, string(deployedNoJekyllOutput))
	deployedMarkerCommand := exec.Command(realGitPath, "--git-dir", remoteDirectory, "show", "refs/heads/gh-pages:.mprlab-release.json")
	deployedMarkerContents, deployedMarkerError := deployedMarkerCommand.Output()
	require.NoError(testInstance, deployedMarkerError)
	var deployedMarker map[string]any
	require.NoError(testInstance, json.Unmarshal(deployedMarkerContents, &deployedMarker))
	require.Equal(testInstance, releaseFixtureSourceCommit, deployedMarker["source_commit"])

	invalidMarkers := []map[string]any{
		{
			"schema_version":    float64(2),
			"release_version":   releaseFixtureVersion,
			"source_commit":     releaseFixtureSourceCommit,
			"release_timestamp": "2026-07-09T12:00:00-07:00",
		},
		{
			"schema_version":    float64(1),
			"release_version":   "v9.9.9",
			"source_commit":     releaseFixtureSourceCommit,
			"release_timestamp": "2026-07-09T12:00:00-07:00",
		},
		{
			"schema_version":    float64(1),
			"release_version":   releaseFixtureVersion,
			"source_commit":     releaseFixtureReleaseCommit,
			"release_timestamp": "2026-07-09T12:00:00-07:00",
		},
	}
	for invalidMarkerIndex, invalidMarker := range invalidMarkers {
		testInstance.Run(fmt.Sprintf("invalid-public-marker-%d", invalidMarkerIndex), func(t *testing.T) {
			invalidMarkerContents, marshalError := json.Marshal(invalidMarker)
			require.NoError(t, marshalError)
			writeReleaseFixtureFile(t, publicMarkerPath, string(invalidMarkerContents))
			outputText, runError := runReleaseDeployScript(
				t,
				repositoryRoot,
				fixtureRepository,
				integrationCommandOptions{PathVariable: deployPath, EnvironmentOverrides: deployEnvironment},
				"--version", releaseFixtureVersion,
				"--url", releaseFixtureURL,
				"--skip-configure",
			)
			require.Error(t, runError, outputText)
			require.Contains(t, outputText, "source "+releaseFixtureSourceCommit)
		})
	}
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

func TestPagesDeployReconcilesExistingBuildState(testInstance *testing.T) {
	testCases := []struct {
		name             string
		buildStates      string
		publicMarker     string
		expectError      bool
		expectBuildPosts int
		expectedOutput   string
	}{
		{
			name:             "completed-build-is-accepted",
			buildStates:      "built",
			publicMarker:     releasePagesMarkerFixture,
			expectBuildPosts: 0,
			expectedOutput:   "Verified " + releaseFixtureURL,
		},
		{
			name:             "queued-build-completes",
			buildStates:      "queued,built",
			publicMarker:     releasePagesMarkerFixture,
			expectBuildPosts: 0,
			expectedOutput:   "Reusing active GitHub Pages build",
		},
		{
			name:             "active-build-completes",
			buildStates:      "building,built",
			publicMarker:     releasePagesMarkerFixture,
			expectBuildPosts: 0,
			expectedOutput:   "Reusing active GitHub Pages build",
		},
		{
			name:             "missing-build-retry-completes",
			buildStates:      "missing,built",
			publicMarker:     releasePagesMarkerFixture,
			expectBuildPosts: 1,
			expectedOutput:   "Requested one GitHub Pages rebuild",
		},
		{
			name:             "terminal-build-retry-fails",
			buildStates:      "errored,errored",
			publicMarker:     `{}`,
			expectError:      true,
			expectBuildPosts: 1,
			expectedOutput:   "GitHub Pages build failed for commit",
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(t *testing.T) {
			fixture := newReleasePagesDeployFixture(t)
			fixture.seedRemote(t)
			writeReleaseFixtureFile(t, fixture.publicMarkerPath, testCase.publicMarker)

			outputText, runError := fixture.deploy(
				t,
				releaseFakeGHPagesScript,
				map[string]string{
					releaseFakePagesBuildStatesVariable: testCase.buildStates,
					"PAGES_VERIFY_ATTEMPTS":             "2",
					"PAGES_VERIFY_DELAY_SECONDS":        "0",
				},
				"--version", releaseFixtureVersion,
				"--url", releaseFixtureURL,
			)
			if testCase.expectError {
				require.Error(t, runError, outputText)
			} else {
				require.NoError(t, runError, outputText)
			}
			require.Contains(t, outputText, testCase.expectedOutput)

			ghLogContents, ghLogReadError := os.ReadFile(fixture.ghLogPath)
			require.NoError(t, ghLogReadError)
			ghLog := string(ghLogContents)
			require.NotContains(t, ghLog, "PUT repos/{owner}/{repo}/pages")
			require.Equal(t, testCase.expectBuildPosts, strings.Count(ghLog, "POST repos/{owner}/{repo}/pages/builds"))
		})
	}
}

func TestPagesDeployReconcilesLegacyConfigurationWithoutDuplicateBuildRequest(testInstance *testing.T) {
	testCases := []struct {
		name               string
		configurationState string
		expectError        bool
		expectedMutation   string
		expectedOutput     string
	}{
		{
			name:               "missing-configuration",
			configurationState: "missing",
			expectedMutation:   "POST repos/{owner}/{repo}/pages",
			expectedOutput:     "Created GitHub Pages legacy source",
		},
		{
			name:               "drifted-configuration",
			configurationState: "drift",
			expectedMutation:   "PUT repos/{owner}/{repo}/pages",
			expectedOutput:     "Updated GitHub Pages legacy source",
		},
		{
			name:               "configuration-api-unavailable",
			configurationState: "unavailable",
			expectError:        true,
			expectedOutput:     "failed to inspect GitHub Pages configuration",
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(t *testing.T) {
			fixture := newReleasePagesDeployFixture(t)
			writeReleaseFixtureFile(t, fixture.publicMarkerPath, releasePagesMarkerFixture)
			outputText, runError := fixture.deploy(
				t,
				releaseFakeGHPagesScript,
				map[string]string{
					releaseFakePagesConfigStateVariable: testCase.configurationState,
					"PAGES_VERIFY_ATTEMPTS":             "1",
					"PAGES_VERIFY_DELAY_SECONDS":        "0",
				},
				"--version", releaseFixtureVersion,
				"--url", releaseFixtureURL,
			)
			if testCase.expectError {
				require.Error(t, runError, outputText)
			} else {
				require.NoError(t, runError, outputText)
			}
			require.Contains(t, outputText, testCase.expectedOutput)

			ghLogContents, ghLogReadError := os.ReadFile(fixture.ghLogPath)
			require.NoError(t, ghLogReadError)
			ghLog := string(ghLogContents)
			if testCase.expectedMutation == "" {
				require.NotContains(t, ghLog, "PUT repos/{owner}/{repo}/pages")
				require.NotContains(t, ghLog, "POST repos/{owner}/{repo}/pages\n")
			} else {
				require.Equal(t, 1, strings.Count(ghLog, testCase.expectedMutation))
			}
			require.NotContains(t, ghLog, "POST repos/{owner}/{repo}/pages/builds")
		})
	}
}

type releasePagesDeployFixture struct {
	repositoryRoot       string
	fixtureRepository    string
	preparedManifestPath string
	archivePath          string
	publicMarkerPath     string
	remoteDirectory      string
	realGitPath          string
	ghLogPath            string
	buildCounterPath     string
}

func newReleasePagesDeployFixture(testInstance *testing.T) releasePagesDeployFixture {
	testInstance.Helper()
	repositoryRoot := releaseRepositoryRoot(testInstance)
	fixtureRepository := createGitRepository(testInstance, gitRepositoryOptions{InitialBranch: "master"})
	artifactDirectory := filepath.Join(testInstance.TempDir(), "artifact")
	writeReleaseFixtureFile(testInstance, filepath.Join(artifactDirectory, "staging.json"), releaseStagingManifestFixture)
	siteDirectory := filepath.Join(testInstance.TempDir(), "site")
	writeReleaseFixtureFile(testInstance, filepath.Join(siteDirectory, "index.html"), "<!doctype html><title>Fixture</title>\n")

	preparePath := buildReleaseStubbedExecutablePath(testInstance, map[string]string{"rsync": releaseFakeRsyncScript})
	prepareOutput, prepareError := runReleasePrepareScript(
		testInstance,
		repositoryRoot,
		fixtureRepository,
		integrationCommandOptions{
			PathVariable: preparePath,
			EnvironmentOverrides: map[string]string{
				releaseArtifactDirectoryVariable: artifactDirectory,
				"RELEASE_VERSION":                releaseFixtureVersion,
			},
		},
		"--source", siteDirectory,
	)
	require.NoError(testInstance, prepareError, prepareOutput)

	archivePath := filepath.Join(artifactDirectory, "payloads", "release-assets", "pages.tar.gz")
	archiveContents, archiveReadError := os.ReadFile(archivePath)
	require.NoError(testInstance, archiveReadError)
	archiveHash := fmt.Sprintf("%x", sha256.Sum256(archiveContents))
	preparedManifest := strings.Replace(releasePreparedManifestFixture, releasePreparedPagesHash, archiveHash, 1)
	preparedManifestPath := filepath.Join(fixtureRepository, ".git", "mprlab-release", "manifest.json")
	writeReleaseFixtureFile(testInstance, preparedManifestPath, preparedManifest)

	remoteDirectory := filepath.Join(testInstance.TempDir(), "remote.git")
	initRemoteCommand := exec.Command("git", "init", "--bare", remoteDirectory)
	initRemoteOutput, initRemoteError := initRemoteCommand.CombinedOutput()
	require.NoError(testInstance, initRemoteError, string(initRemoteOutput))
	realGitPath, realGitError := exec.LookPath("git")
	require.NoError(testInstance, realGitError)

	return releasePagesDeployFixture{
		repositoryRoot:       repositoryRoot,
		fixtureRepository:    fixtureRepository,
		preparedManifestPath: preparedManifestPath,
		archivePath:          archivePath,
		publicMarkerPath:     filepath.Join(testInstance.TempDir(), ".mprlab-release.json"),
		remoteDirectory:      remoteDirectory,
		realGitPath:          realGitPath,
		ghLogPath:            filepath.Join(testInstance.TempDir(), "gh-api.log"),
		buildCounterPath:     filepath.Join(testInstance.TempDir(), "pages-build-counter"),
	}
}

func (fixture releasePagesDeployFixture) seedRemote(testInstance *testing.T) {
	testInstance.Helper()
	outputText, runError := fixture.deploy(
		testInstance,
		releaseFakeGHDownloadScript,
		nil,
		"--version", releaseFixtureVersion,
		"--skip-configure",
		"--skip-verify",
	)
	require.NoError(testInstance, runError, outputText)
}

func (fixture releasePagesDeployFixture) deploy(
	testInstance *testing.T,
	ghScript string,
	environmentOverrides map[string]string,
	arguments ...string,
) (string, error) {
	testInstance.Helper()
	deployPath := buildReleaseStubbedExecutablePath(testInstance, map[string]string{
		"curl": releaseFakeCurlScript,
		"gh":   ghScript,
		"git":  releaseFakeGitDeployScript,
	})
	environment := map[string]string{
		releaseFakeManifestVariable:          fixture.preparedManifestPath,
		releaseFakePagesArchiveVariable:      fixture.archivePath,
		releaseFakePagesMarkerVariable:       fixture.publicMarkerPath,
		releaseFakeGHLogVariable:             fixture.ghLogPath,
		releaseFakePagesBuildCounterVariable: fixture.buildCounterPath,
		releaseFakeReleaseCommitVariable:     releaseFixtureReleaseCommit,
		releaseFakeRemoteVariable:            fixture.remoteDirectory,
		releaseRealGitVariable:               fixture.realGitPath,
		releaseFakeVersionVariable:           releaseFixtureVersion,
	}
	for key, value := range environmentOverrides {
		environment[key] = value
	}
	return runReleaseDeployScript(
		testInstance,
		fixture.repositoryRoot,
		fixture.fixtureRepository,
		integrationCommandOptions{PathVariable: deployPath, EnvironmentOverrides: environment},
		arguments...,
	)
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
	command := exec.CommandContext(executionContext, "bash", commandArguments...)
	command.Dir = workingDirectory
	command.Env = buildCommandEnvironment(commandOptions)
	outputBytes, runError := command.CombinedOutput()
	return string(outputBytes), runError
}

func runReleasePrepareScript(
	testInstance *testing.T,
	repositoryRoot string,
	workingDirectory string,
	commandOptions integrationCommandOptions,
	arguments ...string,
) (string, error) {
	testInstance.Helper()
	executionContext, cancelFunction := context.WithTimeout(context.Background(), releaseMakeCommandTimeout)
	defer cancelFunction()

	scriptPath := filepath.Join(repositoryRoot, releaseToolDirectoryRelativePath, "prepare_pages_artifact.sh")
	commandArguments := append([]string{scriptPath}, arguments...)
	command := exec.CommandContext(executionContext, "bash", commandArguments...)
	command.Dir = workingDirectory
	command.Env = buildCommandEnvironment(commandOptions)
	outputBytes, runError := command.CombinedOutput()
	return string(outputBytes), runError
}

func runReleasePrepareReleaseScript(
	testInstance *testing.T,
	repositoryRoot string,
	workingDirectory string,
	commandOptions integrationCommandOptions,
	arguments ...string,
) (string, error) {
	testInstance.Helper()
	executionContext, cancelFunction := context.WithTimeout(context.Background(), releaseMakeCommandTimeout)
	defer cancelFunction()

	scriptPath := filepath.Join(repositoryRoot, releaseToolDirectoryRelativePath, "prepare_release.sh")
	commandArguments := append([]string{scriptPath}, arguments...)
	command := exec.CommandContext(executionContext, "bash", commandArguments...)
	command.Dir = workingDirectory
	command.Env = buildCommandEnvironment(commandOptions)
	outputBytes, runError := command.CombinedOutput()
	return string(outputBytes), runError
}

func buildReleaseStubbedExecutablePath(testInstance *testing.T, scripts map[string]string) string {
	testInstance.Helper()
	stubDirectory := testInstance.TempDir()
	for executableName, scriptContents := range scripts {
		stubPath := filepath.Join(stubDirectory, executableName)
		require.NoError(testInstance, os.WriteFile(stubPath, []byte(scriptContents), 0o755))
	}
	return stubDirectory + string(os.PathListSeparator) + os.Getenv("PATH")
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

const releaseFakeMakeScript = `#!/usr/bin/env bash
set -euo pipefail

target="${1:-}"
if [[ "${target}" == "--no-print-directory" ]]; then
  target="${2:-}"
fi
if [[ "${target}" == "${GIX_RELEASE_FAKE_MAKE_FAILURE_TARGET:-}" ]]; then
  if [[ -n "${GIX_RELEASE_FAKE_MAKE_LOG:-}" ]]; then
    printf '%s\n' "${target}" >>"${GIX_RELEASE_FAKE_MAKE_LOG}"
  fi
  echo "fixture artifact failure: ${target}" >&2
  exit 42
fi
if [[ -n "${GIX_RELEASE_FAKE_MAKE_LOG:-}" ]]; then
  printf '%s\n' "${target}" >>"${GIX_RELEASE_FAKE_MAKE_LOG}"
fi
if [[ "${target}" != "ci" ]]; then
  mkdir -p "${RELEASE_ARTIFACT_DIR}/payloads/release-assets"
  printf 'candidate payload\n' >"${RELEASE_ARTIFACT_DIR}/payloads/release-assets/candidate.txt"
  if [[ "${GIX_RELEASE_FAKE_MAKE_UNSAFE_PAYLOAD:-}" == "true" ]]; then
    ln -s candidate.txt "${RELEASE_ARTIFACT_DIR}/payloads/release-assets/unsafe-link"
  fi
fi
`

const releaseFakeGHRecoveryScript = `#!/usr/bin/env bash
set -euo pipefail

if [[ -n "${GIX_RELEASE_FAKE_GH_LOG:-}" ]]; then
  printf '%s\n' "$*" >>"${GIX_RELEASE_FAKE_GH_LOG}"
fi
[[ "${1:-}" == "release" ]] || { echo "unexpected gh invocation: $*" >&2; exit 41; }
case "${2:-}" in
  view)
    cat "${GIX_RELEASE_FAKE_RELEASE_VIEW}"
    ;;
  download)
    shift 2
    pattern=""
    download_directory=""
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --pattern) pattern="$2"; shift 2 ;;
        --dir) download_directory="$2"; shift 2 ;;
        *) shift ;;
      esac
    done
    [[ -n "${pattern}" && -n "${download_directory}" ]] || { echo "missing release download arguments" >&2; exit 42; }
    cp "${GIX_RELEASE_FAKE_PUBLISHED_DIRECTORY}/${pattern}" "${download_directory}/${pattern}"
    ;;
  *)
    echo "unexpected gh release invocation: $*" >&2
    exit 43
    ;;
esac
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
	releaseFakeGHPagesScript = `#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" == "release" && "${2:-}" == "download" ]]; then
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
  exit 0
fi

[[ "${1:-}" == "api" ]] || { echo "unexpected gh invocation: $*" >&2; exit 41; }
shift
method="GET"
endpoint=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --method) method="$2"; shift 2 ;;
    -f|-F) shift 2 ;;
    -*) shift ;;
    *)
      if [[ -z "${endpoint}" ]]; then endpoint="$1"; fi
      shift
      ;;
  esac
done
printf '%s %s\n' "${method}" "${endpoint}" >>"${GIX_RELEASE_FAKE_GH_LOG}"

case "${method} ${endpoint}" in
  "GET repos/{owner}/{repo}/pages")
    case "${GIX_RELEASE_FAKE_PAGES_CONFIG_STATE:-current}" in
      current)
        printf '%s\n' '{"build_type":"legacy","https_enforced":true,"source":{"branch":"gh-pages","path":"/"}}'
        ;;
      drift)
        printf '%s\n' '{"build_type":"workflow","https_enforced":false,"source":{"branch":"master","path":"/docs"}}'
        ;;
      missing)
        echo 'gh: Not Found (HTTP 404)' >&2
        exit 1
        ;;
      unavailable)
        echo 'gh: Service Unavailable (HTTP 503)' >&2
        exit 1
        ;;
      *)
        echo "unexpected Pages configuration fixture state" >&2
        exit 44
        ;;
    esac
    ;;
  "GET repos/{owner}/{repo}/pages/builds?per_page=100")
    pages_commit="$("${GIX_RELEASE_REAL_GIT}" --git-dir "${GIX_RELEASE_FAKE_REMOTE}" rev-parse refs/heads/gh-pages)"
    build_counter=0
    if [[ -f "${GIX_RELEASE_FAKE_PAGES_BUILD_COUNTER}" ]]; then
      build_counter="$(<"${GIX_RELEASE_FAKE_PAGES_BUILD_COUNTER}")"
    fi
    build_counter=$((build_counter + 1))
    printf '%s\n' "${build_counter}" >"${GIX_RELEASE_FAKE_PAGES_BUILD_COUNTER}"
    IFS=',' read -r -a build_states <<<"${GIX_RELEASE_FAKE_PAGES_BUILD_STATES:-built}"
    build_state_index=$((build_counter - 1))
    if (( build_state_index >= ${#build_states[@]} )); then
      build_state_index=$((${#build_states[@]} - 1))
    fi
    build_state="${build_states[${build_state_index}]}"
    if [[ "${build_state}" == "missing" ]]; then
      printf '%s\n' '[]'
    elif [[ "${build_state}" == "errored" ]]; then
      printf '[{"commit":"%s","status":"errored","error":{"message":"fixture Pages failure"},"url":"https://api.example.invalid/pages/builds/%s"}]\n' "${pages_commit}" "${build_counter}"
    else
      printf '[{"commit":"%s","status":"%s","url":"https://api.example.invalid/pages/builds/%s"}]\n' "${pages_commit}" "${build_state}" "${build_counter}"
    fi
    ;;
  "PUT repos/{owner}/{repo}/pages"|"POST repos/{owner}/{repo}/pages"|"POST repos/{owner}/{repo}/pages/builds")
    printf '%s\n' '{}'
    ;;
  *)
    echo "unexpected gh api invocation: ${method} ${endpoint}" >&2
    exit 43
    ;;
esac
`
	releaseFakeRsyncScript = `#!/bin/sh
set -eu
while [ "$#" -gt 2 ]; do shift; done
cp -R "$1"/. "$2"/
`
	releaseFakeCurlScript = `#!/bin/sh
set -eu
cat "${GIX_RELEASE_FAKE_PAGES_MARKER}"
`
	releaseFakeGitDeployScript = `#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" == "ls-remote" ]]; then
  printf '%s\trefs/tags/%s^{}\n' "${GIX_RELEASE_FAKE_RELEASE_COMMIT}" "${GIX_RELEASE_FIXTURE_VERSION}"
  exit 0
fi
if [[ "${1:-}" == "remote" && "${2:-}" == "get-url" ]]; then
  printf '%s\n' "${GIX_RELEASE_FAKE_REMOTE}"
  exit 0
fi
exec "${GIX_RELEASE_REAL_GIT}" "$@"
`
	releaseStagingManifestFixture = `{
  "release_timestamp": "2026-07-09T12:00:00-07:00",
  "source_commit": "2222222222222222222222222222222222222222",
  "version": "v9.8.7"
}
`
	releasePagesMarkerFixture = `{
  "release_timestamp": "2026-07-09T12:00:00-07:00",
  "release_version": "v9.8.7",
  "schema_version": 1,
  "source_commit": "2222222222222222222222222222222222222222"
}
`
)
