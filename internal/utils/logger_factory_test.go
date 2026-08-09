package utils_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/utils"
)

const (
	testLoggerFactoryCaseSupportedFormatConstant   = "supported_log_level_%s_format_%s"
	testLoggerFactoryCaseUnsupportedLevelConstant  = "unsupported_log_level"
	testLoggerFactoryCaseUnsupportedFormatConstant = "unsupported_log_format"
	testLoggerFactorySubtestTemplateConstant       = "%d_%s"
	testInvalidLogLevelConstant                    = "invalid"
	testInvalidLogFormatConstant                   = "invalid"
	testLogMessageConstant                         = "logger_factory_test_message"
	testConsoleLogMessageConstant                  = "console_event_message"
)

func TestLoggerFactoryCreateLogger(testInstance *testing.T) {
	testCases := []struct {
		name                string
		requestedLogLevel   utils.LogLevel
		requestedLogFormat  utils.LogFormat
		expectError         bool
		expectStructuredLog bool
		expectConsoleOutput bool
	}{
		{
			name:                fmt.Sprintf(testLoggerFactoryCaseSupportedFormatConstant, utils.LogLevelDebug, utils.LogFormatStructured),
			requestedLogLevel:   utils.LogLevelDebug,
			requestedLogFormat:  utils.LogFormatStructured,
			expectError:         false,
			expectStructuredLog: true,
			expectConsoleOutput: false,
		},
		{
			name:                fmt.Sprintf(testLoggerFactoryCaseSupportedFormatConstant, utils.LogLevelInfo, utils.LogFormatStructured),
			requestedLogLevel:   utils.LogLevelInfo,
			requestedLogFormat:  utils.LogFormatStructured,
			expectError:         false,
			expectStructuredLog: true,
			expectConsoleOutput: false,
		},
		{
			name:                fmt.Sprintf(testLoggerFactoryCaseSupportedFormatConstant, utils.LogLevelInfo, utils.LogFormatConsole),
			requestedLogLevel:   utils.LogLevelInfo,
			requestedLogFormat:  utils.LogFormatConsole,
			expectError:         false,
			expectStructuredLog: false,
			expectConsoleOutput: true,
		},
		{
			name:               testLoggerFactoryCaseUnsupportedLevelConstant,
			requestedLogLevel:  utils.LogLevel(testInvalidLogLevelConstant),
			requestedLogFormat: utils.LogFormatStructured,
			expectError:        true,
		},
		{
			name:               testLoggerFactoryCaseUnsupportedFormatConstant,
			requestedLogLevel:  utils.LogLevelInfo,
			requestedLogFormat: utils.LogFormat(testInvalidLogFormatConstant),
			expectError:        true,
		},
	}

	for testCaseIndex, testCase := range testCases {
		testInstance.Run(fmt.Sprintf(testLoggerFactorySubtestTemplateConstant, testCaseIndex, testCase.name), func(testInstance *testing.T) {
			loggerFactory := utils.NewLoggerFactory()

			pipeReader, pipeWriter, pipeError := os.Pipe()
			require.NoError(testInstance, pipeError)

			originalStderr := os.Stderr
			os.Stderr = pipeWriter

			loggerOutputs, creationError := loggerFactory.CreateLoggerOutputs(testCase.requestedLogLevel, testCase.requestedLogFormat)

			os.Stderr = originalStderr

			if testCase.expectError {
				require.Error(testInstance, creationError)
				require.Zero(testInstance, loggerOutputs)

				require.NoError(testInstance, pipeWriter.Close())
				require.NoError(testInstance, pipeReader.Close())
				return
			}

			require.NoError(testInstance, creationError)
			require.NotNil(testInstance, loggerOutputs.DiagnosticLogger)

			loggerOutputs.DiagnosticLogger.Info(testLogMessageConstant)
			syncError := loggerOutputs.DiagnosticLogger.Sync()
			if syncError != nil {
				// Some operating systems return syscall.ENOTSUP when Sync is not supported,
				// others surface syscall.EINVAL when Sync is a no-op, while certain Linux
				// environments surface syscall.EBADF for closed file descriptors. Certain
				// macOS environments propagate syscall.ENOTTY for console file descriptors.
				// All scenarios indicate a benign Sync outcome for zap loggers across
				// supported platforms, so they are treated as acceptable here.
				require.True(
					testInstance,
					errors.Is(syncError, syscall.ENOTSUP) ||
						errors.Is(syncError, syscall.EINVAL) ||
						errors.Is(syncError, syscall.EBADF) ||
						errors.Is(syncError, syscall.ENOTTY),
				)
			}

			loggerOutputs.ConsoleLogger.Info(testConsoleLogMessageConstant)
			_ = loggerOutputs.ConsoleLogger.Sync()

			require.NoError(testInstance, pipeWriter.Close())

			capturedOutput, readError := io.ReadAll(pipeReader)
			require.NoError(testInstance, readError)
			require.NoError(testInstance, pipeReader.Close())

			trimmedOutput := bytes.TrimSpace(capturedOutput)
			require.NotEmpty(testInstance, trimmedOutput)
			require.Contains(testInstance, string(trimmedOutput), testLogMessageConstant)
			if testCase.expectConsoleOutput {
				require.Contains(testInstance, string(trimmedOutput), testConsoleLogMessageConstant)
			} else {
				require.NotContains(testInstance, string(trimmedOutput), testConsoleLogMessageConstant)
			}

			outputLines := bytes.Split(trimmedOutput, []byte("\n"))
			require.NotEmpty(testInstance, outputLines)
			firstLine := outputLines[0]
			isJSONLog := json.Valid(firstLine)
			if testCase.expectStructuredLog {
				require.True(testInstance, isJSONLog)
			} else {
				require.False(testInstance, isJSONLog)
			}
		})
	}
}
