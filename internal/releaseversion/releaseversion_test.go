package releaseversion

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPolicyRequiresCanonicalInvocationValues(t *testing.T) {
	standard, standardError := NewPolicy("semver", 0)
	require.NoError(t, standardError)
	require.Equal(t, SchemeSemVer, standard.Scheme())
	require.Zero(t, standard.FixedMajor())

	fixed, fixedError := NewPolicy("semver", 1)
	require.NoError(t, fixedError)
	require.Equal(t, 1, fixed.FixedMajor())
	fixedJSON, marshalError := fixed.MarshalJSON()
	require.NoError(t, marshalError)
	require.JSONEq(t, `{"scheme":"semver","fixed_major":1}`, string(fixedJSON))

	_, schemeError := NewPolicy("automatic", 0)
	require.Error(t, schemeError)
	_, calVerPolicyError := NewPolicy("calver", 1)
	require.Error(t, calVerPolicyError)
	_, fixedMajorError := NewPolicy("semver", -1)
	require.Error(t, fixedMajorError)
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
