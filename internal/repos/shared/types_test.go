package shared_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/repos/shared"
)

func TestNewRepositoryPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{name: "valid_path", input: "/tmp/repo", expected: "/tmp/repo"},
		{name: "strips_whitespace", input: "   /tmp/repo  ", expected: "/tmp/repo"},
		{name: "rejects_empty", input: "", expectError: true},
		{name: "rejects_newline", input: "/tmp/repo\n", expectError: true},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result, err := shared.NewRepositoryPath(testCase.input)
			if testCase.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testCase.expected, result.String())
		})
	}
}

func TestNewOwnerSlug(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		input       string
		expect      string
		expectError bool
	}{
		{name: "valid_owner", input: "Temirov", expect: "Temirov"},
		{name: "trims_owner", input: "  org-name ", expect: "org-name"},
		{name: "rejects_empty", input: "  ", expectError: true},
		{name: "rejects_slash", input: "owner/name", expectError: true},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result, err := shared.NewOwnerSlug(testCase.input)
			if testCase.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testCase.expect, result.String())
		})
	}
}

func TestNewRepositoryName(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		input       string
		expect      string
		expectError bool
	}{
		{name: "valid_name", input: "gix", expect: "gix"},
		{name: "trims_name", input: " gix-cli ", expect: "gix-cli"},
		{name: "rejects_empty", input: "", expectError: true},
		{name: "rejects_slash", input: "owner/repo", expectError: true},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result, err := shared.NewRepositoryName(testCase.input)
			if testCase.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testCase.expect, result.String())
		})
	}
}

func TestNewOwnerRepository(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		input         string
		expectedOwner string
		expectedRepo  string
		expectError   bool
	}{
		{name: "valid_owner_repo", input: "owner/repo", expectedOwner: "owner", expectedRepo: "repo"},
		{name: "rejects_missing_separator", input: "invalid", expectError: true},
		{name: "rejects_invalid_owner", input: "bad owner/repo", expectError: true},
		{name: "rejects_invalid_repository", input: "owner/bad repo", expectError: true},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(testingInstance *testing.T) {
			testingInstance.Parallel()

			result, err := shared.NewOwnerRepository(testCase.input)
			if testCase.expectError {
				require.Error(testingInstance, err)
				require.ErrorIs(testingInstance, err, shared.ErrOwnerRepositoryInvalid)
				return
			}

			require.NoError(testingInstance, err)
			require.Equal(testingInstance, testCase.expectedOwner, result.Owner().String())
			require.Equal(testingInstance, testCase.expectedRepo, result.Repository().String())
		})
	}
}

func TestNewRemoteURL(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{name: "valid_url", input: "https://github.com/owner/repo.git", expected: "https://github.com/owner/repo.git"},
		{name: "rejects_empty", input: "  ", expectError: true},
		{name: "rejects_whitespace", input: "https://github.com/owner repo.git", expectError: true},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(testingInstance *testing.T) {
			testingInstance.Parallel()

			result, err := shared.NewRemoteURL(testCase.input)
			if testCase.expectError {
				require.Error(testingInstance, err)
				require.ErrorIs(testingInstance, err, shared.ErrRemoteURLInvalid)
				return
			}

			require.NoError(testingInstance, err)
			require.Equal(testingInstance, testCase.expected, result.String())
		})
	}
}

func TestNewRemoteName(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{name: "valid_name", input: "origin", expected: "origin"},
		{name: "rejects_whitespace", input: "invalid name", expectError: true},
		{name: "rejects_empty", input: " ", expectError: true},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(testingInstance *testing.T) {
			testingInstance.Parallel()

			result, err := shared.NewRemoteName(testCase.input)
			if testCase.expectError {
				require.Error(testingInstance, err)
				require.ErrorIs(testingInstance, err, shared.ErrRemoteNameInvalid)
				return
			}

			require.NoError(testingInstance, err)
			require.Equal(testingInstance, testCase.expected, result.String())
		})
	}
}

func TestNewBranchName(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{name: "valid_branch", input: "feature/new-ui", expected: "feature/new-ui"},
		{name: "rejects_whitespace", input: "with space", expectError: true},
		{name: "rejects_empty", input: "", expectError: true},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(testingInstance *testing.T) {
			testingInstance.Parallel()

			result, err := shared.NewBranchName(testCase.input)
			if testCase.expectError {
				require.Error(testingInstance, err)
				require.ErrorIs(testingInstance, err, shared.ErrBranchNameInvalid)
				return
			}

			require.NoError(testingInstance, err)
			require.Equal(testingInstance, testCase.expected, result.String())
		})
	}
}

