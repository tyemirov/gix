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

func TestParseConfigurationAcceptsRootGoInstallContract(t *testing.T) {
	configuration, parseError := ParseConfiguration([]byte("schema_version: 1\nscheme: semver\ngo_install:\n  module_path: github.com/tyemirov/gix\n  product_version_file: internal/version/product-version.txt\n"))

	require.NoError(t, parseError)
	require.Equal(t, "github.com/tyemirov/gix", configuration.GoInstall.ModulePath)
	require.Equal(t, "internal/version/product-version.txt", configuration.GoInstall.ProductVersionFile)
}

func TestParseConfigurationRejectsPartialOrUnsafeRootGoInstallContract(t *testing.T) {
	for _, configuration := range []string{
		"schema_version: 1\nscheme: semver\ngo_install:\n  module_path: github.com/tyemirov/gix\n",
		"schema_version: 1\nscheme: semver\ngo_install:\n  module_path: github.com/tyemirov/gix\n  product_version_file: ../VERSION\n",
	} {
		_, parseError := ParseConfiguration([]byte(configuration))
		require.Error(t, parseError)
	}
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

func TestLatestSemVerIgnoresNonCanonicalTags(t *testing.T) {
	require.Equal(t, "v10.0.0", LatestSemVer([]string{"1.2.3", "v2.8.9", "v10.0.0", "v11.0.0-rc.1"}))
}

func TestNextGoInstallVersionAdvancesOnlyTheRootTransportLine(t *testing.T) {
	next, nextError := NextGoInstallVersion([]string{"v1.1.24", "v6.0.0", "v1.1.25", "26.809.120000"})

	require.NoError(t, nextError)
	require.Equal(t, "v1.1.26", next)
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
