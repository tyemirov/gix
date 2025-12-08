package prompt_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/repos/prompt"
	"github.com/tyemirov/gix/internal/repos/shared"
)

type failingReader struct {
	err error
}

func (reader failingReader) Read(target []byte) (int, error) {
	return 0, reader.err
}

type recordingWriter struct {
	buffer bytes.Buffer
	err    error
	writes int
}

func (writer *recordingWriter) Write(data []byte) (int, error) {
	writer.writes++
	if writer.err != nil {
		return 0, writer.err
	}
	return writer.buffer.Write(data)
}

const (
	promptMessageConstant = "Confirm action? "
)

func TestIOConfirmationPrompterConfirm(testInstance *testing.T) {
	testCases := []struct {
		name             string
		reader           io.Reader
		writer           *recordingWriter
		expectedResult   shared.ConfirmationResult
		expectedError    error
		expectPromptEcho bool
	}{
		{
			name:             "decline_response",
			reader:           strings.NewReader("no\n"),
			writer:           &recordingWriter{},
			expectedResult:   shared.ConfirmationResult{},
			expectPromptEcho: true,
		},
		{
			name:             "affirmative_short_response",
			reader:           strings.NewReader("y\n"),
			writer:           &recordingWriter{},
			expectedResult:   shared.ConfirmationResult{Confirmed: true},
			expectPromptEcho: true,
		},
		{
			name:             "affirmative_apply_all_response",
			reader:           strings.NewReader("all\n"),
			writer:           &recordingWriter{},
			expectedResult:   shared.ConfirmationResult{Confirmed: true, ApplyToAll: true},
			expectPromptEcho: true,
		},
		{
			name:             "affirmative_apply_all_uppercase",
			reader:           strings.NewReader("A\n"),
			writer:           &recordingWriter{},
			expectedResult:   shared.ConfirmationResult{Confirmed: true, ApplyToAll: true},
			expectPromptEcho: true,
		},
		{
			name:          "read_error",
			reader:        failingReader{err: errors.New("read failure")},
			writer:        &recordingWriter{},
			expectedError: errors.New("read failure"),
		},
		{
			name:          "write_error",
			reader:        strings.NewReader("y\n"),
			writer:        &recordingWriter{err: errors.New("write failure")},
			expectedError: errors.New("write failure"),
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(testingInstance *testing.T) {
			prompter := prompt.NewIOConfirmationPrompter(testCase.reader, testCase.writer)
			result, err := prompter.Confirm(promptMessageConstant)

			if testCase.expectedError != nil {
				require.Error(testingInstance, err)
				require.ErrorContains(testingInstance, err, testCase.expectedError.Error())
				return
			}

			require.NoError(testingInstance, err)
			require.Equal(testingInstance, testCase.expectedResult, result)

			if testCase.expectPromptEcho {
				require.NotNil(testingInstance, testCase.writer)
				require.Equal(testingInstance, promptMessageConstant, testCase.writer.buffer.String())
				require.Equal(testingInstance, 1, testCase.writer.writes)
			}
		})
	}
}
