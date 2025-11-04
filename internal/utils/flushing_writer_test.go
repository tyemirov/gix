package utils_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/utils"
)

type recordingFlushWriter struct {
	buffer     bytes.Buffer
	flushError error
	flushCount int
}

func (writer *recordingFlushWriter) Write(data []byte) (int, error) {
	return writer.buffer.Write(data)
}

func (writer *recordingFlushWriter) Flush() error {
	writer.flushCount++
	return writer.flushError
}

func TestNewFlushingWriterBehavior(testInstance *testing.T) {
	testCases := []struct {
		name               string
		writerFactory      func() (io.Writer, *recordingFlushWriter, *bytes.Buffer)
		payload            string
		expectError        bool
		expectedOutput     string
		expectedFlushCount int
	}{
		{
			name: "flushable_writer",
			writerFactory: func() (io.Writer, *recordingFlushWriter, *bytes.Buffer) {
				flushWriter := &recordingFlushWriter{}
				return utils.NewFlushingWriter(flushWriter), flushWriter, nil
			},
			payload:            "data",
			expectedOutput:     "data",
			expectedFlushCount: 1,
		},
		{
			name: "flush_error",
			writerFactory: func() (io.Writer, *recordingFlushWriter, *bytes.Buffer) {
				flushWriter := &recordingFlushWriter{flushError: errors.New("flush failed")}
				return utils.NewFlushingWriter(flushWriter), flushWriter, nil
			},
			payload:            "content",
			expectError:        true,
			expectedOutput:     "content",
			expectedFlushCount: 1,
		},
		{
			name: "non_flushable_writer",
			writerFactory: func() (io.Writer, *recordingFlushWriter, *bytes.Buffer) {
				buffer := &bytes.Buffer{}
				return utils.NewFlushingWriter(buffer), nil, buffer
			},
			payload:            "log",
			expectedOutput:     "log",
			expectedFlushCount: 0,
		},
	}

	for testIndex := range testCases {
		testCase := testCases[testIndex]
		testInstance.Run(testCase.name, func(testInstance *testing.T) {
			writer, flushWriter, buffer := testCase.writerFactory()
			require.NotNil(testInstance, writer)

			bytesWritten, writeError := writer.Write([]byte(testCase.payload))
			require.Equal(testInstance, len(testCase.payload), bytesWritten)
			if testCase.expectError {
				require.Error(testInstance, writeError)
			} else {
				require.NoError(testInstance, writeError)
			}

			if flushWriter != nil {
				require.Equal(testInstance, testCase.expectedOutput, flushWriter.buffer.String())
				require.Equal(testInstance, testCase.expectedFlushCount, flushWriter.flushCount)
				return
			}

			if buffer != nil {
				require.Equal(testInstance, testCase.expectedOutput, buffer.String())
			}
			require.Equal(testInstance, testCase.expectedFlushCount, 0)
		})
	}
}
