package cli

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

type stdoutCapture struct {
	original *os.File
	reader   *os.File
	writer   *os.File
}

func startStdoutCapture(t *testing.T) stdoutCapture {
	t.Helper()

	reader, writer, pipeError := os.Pipe()
	require.NoError(t, pipeError)

	capture := stdoutCapture{
		original: os.Stdout,
		reader:   reader,
		writer:   writer,
	}

	os.Stdout = writer
	return capture
}

func (capture *stdoutCapture) Stop(t *testing.T) string {
	t.Helper()

	os.Stdout = capture.original
	require.NoError(t, capture.writer.Close())

	capturedBytes, readError := io.ReadAll(capture.reader)
	require.NoError(t, readError)
	require.NoError(t, capture.reader.Close())

	output := string(capturedBytes)
	capture.reader = nil
	capture.writer = nil
	return output
}

func TestApplicationVersionFlagPrintsVersionAndExits(t *testing.T) {
	application := NewApplication()
	configureApplicationWithTestConfig(t, application)
	application.versionResolver = func(context.Context) string {
		return "v2.0.0"
	}

	exitCode := -1
	sentinel := "version-exit"
	application.exitFunction = func(code int) {
		exitCode = code
		panic(sentinel)
	}

	capture := startStdoutCapture(t)
	defer func() {
		if capture.reader != nil {
			_ = capture.Stop(t)
		}
	}()

	originalArgs := os.Args
	defer func() {
		os.Args = originalArgs
	}()
	os.Args = []string{"gix", "--version"}

	require.PanicsWithValue(t, sentinel, func() {
		_ = application.Execute()
	})

	output := capture.Stop(t)
	require.Equal(t, "gix version: v2.0.0\n", output)
	require.Equal(t, 0, exitCode)
}

func TestApplicationVersionCommandPrintsVersion(t *testing.T) {
	application := NewApplication()
	configureApplicationWithTestConfig(t, application)
	application.versionResolver = func(context.Context) string {
		return "v2.0.0"
	}

	exitCode := -1
	application.exitFunction = func(code int) {
		exitCode = code
		panic("unexpected exit invocation")
	}

	capture := startStdoutCapture(t)
	defer func() {
		if capture.reader != nil {
			_ = capture.Stop(t)
		}
	}()

	originalArgs := os.Args
	defer func() {
		os.Args = originalArgs
	}()
	os.Args = []string{"gix", "version"}

	require.NoError(t, application.Execute())

	output := capture.Stop(t)
	require.Equal(t, "gix version: v2.0.0\n", output)
	require.Equal(t, -1, exitCode)
}
