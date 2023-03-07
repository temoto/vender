package extremofile

import (
	"bytes"
	"fmt"
	"hash/crc64"
	"os"
	"sync"
)

const checkSize = crc64.Size

var crc64ecma = crc64.MakeTable(crc64.ECMA)

type efile struct {
	sync.Mutex
	config     *Config
	pathMain   string
	pathBackup string
}

func (e *efile) mkdir() error {
	return wrapCritical(os.MkdirAll(e.config.Dir, e.config.DirPerm))
}

func (e *efile) read() ([]byte, error) {
	var errMain, errBackup error
	var raw, data []byte
	raw, errMain = fileRead(e.pathMain)
	if errMain == nil {
		if data, errMain = parse(raw); errMain == nil {
			return data, nil // success path
		}
	}
	if raw, errBackup = fileRead(e.pathBackup); errBackup == nil {
		if data, errBackup = parse(raw); errBackup == nil {
			return data, errMain // success path backup worked
		}
	}

	switch {
	case os.IsNotExist(errMain) && os.IsNotExist(errBackup):
		return nil, nil // success path no data

	case errMain != nil && errMain == errBackup:
		// main and backup have same error
		return nil, wrapCritical(errMain)

	case os.IsNotExist(errMain):
		// main doesn't exist, return backup read/parse error
		return nil, wrapCritical(errBackup)

	case os.IsNotExist(errBackup):
		// backup doesn't exist, return main read/parse error which led to trying backup
		return nil, wrapCritical(errMain)
	}

	// unlabeled combination of errors
	return nil, wrapCritical(fmt.Errorf("extremofile.read errors main=%v backup=%v", errMain, errBackup))
}

func (e *efile) write(data []byte) error {
	chk := checksum(data)
	b := append(data, chk...)
	em := fileWrite(e.pathMain, e.config.FilePerm, b)
	eb := fileWrite(e.pathBackup, e.config.FilePerm, b)
	// TODO read, check
	if em == nil && eb == nil {
		return nil // success path
	}
	if em == nil && eb != nil {
		return eb // not critical, main is written
	}
	return wrapCritical(fmt.Errorf("extremofile.write errors main=%v backup=%v", em, eb))
}

func checksum(b []byte) []byte {
	h := crc64.New(crc64ecma)
	_, _ = h.Write(b)
	return h.Sum(nil)
}

func parse(b []byte) ([]byte, error) {
	blen := len(b)
	if blen < checkSize {
		return nil, errMetaParse
	}
	data, expectSum := b[:blen-checkSize], b[blen-checkSize:]
	dataSum := checksum(data)
	if !bytes.Equal(expectSum, dataSum) {
		return nil, errCorrupt
	}
	return data, nil
}
