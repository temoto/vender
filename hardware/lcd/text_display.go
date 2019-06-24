package lcd

import (
	"bytes"
	"sync"
	"sync/atomic"
	"time"

	"github.com/paulrosania/go-charset/charset"
	_ "github.com/paulrosania/go-charset/data"
	"github.com/temoto/alive"
	"github.com/temoto/errors"
)

// TODO extract this generic text display code
// away from hardware HD44780/MT-16 driver

const MaxWidth = 40

var spaceBytes = bytes.Repeat([]byte{' '}, MaxWidth)

type TextDisplay struct { //nolint:maligned
	alive *alive.Alive
	mu    sync.Mutex
	dev   Devicer
	tr    atomic.Value
	width uint32
	line1 []byte
	line2 []byte

	tickd time.Duration
	tick  uint16
	upd   chan<- struct{}
}

type Devicer interface {
	Clear()
	// Control() Control
	// SetControl(new Control) Control
	CursorYX(y, x uint8) bool
	Write(b []byte)
}

func NewTextDisplay(width uint16, codepage string, scrollDelay time.Duration) (*TextDisplay, error) {
	self := &TextDisplay{
		alive: alive.NewAlive(),
		width: uint32(width),
		tickd: scrollDelay,
	}

	if codepage != "" {
		if err := self.SetCodepage(codepage); err != nil {
			return nil, errors.Trace(err)
		}
	}

	return self, nil
}

func (self *TextDisplay) SetCodepage(cp string) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	tr, err := charset.TranslatorTo(cp)
	if err != nil {
		return err
	}
	self.tr.Store(tr)
	return nil
}
func (self *TextDisplay) SetDevice(dev *LCD) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.dev = dev
}
func (self *TextDisplay) SetScrollDelay(d time.Duration) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.tickd = d
}

func (self *TextDisplay) Clear() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.line1 = nil
	self.line2 = nil
	self.flush()
}

func (self *TextDisplay) Message(s1, s2 string, wait func()) {
	next1, next2 := self.Translate(s1), self.Translate(s2)

	self.mu.Lock()
	prev1, prev2 := self.line1, self.line2
	self.line1, self.line2 = next1, next2
	// self.tick = 0
	self.flush()
	self.mu.Unlock()

	wait()

	self.mu.Lock()
	self.line1, self.line2 = prev1, prev2
	self.flush()
	self.mu.Unlock()
}

// nil: don't change
// len=0: set empty
func (self *TextDisplay) SetLinesBytes(b1, b2 []byte) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if b1 != nil {
		self.line1 = b1
	}
	if b2 != nil {
		self.line2 = b2
	}
	self.tick = 0
	self.flush()
}
func (self *TextDisplay) SetLine1(line1 string) {
	self.SetLinesBytes(self.Translate(line1), nil)
}
func (self *TextDisplay) SetLine2(line2 string) {
	self.SetLinesBytes(nil, self.Translate(line2))
}
func (self *TextDisplay) SetLines(line1, line2 string) {
	self.SetLinesBytes(
		self.Translate(line1),
		self.Translate(line2))
}

func (self *TextDisplay) Tick() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.tick++
	self.flush()
}

func (self *TextDisplay) Run() {
	tmr := time.NewTicker(self.tickd)
	stopch := self.alive.StopChan()

	for self.alive.IsRunning() {
		select {
		case <-tmr.C:
			self.Tick()
		case <-stopch:
			tmr.Stop()
			return
		}
	}
}

// sometimes returns slice into shared spaceBytes
// sometimes returns `b` (len>=width-1)
// sometimes allocates new buffer
func (self *TextDisplay) JustCenter(b []byte) []byte {
	l := len(b)
	w := int(atomic.LoadUint32(&self.width))

	// optimize short paths
	if l == 0 {
		return spaceBytes[:w]
	}
	if l >= w-1 {
		return b
	}
	padtotal := w - l
	n := padtotal / 2
	padleft := spaceBytes[:n]
	padright := spaceBytes[:n+padtotal%2] // account for odd length
	buf := make([]byte, 0, w)
	buf = append(append(append(buf, padleft...), b...), padright...)
	return buf
}

// returns `b` when len>=width
// otherwise pads with spaces
func (self *TextDisplay) PadRight(b []byte) []byte {
	l := len(b)
	w := int(atomic.LoadUint32(&self.width))

	if l == 0 {
		return spaceBytes[:w]
	}
	if l >= w {
		return b
	}
	buf := make([]byte, 0, w)
	buf = append(append(buf, b...), spaceBytes[:w-l]...)
	return buf
}

func (self *TextDisplay) Translate(s string) []byte {
	if len(s) == 0 {
		return spaceBytes[:0]
	}

	// pad by default, \x00 marks place for cursor
	pad := true
	if s[len(s)-1] == '\x00' {
		pad = false
		s = s[:len(s)-1]
	}

	result := []byte(s)
	tr, ok := self.tr.Load().(charset.Translator)
	if ok && tr != nil {
		_, tb, err := tr.Translate(result, true)
		if err != nil {
			panic(err)
		}
		// translator reuses single internal buffer, make a copy
		result = append([]byte(nil), tb...)
	}

	if pad {
		result = self.PadRight(result)
	}
	return result
}

func (self *TextDisplay) SetUpdateChan(ch chan<- struct{}) {
	self.upd = ch
}

func (self *TextDisplay) flush() {
	var buf1 [MaxWidth]byte
	var buf2 [MaxWidth]byte
	b1 := buf1[:self.width]
	b2 := buf2[:self.width]
	n1 := scrollWrap(b1, self.line1, self.tick)
	n2 := scrollWrap(b2, self.line2, self.tick)

	// === Option 1: clear
	// self.dev.Clear()
	// self.dev.Write(b1[:n1])
	// self.dev.CursorYX(2, 1)
	// self.dev.Write(b2[:n2])

	// === Option 2: rewrite without clear, looks smoother
	// no padding: "erase" modified area, for now - whole line
	if n1 < self.width {
		self.dev.CursorYX(1, 1)
		self.dev.Write(spaceBytes[:self.width])
	}
	if len(self.line1) > 0 {
		self.dev.CursorYX(1, 1)
		self.dev.Write(b1[:n1])
	}
	// no padding: "erase" modified area, for now - whole line
	if n2 < self.width {
		self.dev.CursorYX(2, 1)
		self.dev.Write(spaceBytes[:self.width])
	}
	if len(self.line2) > 0 {
		self.dev.CursorYX(2, 1)
		self.dev.Write(b2[:n2])
	}

	if self.upd != nil {
		self.upd <- struct{}{}
	}
}

// relies that len(buf) == display width
func scrollWrap(buf []byte, content []byte, tick uint16) uint32 {
	length := len(content)
	width := uint16(len(buf))
	gap := uint16(width / 2)
	n := 0
	if uint16(length) <= width {
		n = copy(buf, content)
		copy(buf[n:], spaceBytes)
		return uint32(n)
	}

	offset := tick % (uint16(length) + gap)
	if offset < uint16(length) {
		n = copy(buf, content[offset:])
	} else {
		gap = gap - (offset - uint16(length))
	}
	n += copy(buf[n:], spaceBytes[:gap])
	n += copy(buf[n:], content[0:])
	return uint32(n)
}
