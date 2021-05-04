package extremofile

import (
	"io"
	"os"
	"path/filepath"
)

const DefaultDirPerm os.FileMode = 0700
const DefaultFilePerm os.FileMode = 0600
const DefaultFilePrefix = "extremofile."

type Config struct {
	Dir        string
	FilePrefix string
	DirPerm    os.FileMode
	FilePerm   os.FileMode
	MustExist  bool
	ReadOnly   bool
}

// Does not perform IO.
func New(config Config) *efile {
	e := &efile{config: &config}
	if e.config.FilePrefix == "" {
		e.config.FilePrefix = DefaultFilePrefix
	}
	if e.config.FilePerm == 0 {
		e.config.FilePerm = DefaultFilePerm
	}
	if e.config.DirPerm == 0 {
		e.config.DirPerm = DefaultDirPerm
	}
	e.pathMain = filepath.Join(e.config.Dir, e.config.FilePrefix+"v1.main")
	e.pathBackup = filepath.Join(e.config.Dir, e.config.FilePrefix+"v1.backup")
	return e
}

// Open is simpler API, checks write access.
// Bytes and Writer may be not nil together with error to indicate non fatal issues.
func Open(dir string) ([]byte, io.Writer, error) {
	e := New(Config{Dir: dir})

	if err := e.mkdir(); IsCritical(err) {
		return nil, nil, err
	}

	data, readErr := e.read()
	if IsCritical(readErr) {
		return nil, nil, readErr
	}

	return data, e, readErr
}

func (e *efile) Read() ([]byte, error) {
	e.Lock()
	defer e.Unlock()

	if err := e.mkdir(); IsCritical(err) {
		return nil, err
	}
	data, err := e.read()
	if IsCritical(err) {
		return nil, err
	}
	return data, err
}

// Write returns after write, backup and sync. After success, `.Bytes()` will return new data.
// Write is atomic and thread-safe.
func (e *efile) Write(b []byte) (int, error) {
	e.Lock()
	defer e.Unlock()

	if err := e.mkdir(); IsCritical(err) {
		return 0, err
	}
	n := len(b)
	err := e.write(b)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// storage is not available, most commonly: disk is full or fs permission error
func IsCritical(e error) bool {
	if _, ok := e.(critical); ok {
		return true
	}
	return false
}

// data is corrupted, restore is not possible
func IsCorrupt(e error) bool {
	if x, ok := e.(critical); ok {
		e = x.E
	}
	return e == errCorrupt
}
