package ioutils

import "io"

// An ErrorConversionWriter delegates all writes to W and allows to convert
// errors returned by W using the Converter function. Converter is called for
// every non-nil error returned by W. Converter should not return nil, as this
// may violate the spec of Writer (returning non-zero bytes written without
// error).
type ErrorConversionWriter struct {
	W         io.Writer             // underlying Writer
	Converter func(err error) error // converter function
}

// Write implements io.Writer
func (w *ErrorConversionWriter) Write(p []byte) (int, error) {
	written, err := w.W.Write(p)
	if err != nil {
		err = w.Converter(err)
	}
	return written, err
}
