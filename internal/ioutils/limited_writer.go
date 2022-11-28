package ioutils

import (
	"errors"
	"io"
)

var (
	// ErrLimitExceeded is the error returned when a limit was exceeded.
	// Callers will test for ErrLimitExceeded using ==.
	ErrLimitExceeded = errors.New("limit exceeded")
)

// A LimitedWriter writes to W but limits the amount of data that can be written
// to just N bytes. Each call to Write immediately returns ErrLimitExceeded if N
// <= 0. Otherwise Write writes max N bytes to W and updates N to reflect the
// new amount remaining. If the number of byte to be written is greater than N,
// ErrLimitExceeded is returned. Any error from W is returned to the caller of
// Write, i.e. they have precedence over ErrLimitExceeded.
type LimitedWriter struct {
	W io.Writer // underlying writer
	N int64     // max bytes remaining
}

// Write implements io.Writer
func (l *LimitedWriter) Write(p []byte) (int, error) {
	if l.N <= 0 {
		return 0, ErrLimitExceeded
	}

	writeable := int64(len(p))
	if l.N < writeable {
		writeable = l.N
	}

	written, err := l.W.Write(p[:writeable])
	if written < 0 {
		written = 0
	}
	l.N -= int64(written)
	if err != nil {
		return written, err
	}
	if written < int(writeable) {
		return written, io.ErrShortWrite
	}

	if writeable < int64(len(p)) {
		err = ErrLimitExceeded
	}
	return written, err
}
