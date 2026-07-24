package utils_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/utils"
)

type configurationFixture struct {
	Common configurationCommonFixture `mapstructure:"common"`
	LLM    configurationLLMFixture    `mapstructure:"llm"`
}

type configurationCommonFixture struct {
	LogLevel string `mapstructure:"log_level"`
}

type configurationLLMFixture struct {
	Credential string `mapstructure:"credential"`
}

func TestConfigurationLoaderUsesProcessEnvironmentAndIgnoresSiblingDotEnv(t *testing.T) {
	configurationDirectory := t.TempDir()
	configurationPath := filepath.Join(configurationDirectory, "config.yml")
	require.NoError(t, os.WriteFile(configurationPath, []byte(
		"common:\n  log_level: \"${CONFIG_TEST_LEVEL}\"\nllm:\n  credential: \"${CONFIG_TEST_CREDENTIAL}\"\n",
	), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(configurationDirectory, ".env"), []byte(
		"CONFIG_TEST_LEVEL=error\nCONFIG_TEST_CREDENTIAL=dotenv-secret\n",
	), 0o600))
	t.Setenv("CONFIG_TEST_LEVEL", "debug")

	var loadedConfiguration configurationFixture
	metadata, loadError := utils.NewConfigurationLoader().LoadConfiguration(configurationPath, &loadedConfiguration)

	require.NoError(t, loadError)
	require.Equal(t, configurationPath, metadata.ConfigFileUsed)
	require.Equal(t, "debug", loadedConfiguration.Common.LogLevel)
	require.Empty(t, loadedConfiguration.LLM.Credential)
}

func TestConfigurationLoaderPreservesLiteralCredential(t *testing.T) {
	configurationPath := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(configurationPath, []byte(
		"common:\n  log_level: error\nllm:\n  credential: literal-secret\n",
	), 0o600))

	var loadedConfiguration configurationFixture
	_, loadError := utils.NewConfigurationLoader().LoadConfiguration(configurationPath, &loadedConfiguration)

	require.NoError(t, loadError)
	require.Equal(t, "literal-secret", loadedConfiguration.LLM.Credential)
}

func TestConfigurationLoaderAllowsMissingCredentialPlaceholder(t *testing.T) {
	configurationPath := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(configurationPath, []byte(
		"common:\n  log_level: error\nllm:\n  credential: \"${CONFIG_TEST_OPTIONAL_CREDENTIAL}\"\n",
	), 0o600))

	var loadedConfiguration configurationFixture
	_, loadError := utils.NewConfigurationLoader().LoadConfiguration(configurationPath, &loadedConfiguration)

	require.NoError(t, loadError)
	require.Empty(t, loadedConfiguration.LLM.Credential)
}

func TestConfigurationLoaderRejectsMissingRequiredPlaceholder(t *testing.T) {
	configurationPath := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(configurationPath, []byte(
		"common:\n  log_level: \"${CONFIG_TEST_MISSING_LEVEL}\"\n",
	), 0o600))

	var loadedConfiguration configurationFixture
	_, loadError := utils.NewConfigurationLoader().LoadConfiguration(configurationPath, &loadedConfiguration)

	require.Error(t, loadError)
	require.Contains(t, loadError.Error(), "config_placeholder_missing")
	require.Contains(t, loadError.Error(), "CONFIG_TEST_MISSING_LEVEL")
}

func TestConfigurationLoaderRejectsUnknownFields(t *testing.T) {
	configurationPath := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(configurationPath, []byte(
		"common:\n  log_level: error\n  obsolete: true\n",
	), 0o600))

	var loadedConfiguration configurationFixture
	_, loadError := utils.NewConfigurationLoader().LoadConfiguration(configurationPath, &loadedConfiguration)

	require.Error(t, loadError)
	require.Contains(t, loadError.Error(), "config_file_parse_failed")
	require.Contains(t, loadError.Error(), "obsolete")
}

func TestConfigurationLoaderRejectsMissingFile(t *testing.T) {
	configurationPath := filepath.Join(t.TempDir(), "config.yml")

	var loadedConfiguration configurationFixture
	_, loadError := utils.NewConfigurationLoader().LoadConfiguration(configurationPath, &loadedConfiguration)

	require.Error(t, loadError)
	require.Contains(t, loadError.Error(), "config_file_read_failed")
}
