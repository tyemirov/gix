package utils

import (
	"io"
	"sync"
)

// FlushingWriter ensures data written to buffered writers becomes visible immediately by invoking Flush when available.
type FlushingWriter struct {
	writer io.Writer
	mutex  sync.Mutex
}

// NewFlushingWriter wraps the provided writer and flushes it after each write when the writer supports flushing.
func NewFlushingWriter(writer io.Writer) io.Writer {
	if writer == nil {
		return nil
	}
	if _, alreadyWrapped := writer.(*FlushingWriter); alreadyWrapped {
		return writer
	}
	return &FlushingWriter{writer: writer}
}

// Write delegates to the underlying writer and flushes it when possible.
func (flushingWriter *FlushingWriter) Write(data []byte) (int, error) {
	if flushingWriter == nil || flushingWriter.writer == nil {
		return 0, nil
	}

	flushingWriter.mutex.Lock()
	defer flushingWriter.mutex.Unlock()

	bytesWritten, writeError := flushingWriter.writer.Write(data)
	if writeError != nil {
		return bytesWritten, writeError
	}

	if flushableWriter, implementsFlush := flushingWriter.writer.(interface{ Flush() error }); implementsFlush {
		if flushError := flushableWriter.Flush(); flushError != nil {
			return bytesWritten, flushError
		}
	}

	return bytesWritten, nil
}

// Unwrap returns the writer whose output is flushed by this wrapper.
func (flushingWriter *FlushingWriter) Unwrap() io.Writer {
	return flushingWriter.writer
}
