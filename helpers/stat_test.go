package helpers

import (
	"bytes"
	"expvar"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatReader(t *testing.T) {
	var counter expvar.Int
	s := NewStatReader(strings.NewReader(strings.Repeat(".", 1024)), &counter, 0)
	assert.Equal(t, int64(0), counter.Value())
	buf := make([]byte, 17)
	_, _ = s.Read(buf[:0])
	assert.Equal(t, int64(0), counter.Value())
	_, _ = s.Read(buf[:5])
	assert.Equal(t, int64(5), counter.Value())
	_, _ = s.Read(buf)
	assert.Equal(t, int64(22), counter.Value())
}

func TestStatWriter(t *testing.T) {
	var counter expvar.Int
	s := NewStatWriter(bytes.NewBuffer(nil), &counter, 0)
	assert.Equal(t, int64(0), counter.Value())
	buf := make([]byte, 17)
	_, _ = s.Write(buf[:0])
	assert.Equal(t, int64(0), counter.Value())
	_, _ = s.Write(buf[:5])
	assert.Equal(t, int64(5), counter.Value())
	_, _ = s.Write(buf)
	assert.Equal(t, int64(22), counter.Value())
}
