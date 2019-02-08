// Public API to easy create MDB stubs for test code.
package mdb

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"testing"
	"time"

	"github.com/juju/errors"
)

// Mock Uarter for tests
type nullUart struct {
	src io.Reader
	br  *bufio.Reader
	w   io.Writer
}

func NewNullUart(r io.Reader, w io.Writer) *nullUart {
	return &nullUart{
		src: r,
		br:  bufio.NewReader(r),
		w:   w,
	}
}

func (self *nullUart) Break(d time.Duration) error {
	self.resetRead()
	time.Sleep(d)
	return nil
}

func (self *nullUart) Close() error {
	self.src = nil
	self.w = nil
	return nil
}

func (self *nullUart) Open(path string) error { return nil }

func (self *nullUart) Tx(request, response []byte) (n int, err error) {
	if _, err = self.write9(request, true); err != nil {
		return 0, err
	}
	if _, err = self.write9([]byte{checksum(request)}, false); err != nil {
		return 0, err
	}
	buf := [PacketMaxLength]byte{}
	if n, err = self.ReadPacket(buf[:]); err != nil {
		return n, err
	}
	chkin := response[n-1]
	n--
	chkcomp := checksum(response)
	if chkin != chkcomp {
		log.Printf("debug: mdb.fileUart.Tx InvalidChecksum frompacket=%x actual=%x", chkin, chkcomp)
		return n, errors.Trace(InvalidChecksum{Received: chkin, Actual: chkcomp})
	}
	n = copy(response, buf[:n])
	if n > 0 {
		_, err = self.write9(PacketNul1.b[:1], false)
	}
	return n, err
}

func (self *nullUart) ReadPacket(buf []byte) (n int, err error) {
	var b byte
	for {
		b, err = self.br.ReadByte()
		if err != nil {
			return 0, err
		}
		if b != 0xff {
			buf[n] = b
			n++
			continue
		}
		b, err = self.br.ReadByte() // after ff
		if err != nil {
			return 0, err
		}
		switch b {
		case 0xff:
			buf[n] = b
			n++
			continue
		case 0x00:
			b, err = self.br.ReadByte() // chk
			if err != nil {
				return 0, err
			}
			buf[n] = b
			n++
			return n, err
		default:
			err = errors.Errorf("ReadPacket unknown sequence ff %x", b)
			return 0, err
		}
	}
}

func (self *nullUart) write9(p []byte, start9 bool) (int, error) {
	return self.w.Write(p)
}

func (self *nullUart) resetRead() error {
	self.br.Reset(self.src)
	return nil
}

func NewTestMDBRaw(t testing.TB) (Mdber, func([]byte), *bytes.Buffer) {
	r := bytes.NewBuffer(nil)
	w := bytes.NewBuffer(nil)
	uarter := NewNullUart(r, w)
	m, err := NewMDB(uarter, "")
	if err != nil {
		t.Fatal(err)
	}
	mockRead := func(b []byte) {
		if _, err := r.Write(b); err != nil {
			t.Fatal(err)
		}
		uarter.resetRead()
	}
	return m, mockRead, w
}

type ChanIO struct {
	r       chan []byte
	w       chan []byte
	timeout time.Duration
	rtmr    *time.Timer
	wtmr    *time.Timer
}

func (self *ChanIO) Read(p []byte) (int, error) {
	if !self.rtmr.Stop() {
		<-self.rtmr.C
	}
	self.rtmr.Reset(self.timeout)
	select {
	case b, ok := <-self.w:
		if !ok {
			return 0, io.EOF
		}
		copy(p, b)
		return len(b), nil
	case <-self.rtmr.C:
		panic("mdb mock ChanIO.Read timeout guard. mdber.Tx() without corresponding Packet channel receive")
	}
}

func (self *ChanIO) Write(p []byte) (int, error) {
	// log.Printf("cio.Write %x", p)
	if !self.wtmr.Stop() {
		<-self.wtmr.C
	}
	self.wtmr.Reset(self.timeout - time.Second)
	select {
	case self.r <- p:
		return len(p), nil
	case <-self.wtmr.C:
		panic("mdb mock ChanIO.Write timeout guard")
	}
}

func NewChanIO(timeout time.Duration) *ChanIO {
	c := &ChanIO{
		r:       make(chan []byte),
		w:       make(chan []byte),
		timeout: timeout,
		rtmr:    time.NewTimer(0),
		wtmr:    time.NewTimer(0),
	}
	return c
}

func NewTestMDBChan(t testing.TB) (Mdber, <-chan Packet, chan<- Packet) {
	cio := NewChanIO(5 * time.Second)
	uarter := NewNullUart(cio, cio)
	m, err := NewMDB(uarter, "")
	if err != nil {
		t.Fatal(err)
	}
	reqCh := make(chan Packet)
	respCh := make(chan Packet)

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
			request, err := PacketFromBytes(rb, true)
			if err != nil {
				t.Fatal(err)
			}
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

type TestReplyFunc func(t testing.TB, reqCh <-chan Packet, respCh chan<- Packet)

func TestChanTx(t testing.TB, reqCh <-chan Packet, respCh chan<- Packet, expectRequestHex, responseHex string) {
	request := <-reqCh
	request.TestHex(t, expectRequestHex)
	response, err := PacketFromHex(responseHex, true)
	if err != nil {
		t.Fatal(err)
	}
	respCh <- response
}
