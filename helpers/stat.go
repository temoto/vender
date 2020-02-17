package helpers

import (
	"expvar"
	"io"
)

type StatReader struct {
	R io.Reader
	V *expvar.Int
	F int64
}

var _ io.Reader = &StatReader{}

func NewStatReader(r io.Reader, expvar *expvar.Int, fix int64) io.Reader {
	return &StatReader{R: r, F: fix, V: expvar}
}

func (sr *StatReader) Read(p []byte) (n int, err error) {
	n, err = sr.R.Read(p)
	sr.V.Add(int64(n) + sr.F)
	return
}

type StatWriter struct {
	W io.Writer
	V *expvar.Int
	F int64
}

var _ io.Reader = &StatReader{}

func NewStatWriter(w io.Writer, expvar *expvar.Int, fix int64) io.Writer {
	return &StatWriter{W: w, F: fix, V: expvar}
}

func (sw *StatWriter) Write(p []byte) (n int, err error) {
	n, err = sw.W.Write(p)
	sw.V.Add(int64(n) + sw.F)
	return
}
