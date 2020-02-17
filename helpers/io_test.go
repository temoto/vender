package helpers

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriteAll(t *testing.T) {
	t.Parallel()
	buf := bytes.NewBuffer(nil)
	content := []byte("12345678901234567890")
	tw := &throttleWriter{buf, 7}
	n, err := tw.Write(content[:2])
	assert.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, 2, buf.Len())
	buf.Reset()
	n, err = tw.Write(content)
	assert.NoError(t, err)
	assert.Equal(t, tw.n, n)
	assert.Equal(t, tw.n, buf.Len())
	buf.Reset()
	err = WriteAll(tw, content)
	assert.NoError(t, err)
	assert.Equal(t, len(content), buf.Len())
}

type throttleWriter struct {
	w io.Writer
	n int
}

func (tw *throttleWriter) Write(p []byte) (n int, err error) {
	limit := len(p)
	if limit > tw.n {
		limit = tw.n
	}
	// log.Printf("throttle len=%d cap=%d sliced=%d", len(p), cap(p), len(p[:limit]))
	return tw.w.Write(p[:limit])
}
