package persist

import (
	"encoding"
	"io"
	"path/filepath"
	"sync"
	"time"

	"github.com/temoto/errors"
	"github.com/temoto/extremofile"
	"github.com/temoto/vender/log2"
)

type Stater interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

type storage interface {
	Read() ([]byte, error)
	io.Writer
}

// Binds State{Load,Store} to persistent storage
type Persist struct {
	sync.Mutex
	log     *log2.Log
	tag     string
	target  Stater
	storage storage
}

func (p *Persist) Init(tag string, target Stater, root string, enabled bool, log *log2.Log) error {
	p.tag = tag
	p.log = log
	if !enabled {
		p.log.Debugf("persist %s disabled", p.tag)
		return nil
	}
	if root == "" {
		return errors.Errorf("persist %s enabled but root=empty", p.tag)
	}
	if target == nil {
		panic("code error persist target nil")
	}
	p.target = target
	p.storage = extremofile.New(extremofile.Config{
		Dir:      filepath.Join(root, tag),
		DirPerm:  0755,
		FilePerm: 0644,
	})
	// TODO extremofile check write
	return nil
}

func (p *Persist) Load() error {
	if p.tag == "" {
		panic("code error persist must call .Init() first")
	}
	if p.storage == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()
	tbegin := time.Now()
	b, err := p.storage.Read()
	duration := time.Since(tbegin)
	p.log.Debugf("persist %s storage.read duration=%v", p.tag, duration)
	if b != nil {
		if err != nil {
			p.log.Errorf("persist %s ignore non-critical storage err=%v", p.tag, err)
		}
		err = p.target.UnmarshalBinary(b)
	}
	return errors.Annotatef(err, "persist %s Load", p.tag)
}

func (p *Persist) Store() error {
	if p.tag == "" {
		panic("code error persist must call .Init() first")
	}
	if p.storage == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()
	b, err := p.target.MarshalBinary()
	if err == nil {
		tbegin := time.Now()
		_, err = p.storage.Write(b)
		duration := time.Since(tbegin)
		p.log.Debugf("persist %s storage.write duration=%v", p.tag, duration)
	}
	return errors.Annotatef(err, "persist %s Store", p.tag)
}
