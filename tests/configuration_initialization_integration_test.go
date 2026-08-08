package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCLIMissingConfigurationOffersUserConfigurationCreation(testInstance *testing.T) {
	currentWorkingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRootDirectory := filepath.Dir(currentWorkingDirectory)

	binaryPath := buildIntegrationBinary(testInstance, repositoryRootDirectory)

	testInstance.Run("declined", func(subtest *testing.T) {
		homeDirectory := subtest.TempDir()
		outputText, runError := runBinaryIntegrationCommandWithInput(
			subtest,
			binaryPath,
			subtest.TempDir(),
			map[string]string{
				configurationInitializationHomeEnvNameConstant: homeDirectory,
				"GIX_TEST_DISABLE_CONFIG_INJECTION":            "1",
			},
			integrationCommandTimeout,
			"n\n",
			[]string{"version"},
		)

		require.Error(subtest, runError)
		require.Contains(subtest, outputText, "No gix configuration was found.")
		require.Contains(subtest, outputText, filepath.Join(homeDirectory, ".gix", "config.yml"))
		require.NoFileExists(subtest, filepath.Join(homeDirectory, ".gix", "config.yml"))
	})

	testInstance.Run("accepted", func(subtest *testing.T) {
		homeDirectory := subtest.TempDir()
		configurationPath := filepath.Join(homeDirectory, ".gix", "config.yml")
		outputText, runError := runBinaryIntegrationCommandWithInput(
			subtest,
			binaryPath,
			subtest.TempDir(),
			map[string]string{
				configurationInitializationHomeEnvNameConstant: homeDirectory,
				"LLM_PROXY_SECRET_KEY":                         "proxy-test-secret",
				"GIX_TEST_DISABLE_CONFIG_INJECTION":            "1",
			},
			integrationCommandTimeout,
			"y\n",
			[]string{"version"},
		)

		require.NoError(subtest, runError, outputText)
		require.Contains(subtest, outputText, "No gix configuration was found.")
		require.FileExists(subtest, configurationPath)
		configurationData, readError := os.ReadFile(configurationPath)
		require.NoError(subtest, readError)
		require.Contains(subtest, string(configurationData), `credential: "${LLM_PROXY_SECRET_KEY}"`)
		require.Contains(subtest, string(configurationData), `credential: "${OPENAI_API_KEY}"`)
		require.Contains(subtest, string(configurationData), `credential: "${GH_TOKEN}"`)
		require.Contains(subtest, string(configurationData), `credential: "${GITHUB_PACKAGES_TOKEN}"`)
	})
}

func TestCLIConfigurationInterpolatesProcessEnvironment(testInstance *testing.T) {
	currentWorkingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRootDirectory := filepath.Dir(currentWorkingDirectory)

	binaryPath := buildIntegrationBinary(testInstance, repositoryRootDirectory)
	workingDirectory := testInstance.TempDir()
	configurationPath := filepath.Join(workingDirectory, "config.yml")
	configurationContent := strings.Join([]string{
		"common:",
		`  log_level: "${GIX_TEST_LOG_LEVEL}"`,
		"  log_format: console",
		"  assume_yes: false",
		"  require_clean: false",
		"llm:",
		"  openai:",
		"    priority: 2",
		"    model: gpt-5.6-terra",
		`    base_url: "https://api.openai.com/v1"`,
		`    credential: "${OPENAI_API_KEY}"`,
		"  llm_proxy:",
		"    priority: 1",
		"    provider: meta",
		"    model: muse-spark-1.1",
		`    base_url: "https://llm-proxy-api.mprlab.com"`,
		`    credential: "${LLM_PROXY_SECRET_KEY}"`,
		"operations: []",
		"",
	}, "\n")
	require.NoError(testInstance, os.WriteFile(configurationPath, []byte(configurationContent), 0o600))

	outputText, runError := runBinaryIntegrationCommand(
		testInstance,
		binaryPath,
		workingDirectory,
		map[string]string{
			"GIX_TEST_LOG_LEVEL":   "debug",
			"LLM_PROXY_SECRET_KEY": "process-secret",
		},
		integrationCommandTimeout,
		[]string{"--config", configurationPath, "version"},
	)

	require.NoError(testInstance, runError, outputText)
	require.Contains(testInstance, outputText, "configuration initialized")
}

