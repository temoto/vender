package extremofile

// Here be file IO routines.

import (
	"io/ioutil"
	"os"
)

func fileRead(path string) ([]byte, error) {
	f, err := os.Open(path)
	if f != nil {
		defer f.Close()
	}
	if err != nil {
		return nil, err
	}
	var rdata []byte
	rdata, err = ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	return rdata, nil
}

func fileWrite(path string, perm os.FileMode, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, perm)
	if f != nil {
		// Ignore Close() error, check full read later.
		defer f.Close()
	}
	if err != nil {
		return err
	}
	// Ignore number of bytes written, check full read later.
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	return nil
}
