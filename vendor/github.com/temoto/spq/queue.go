// Persistent queue library designed for single process thread-safe job queue
// where data loss is not an option at cost of speed.
package spq

import (
	"encoding"
	"sync"

	"github.com/juju/errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

const OnlyForTesting = "\x00"

type Queue struct { //nolint:maligned
	db         *leveldb.DB
	dbROpt     opt.ReadOptions
	dbWOpt     opt.WriteOptions
	dbRangeAll *util.Range

	mu      sync.RWMutex
	closed  bool
	next    uint64
	readch  chan struct{}
	stopch  chan struct{}
	rdonech chan struct{}
}

func Open(path string) (*Queue, error) {
	q := &Queue{
		readch:  make(chan struct{}, 1),
		stopch:  make(chan struct{}),
		rdonech: make(chan struct{}),

		dbROpt: opt.ReadOptions{},
		dbWOpt: opt.WriteOptions{
			NoWriteMerge: true,
			Sync:         true,
		},
		dbRangeAll: &util.Range{
			Start: itemKeyPrefix[:],
			Limit: itemKeyLimit[:],
		},
	}
	err := q.load(path)
	return q, err
}

func (q *Queue) load(path string) error {
	opt := &opt.Options{
		BlockCacheCapacity:   -1,
		BlockRestartInterval: 1, // checksum each key, if I understood doc correctly
		BlockSize:            1 << 10,
		DisableBlockCache:    true,
		NoSync:               false,
		NoWriteMerge:         true,
		Strict:               opt.StrictJournalChecksum | opt.StrictBlockChecksum,
		WriteBuffer:          4 << 10,
	}
	var err error
	if path == OnlyForTesting {
		q.db, err = leveldb.Open(storage.NewMemStorage(), opt)
	} else {
		q.db, err = leveldb.RecoverFile(path, opt)
		// q.db, err = leveldb.OpenFile(path, opt)
	}
	if err != nil {
		return errors.Annotate(err, "leveldb open")
	}
	err = q.db.CompactRange(util.Range{})
	if err != nil {
		return errors.Annotate(err, "leveldb compact")
	}

	iter := q.db.NewIterator(q.dbRangeAll, &q.dbROpt)
	defer iter.Release()
	if iter.Last() {
		q.next, err = unkey(iter.Key())
		if err != nil {
			return errors.Annotatef(err, "spq load key=%x", iter.Key())
		}
	}
	q.next++

	return nil
}

func (q *Queue) Close() error {
	var err error
	q.mu.Lock()
	if !q.closed {
		close(q.stopch)
		err = q.db.Close()
		q.closed = true
	}
	q.mu.Unlock()
	return err
}

func (q *Queue) MarshalPush(item encoding.BinaryMarshaler) error {
	b, err := item.MarshalBinary()
	if err != nil {
		return err
	}
	return q.Push(b)
}

func (q *Queue) Push(value []byte) error {
	var key [keyLen]byte
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return ErrClosed
	}
	encodeKey(key[:], q.next)
	err := q.db.Put(key[:], value, &q.dbWOpt)
	if err != nil {
		return err
	}
	q.next++
	signal(q.readch)
	return nil
}

func (q *Queue) Peek() (Box, error) {
	var box Box
	for {
		q.mu.RLock()
		if !q.closed {
			box = q.dbReadFirst()
		} else {
			box = Box{err: ErrClosed}
		}
		q.mu.RUnlock()
		if !box.empty() {
			return box, box.err
		}
		select {
		case <-q.readch: // success path
		case <-q.stopch:
			return Box{}, ErrClosed
		}
	}
}

func (q *Queue) Delete(box Box) error {
	if _, err := unkey(box.key[:]); err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return ErrClosed
	}
	return q.db.Delete(box.key[:], &q.dbWOpt)
}

// Atomic Delete+Push
func (q *Queue) DeletePush(box Box) error {
	if _, err := unkey(box.key[:]); err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return ErrClosed
	}

	var newKey [keyLen]byte
	encodeKey(newKey[:], q.next)
	b := leveldb.Batch{}
	b.Delete(box.key[:])
	b.Put(newKey[:], box.value)
	err := q.db.Write(&b, &q.dbWOpt)
	if err != nil {
		return err
	}
	q.next++
	signal(q.readch)
	return nil
}

func (q *Queue) dbReadFirst() Box {
	iter := q.db.NewIterator(q.dbRangeAll, &q.dbROpt)
	defer iter.Release()
	if !iter.First() {
		return Box{err: iter.Error()}
	}

	k := iter.Key()
	v := iter.Value()
	box := Box{value: make([]byte, len(v))}
	copy(box.key[:], k)
	copy(box.value, v)

	return box
}

func signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}
