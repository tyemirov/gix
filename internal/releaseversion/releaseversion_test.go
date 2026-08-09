package releaseversion

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseConfigurationRequiresCanonicalContract(t *testing.T) {
	configuration, parseError := ParseConfiguration([]byte("schema_version: 1\nscheme: semver\n"))
	require.NoError(t, parseError)
	require.Equal(t, SchemeSemVer, configuration.Scheme)

	_, unknownError := ParseConfiguration([]byte("schema_version: 1\nscheme: semver\nlegacy: true\n"))
	require.Error(t, unknownError)
	_, schemeError := ParseConfiguration([]byte("schema_version: 1\nscheme: automatic\n"))
	require.Error(t, schemeError)
}

func TestParseConfigurationAcceptsFixedMajorSemVer(t *testing.T) {
	configuration, parseError := ParseConfiguration([]byte("schema_version: 1\nscheme: semver\nsemver:\n  fixed_major: 1\n"))
	require.NoError(t, parseError)
	require.Equal(t, 1, configuration.SemVer.FixedMajor)

	_, goInstallError := ParseConfiguration([]byte("schema_version: 1\nscheme: semver\ngo_install:\n  module_path: github.com/tyemirov/gix\n"))
	require.Error(t, goInstallError)
	_, calVerPolicyError := ParseConfiguration([]byte("schema_version: 1\nscheme: calver\nsemver:\n  fixed_major: 1\n"))
	require.Error(t, calVerPolicyError)
}

func TestNextSemVerUsesArbitraryPrecisionCanonicalMath(t *testing.T) {
	next, nextError := NextSemVer("v999999999999999999999.4.8", BumpMajor)
	require.NoError(t, nextError)
	require.Equal(t, "v1000000000000000000000.0.0", next)

	minor, minorError := NextSemVer("v2.9.7", BumpMinor)
	require.NoError(t, minorError)
	require.Equal(t, "v2.10.0", minor)

	patch, patchError := NextSemVer("v2.9.7", BumpPatch)
	require.NoError(t, patchError)
	require.Equal(t, "v2.9.8", patch)
}

func TestFixedMajorSemVerUsesOnlyConfiguredMajor(t *testing.T) {
	tags := []string{"1.2.3", "v0.9.9", "v1.2.3", "v1.9.8", "v2.8.9", "v7.0.0", "v1.10.0-rc.1"}
	require.Equal(t, "v7.0.0", LatestSemVer(tags))
	require.Equal(t, "v1.9.8", LatestFixedMajorSemVer(tags, 1))

	next, nextError := NextFixedMajorSemVer("v1.9.8", BumpMinor, 1)
	require.NoError(t, nextError)
	require.Equal(t, "v1.10.0", next)
	_, majorError := NextFixedMajorSemVer("v1.9.8", BumpMajor, 1)
	require.Error(t, majorError)
	_, wrongLineError := NextFixedMajorSemVer("v2.9.8", BumpMinor, 1)
	require.Error(t, wrongLineError)
}

func TestNextCalVerUsesCanonicalUTCSecond(t *testing.T) {
	releaseTime := time.Date(2026, time.August, 9, 3, 4, 5, 0, time.FixedZone("PDT", -7*60*60))
	next, nextError := NextCalVer("26.809.100404", releaseTime)
	require.NoError(t, nextError)
	require.Equal(t, "26.809.100405", next)
	require.Equal(t, "26.809.100405", LatestCalVer([]string{"v1.2.3", "26.808.235959", next}))
}

func TestNextCalVerRejectsNonIncreasingTimestamp(t *testing.T) {
	_, nextError := NextCalVer("26.809.100405", time.Date(2026, time.August, 9, 10, 4, 5, 0, time.UTC))
	require.Error(t, nextError)
	require.Contains(t, nextError.Error(), "must be later")
	require.Empty(t, LatestCalVer([]string{"26.809.999999999999999999999999"}))
}
