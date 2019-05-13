package state

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/juju/errors"
)

type FullReader interface {
	Normalize(key string) string
	// nil,nil = not found
	ReadAll(key string) ([]byte, error)
}

type OsFullReader struct {
	base string
}

func NewOsFullReader(basePath string) *OsFullReader {
	abs, err := filepath.Abs(basePath)
	if err != nil {
		err = errors.Annotatef(err, "filepath.Abs() path=%s", basePath)
		log.Fatal(errors.ErrorStack(err))
	}
	return &OsFullReader{base: abs}
}

func (self OsFullReader) Normalize(path string) string {
	return filepath.Clean(filepath.Join(self.base, path))
}

func (OsFullReader) ReadAll(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	b, err := ioutil.ReadAll(f)
	f.Close()
	return b, err
}

type MockFullReader struct {
	Map map[string]string
}

func NewMockFullReader(sources map[string]string) *MockFullReader {
	return &MockFullReader{Map: sources}
}

func (self *MockFullReader) Normalize(name string) string {
	return filepath.Clean(name)
}

func (self *MockFullReader) ReadAll(name string) ([]byte, error) {
	if s, ok := self.Map[name]; ok {
		return []byte(s), nil
	}
	return nil, nil
}
