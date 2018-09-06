package mdb

// Public API to easy create MDB stubs to test your code.
import (
	"bytes"
	"testing"
)

func NewTestMDB(t *testing.T) (Mdber, func([]byte), *bytes.Buffer) {
	r := bytes.NewBuffer(nil)
	w := bytes.NewBuffer(nil)
	uarter := NewNullUart(r, w)
	m, err := NewMDB(uarter, "", 9600)
	if err != nil {
		t.Fatal(err)
	}
	mockRead := func(b []byte) {
		if _, err := r.Write(b); err != nil {
			t.Fatal(err)
		}
		uarter.ResetRead()
	}
	return m, mockRead, w
}