func TestCLIConfigurationIgnoresSiblingDotEnv(testInstance *testing.T) {
	const dotenvOnlyVariableName = "GIX_TEST_DOTENV_ONLY_LOG_LEVEL_9F6A3C"

	_, variableExists := os.LookupEnv(dotenvOnlyVariableName)
	require.False(testInstance, variableExists)

	currentWorkingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRootDirectory := filepath.Dir(currentWorkingDirectory)

	binaryPath := buildIntegrationBinary(testInstance, repositoryRootDirectory)
	workingDirectory := testInstance.TempDir()
	configurationPath := filepath.Join(workingDirectory, "config.yml")
	configurationContent := strings.Join([]string{
		"common:",
		`  log_level: "${` + dotenvOnlyVariableName + `}"`,
		"  log_format: console",
		"llm:",
		"  openai:",
		"    priority: 1",
		"    model: gpt-5.6-terra",
		"    base_url: https://api.openai.com/v1",
		"    credential: literal-openai-secret",
		"  llm_proxy:",
		"    priority: 2",
		"    provider: meta",
		"    model: muse-spark-1.1",
		"    base_url: https://llm-proxy-api.mprlab.com",
		`    credential: ""`,
		"operations: []",
		"",
	}, "\n")
	require.NoError(testInstance, os.WriteFile(configurationPath, []byte(configurationContent), 0o600))
	require.NoError(testInstance, os.WriteFile(
		filepath.Join(workingDirectory, ".env"),
		[]byte(dotenvOnlyVariableName+"=error\n"),
		0o600,
	))

	outputText, runError := runBinaryIntegrationCommand(
		testInstance,
		binaryPath,
		workingDirectory,
		map[string]string{},
		integrationCommandTimeout,
		[]string{"--config", configurationPath, "version"},
	)

	require.Error(testInstance, runError)
	require.Contains(testInstance, outputText, "config_placeholder_missing")
	require.Contains(testInstance, outputText, dotenvOnlyVariableName)
}

func TestCLIConfigurationRejectsMissingRequiredPlaceholder(testInstance *testing.T) {
	currentWorkingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRootDirectory := filepath.Dir(currentWorkingDirectory)

	binaryPath := buildIntegrationBinary(testInstance, repositoryRootDirectory)
	workingDirectory := testInstance.TempDir()
	configurationPath := filepath.Join(workingDirectory, "config.yml")
	require.NoError(testInstance, os.WriteFile(configurationPath, []byte(strings.Join([]string{
		"common:",
		`  log_level: "${MISSING_LOG_LEVEL}"`,
		"  log_format: console",
		"  assume_yes: false",
		"  require_clean: false",
		"operations: []",
		"",
	}, "\n")), 0o600))

	outputText, runError := runBinaryIntegrationCommand(
		testInstance,
		binaryPath,
		workingDirectory,
		map[string]string{},
		integrationCommandTimeout,
		[]string{"--config", configurationPath, "version"},
	)

	require.Error(testInstance, runError)
	require.Contains(testInstance, outputText, "config_placeholder_missing")
	require.Contains(testInstance, outputText, "MISSING_LOG_LEVEL")
}

func TestCLIConfigurationRejectsInvalidLLMSelection(testInstance *testing.T) {
	currentWorkingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRootDirectory := filepath.Dir(currentWorkingDirectory)

	binaryPath := buildIntegrationBinary(testInstance, repositoryRootDirectory)
	testCases := []struct {
		name              string
		llmFields         string
		proxyFields       string
		expectedErrorText string
	}{
		{
			name:              "model_without_provider",
			proxyFields:       "    model: muse-spark-1.1\n",
			expectedErrorText: "invalid llm configuration: llm llm_proxy provider is required",
		},
		{
			name:              "public_transport",
			llmFields:         "  transport: llm_proxy\n",
			proxyFields:       "    provider: meta\n    model: muse-spark-1.1\n",
			expectedErrorText: "transport",
		},
		{
			name:              "top_level_provider",
			llmFields:         "  provider: meta\n",
			proxyFields:       "    provider: meta\n    model: muse-spark-1.1\n",
			expectedErrorText: "provider",
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(subtest *testing.T) {
			workingDirectory := subtest.TempDir()
			configurationPath := filepath.Join(workingDirectory, "config.yml")
			configurationContent := "common:\n" +
				"  log_level: error\n" +
				"  log_format: console\n" +
				"llm:\n" +
				testCase.llmFields +
				"  openai:\n" +
				"    priority: 2\n" +
				"    model: gpt-5.6-terra\n" +
				"    base_url: https://api.openai.com/v1\n" +
				"    credential: openai-secret\n" +
				"  llm_proxy:\n" +
				"    priority: 1\n" +
				testCase.proxyFields +
				"    base_url: https://llm-proxy.example\n" +
				"    credential: proxy-secret\n" +
				"operations: []\n"
			require.NoError(subtest, os.WriteFile(configurationPath, []byte(configurationContent), 0o600))

			outputText, runError := runBinaryIntegrationCommand(
				subtest,
				binaryPath,
				workingDirectory,
				map[string]string{},
				integrationCommandTimeout,
				[]string{"--config", configurationPath, "version"},
			)

			require.Error(subtest, runError)
			require.Contains(subtest, outputText, testCase.expectedErrorText)
		})
	}
}

const (
	configurationInitializationOverwriteCaseNameConstant    = "overwrite_protection"
	configurationInitializationForceCaseNameConstant        = "force_overwrite"
	configurationInitializationCommandArgumentConstant      = "init"
	configurationInitializationLegacyArgumentConstant       = "--init"
	configurationInitializationLegacyUserArgumentConstant   = "user"
	configurationInitializationForceFlagConstant            = "--force"
	configurationInitializationHomeEnvNameConstant          = "HOME"
	configurationInitializationUserDirectoryNameConstant    = ".gix"
	configurationInitializationErrorMessageFragmentConstant = "already exists"
	configurationInitializationUnknownFlagFragmentConstant  = "unknown flag: --init"
)

