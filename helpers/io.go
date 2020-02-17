package helpers

import (
	"io"
)

func WriteAll(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if err != nil {
			return err
		}
		if n == len(b) {
			return nil
		}
		b = b[n:]
	}
	return nil
}
