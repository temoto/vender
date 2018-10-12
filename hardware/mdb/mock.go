package mdb

// Public API to easy create MDB stubs to test your code.
import (
	"bufio"
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/juju/errors"
)

// Mock Uarter for tests
type nullUart struct {
	src io.Reader
	r   *bufio.Reader
	w   io.Writer
}

func NewNullUart(r io.Reader, w io.Writer) *nullUart {
	return &nullUart{
		src: r,
		r:   bufio.NewReader(r),
		w:   w,
	}
}

func (self *nullUart) set9(b bool) error { return nil }

func (self *nullUart) read(p []byte) (int, error) { return self.r.Read(p) }

func (self *nullUart) write(p []byte) (int, error) { return self.w.Write(p) }

func (self *nullUart) Break(d time.Duration) (err error) {
	self.ResetRead()
	time.Sleep(d)
	return nil
}

func (self *nullUart) Close() error {
	self.src = nil
	self.r = nil
	self.w = nil
	return nil
}

func (self *nullUart) Open(path string, baud int) (err error) { return nil }

func (self *nullUart) ReadByte() (byte, error) { return self.r.ReadByte() }

func (self *nullUart) ReadSlice(delim byte) ([]byte, error) { return self.r.ReadSlice(delim) }

func (self *nullUart) ResetRead() error {
	self.r.Reset(self.src)
	return nil
}

func NewTestMDBRaw(t testing.TB) (Mdber, func([]byte), *bytes.Buffer) {
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

type ChanIO struct {
	r       chan []byte
	w       chan []byte
	timeout time.Duration
}

func (self *ChanIO) Read(p []byte) (int, error) {
	select {
	case b, ok := <-self.w:
		if !ok {
			return 0, io.EOF
		}
		copy(p, b)
		return len(b), nil
	case <-time.After(self.timeout):
		panic("mdb mock ChanIO.Read timeout guard. mdber.Tx() without corresponding Packet channel receive")
	}
}

func (self *ChanIO) Write(p []byte) (int, error) {
	// log.Printf("cio.Write %x", p)
	select {
	case self.r <- p:
		return len(p), nil
	case <-time.After(self.timeout - time.Second):
		panic("mdb mock ChanIO.Write timeout guard")
	}
}

func NewChanIO(timeout time.Duration) *ChanIO {
	c := &ChanIO{
		r:       make(chan []byte),
		w:       make(chan []byte),
		timeout: timeout,
	}
	return c
}

func NewTestMDBChan(t testing.TB) (Mdber, <-chan *Packet, chan<- *Packet) {
	cio := NewChanIO(5 * time.Second)
	uarter := NewNullUart(cio, cio)
	m, err := NewMDB(uarter, "", 9600)
	if err != nil {
		t.Fatal(err)
	}
	reqCh := make(chan *Packet)
	respCh := make(chan *Packet)

	go func() {
		for {
			rb := make([]byte, 0, PacketMaxLength)
			// first byte, 9bit=1
			if rb1, ok := <-cio.r; !ok {
				panic("code error")
			} else {
				rb = append(rb, rb1...)
			}
			// rest bytes, 9bit=0
			if rb2, ok := <-cio.r; !ok {
				panic("code error")
			} else {
				rb = append(rb, rb2...)
			}
			rb = rb[:len(rb)-1] // minus checksum
			request := PacketFromBytes(rb)
			reqCh <- request
			response, ok := <-respCh
			if ok {
				cio.w <- response.Wire(true)
				if response.Len() > 0 {
					ackb := <-cio.r // ACK
					if !(len(ackb) == 1 && ackb[0] == 0) {
						t.Error("expected ACK")
					}
				}
			} else {
				close(cio.r)
				close(cio.w)
				return
			}
		}
	}()
	return m, reqCh, respCh
}

type TestReplyFunc func(t testing.TB, reqCh <-chan *Packet, respCh chan<- *Packet)

func TestChanTx(t testing.TB, reqCh <-chan *Packet, respCh chan<- *Packet, expectRequestHex, responseHex string) {
	request := <-reqCh
	request.TestHex(t, expectRequestHex)
	respCh <- PacketFromHex(responseHex)
}