func TestCLIConfigurationInitializationCreatesFiles(testInstance *testing.T) {
	currentWorkingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRootDirectory := filepath.Dir(currentWorkingDirectory)

	binaryPath := buildIntegrationBinary(testInstance, repositoryRootDirectory)

	workingDirectory := testInstance.TempDir()
	homeDirectory := testInstance.TempDir()
	expectedConfigurationPath := filepath.Join(homeDirectory, configurationInitializationUserDirectoryNameConstant, integrationConfigFileNameConstant)
	outputText, runError := runBinaryIntegrationCommand(
		testInstance,
		binaryPath,
		workingDirectory,
		map[string]string{configurationInitializationHomeEnvNameConstant: homeDirectory},
		integrationCommandTimeout,
		[]string{configurationInitializationCommandArgumentConstant},
	)
	require.NoError(testInstance, runError, outputText)

	fileContent, readError := os.ReadFile(expectedConfigurationPath)
	require.NoError(testInstance, readError)
	require.NotEmpty(testInstance, fileContent)
	require.NoFileExists(testInstance, filepath.Join(workingDirectory, integrationConfigFileNameConstant))
}

func TestCLIConfigurationInitializationRejectsRootInitFlag(testInstance *testing.T) {
	currentWorkingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRootDirectory := filepath.Dir(currentWorkingDirectory)

	binaryPath := buildIntegrationBinary(testInstance, repositoryRootDirectory)
	workingDirectory := testInstance.TempDir()
	homeDirectory := testInstance.TempDir()

	outputText, runError := runBinaryIntegrationCommand(
		testInstance,
		binaryPath,
		workingDirectory,
		map[string]string{
			configurationInitializationHomeEnvNameConstant: homeDirectory,
		},
		integrationCommandTimeout,
		[]string{
			configurationInitializationLegacyArgumentConstant,
			configurationInitializationLegacyUserArgumentConstant,
		},
	)

	require.Error(testInstance, runError)
	require.Contains(testInstance, outputText, configurationInitializationUnknownFlagFragmentConstant)

	_, localStatError := os.Stat(filepath.Join(workingDirectory, integrationConfigFileNameConstant))
	require.ErrorIs(testInstance, localStatError, os.ErrNotExist)

	_, userStatError := os.Stat(filepath.Join(homeDirectory, configurationInitializationUserDirectoryNameConstant, integrationConfigFileNameConstant))
	require.ErrorIs(testInstance, userStatError, os.ErrNotExist)
}

func TestCLIConfigurationInitializationOverwriteProtection(testInstance *testing.T) {
	currentWorkingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRootDirectory := filepath.Dir(currentWorkingDirectory)

	binaryPath := buildIntegrationBinary(testInstance, repositoryRootDirectory)

	testCases := []struct {
		name            string
		secondArguments []string
		expectError     bool
	}{
		{
			name:            configurationInitializationOverwriteCaseNameConstant,
			secondArguments: []string{configurationInitializationCommandArgumentConstant},
			expectError:     true,
		},
		{
			name: configurationInitializationForceCaseNameConstant,
			secondArguments: []string{
				configurationInitializationCommandArgumentConstant,
				configurationInitializationForceFlagConstant,
			},
			expectError: false,
		},
	}

	for testCaseIndex, testCase := range testCases {
		testInstance.Run(fmt.Sprintf(integrationSubtestNameTemplateConstant, testCaseIndex, testCase.name), func(t *testing.T) {
			workingDirectory := t.TempDir()
			homeDirectory := t.TempDir()
			environmentOverrides := map[string]string{
				configurationInitializationHomeEnvNameConstant: homeDirectory,
			}

			firstOutput, firstError := runBinaryIntegrationCommand(
				t,
				binaryPath,
				workingDirectory,
				environmentOverrides,
				integrationCommandTimeout,
				[]string{configurationInitializationCommandArgumentConstant},
			)
			require.NoError(t, firstError, firstOutput)

			configurationPath := filepath.Join(homeDirectory, configurationInitializationUserDirectoryNameConstant, integrationConfigFileNameConstant)
			initialContent, readError := os.ReadFile(configurationPath)
			require.NoError(t, readError)
			require.NotEmpty(t, initialContent)

			secondOutput, secondError := runBinaryIntegrationCommand(
				t,
				binaryPath,
				workingDirectory,
				environmentOverrides,
				integrationCommandTimeout,
				testCase.secondArguments,
			)

			if testCase.expectError {
				require.Error(t, secondError)
				require.Contains(t, secondOutput, configurationInitializationErrorMessageFragmentConstant)

				resultingContent, verifyError := os.ReadFile(configurationPath)
				require.NoError(t, verifyError)
				require.Equal(t, initialContent, resultingContent)
				return
			}

			require.NoError(t, secondError, secondOutput)

			resultingContent, verifyError := os.ReadFile(configurationPath)
			require.NoError(t, verifyError)
			require.NotEmpty(t, resultingContent)
		})
	}
}