func TestParseRemoteProtocol(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		input       string
		expect      shared.RemoteProtocol
		expectError bool
	}{
		{name: "git_protocol", input: "git", expect: shared.RemoteProtocolGit},
		{name: "ssh_protocol", input: "SSH", expect: shared.RemoteProtocolSSH},
		{name: "https_protocol", input: " https ", expect: shared.RemoteProtocolHTTPS},
		{name: "empty_defaults_other", input: " ", expect: shared.RemoteProtocolOther},
		{name: "unknown_error", input: "svn", expectError: true},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result, err := shared.ParseRemoteProtocol(testCase.input)
			if testCase.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testCase.expect, result)
		})
	}

	require.NoError(t, shared.RemoteProtocolSSH.Validate())
	require.Error(t, shared.RemoteProtocol("invalid").Validate())
}

func TestNewOwnerRepositoryFromParts(t *testing.T) {
	t.Parallel()

	owner, ownerErr := shared.NewOwnerSlug("owner")
	require.NoError(t, ownerErr)

	repository, repositoryErr := shared.NewRepositoryName("repo")
	require.NoError(t, repositoryErr)

	result := shared.NewOwnerRepositoryFromParts(owner, repository)
	require.Equal(t, "owner/repo", result.String())
}

func TestParseOwnerRepositoryOptional(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		input       string
		expected    string
		expectNil   bool
		expectError bool
	}{
		{name: "empty_returns_nil", input: "   ", expectNil: true},
		{name: "valid_owner_repo", input: "owner/repo", expected: "owner/repo"},
		{name: "invalid_owner_repo", input: "invalid", expectError: true},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(testingInstance *testing.T) {
			testingInstance.Parallel()

			result, err := shared.ParseOwnerRepositoryOptional(testCase.input)
			if testCase.expectError {
				require.Error(testingInstance, err)
				require.ErrorIs(testingInstance, err, shared.ErrOwnerRepositoryInvalid)
				return
			}

			require.NoError(testingInstance, err)

			if testCase.expectNil {
				require.Nil(testingInstance, result)
				return
			}

			require.NotNil(testingInstance, result)
			require.Equal(testingInstance, testCase.expected, result.String())
		})
	}
}

func TestParseOwnerSlugOptional(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		input       string
		expected    string
		expectNil   bool
		expectError bool
	}{
		{name: "empty_returns_nil", input: "", expectNil: true},
		{name: "valid_slug", input: "owner", expected: "owner"},
		{name: "invalid_slug", input: "owner/repo", expectError: true},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(testingInstance *testing.T) {
			testingInstance.Parallel()

			result, err := shared.ParseOwnerSlugOptional(testCase.input)
			if testCase.expectError {
				require.Error(testingInstance, err)
				require.ErrorIs(testingInstance, err, shared.ErrOwnerSlugInvalid)
				return
			}

			require.NoError(testingInstance, err)

			if testCase.expectNil {
				require.Nil(testingInstance, result)
				return
			}

			require.NotNil(testingInstance, result)
			require.Equal(testingInstance, testCase.expected, result.String())
		})
	}
}

func TestParseRemoteURLOptional(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		input       string
		expected    string
		expectNil   bool
		expectError bool
	}{
		{name: "empty_returns_nil", input: "", expectNil: true},
		{name: "valid_url", input: "https://github.com/owner/repo.git", expected: "https://github.com/owner/repo.git"},
		{name: "invalid_url_contains_whitespace", input: "https://github.com/owner repo.git", expectError: true},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(testingInstance *testing.T) {
			testingInstance.Parallel()

			result, err := shared.ParseRemoteURLOptional(testCase.input)
			if testCase.expectError {
				require.Error(testingInstance, err)
				require.ErrorIs(testingInstance, err, shared.ErrRemoteURLInvalid)
				return
			}

			require.NoError(testingInstance, err)

			if testCase.expectNil {
				require.Nil(testingInstance, result)
				return
			}

			require.NotNil(testingInstance, result)
			require.Equal(testingInstance, testCase.expected, result.String())
		})
	}
}
