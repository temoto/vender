package text_display

import (
	"bytes"
	"fmt"

	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/errors"
	"github.com/paulrosania/go-charset/charset"
	_ "github.com/paulrosania/go-charset/data"
	"github.com/temoto/alive/v2"
	"github.com/temoto/vender/internal/global"
	"github.com/temoto/vender/internal/types"
)

const MaxWidth = 40

var spaceBytes = bytes.Repeat([]byte{' '}, MaxWidth)

type TextDisplay struct { //nolint:maligned
	alive *alive.Alive
	mu    sync.Mutex
	dev   Devicer
	tr    atomic.Value
	width uint32
	state State

	tickd time.Duration
	tick  uint32
	upd   chan<- State
}

type TextDisplayConfig struct {
	Codepage    string
	ScrollDelay time.Duration
	Width       uint32
}

type Devicer interface {
	Clear()
	// Control() Control
	// SetControl(new Control) Control
	CursorYX(y, x uint8) bool
	Write(b []byte)
}

func NewTextDisplay(opt *TextDisplayConfig) (*TextDisplay, error) {
	if opt == nil {
		panic("code error TODO make default TextDisplayConfig")
	}
	self := &TextDisplay{
		alive: alive.NewAlive(),
		tickd: opt.ScrollDelay,
		width: uint32(opt.Width),
	}

	if opt.Codepage != "" {
		if err := self.SetCodepage(opt.Codepage); err != nil {
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
func (self *TextDisplay) SetDevice(dev Devicer) {
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

	self.state.Clear()
	self.flush()
}

func (self *TextDisplay) Message(s1, s2 string, wait func()) {
	next := State{
		L1: self.Translate(s1),
		L2: self.Translate(s2),
	}

	self.mu.Lock()
	prev := self.state
	self.state = next
	// atomic.StoreUint32(&self.tick, 0)
	self.flush()
	self.mu.Unlock()

	wait()

	self.mu.Lock()
	self.state = prev
	self.flush()
	self.mu.Unlock()
}

// nil: don't change
// len=0: set empty
func (self *TextDisplay) SetLinesBytes(b1, b2 []byte) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if b1 != nil {
		self.state.L1 = b1
	}
	if b2 != nil {
		self.state.L2 = b2
	}
	atomic.StoreUint32(&self.tick, 0)
	self.flush()
}

func (self *TextDisplay) SetLines(line1, line2 string) {
	self.SetLinesBytes(
		self.Translate(line1),
		self.Translate(line2))
	if types.VMC.HW.Display.L1 != line1 {
		types.VMC.HW.Display.L1 = line1
		global.Log.Infof("Display.L1=%s", line1)
	}
	if types.VMC.HW.Display.L2 != line2 {
		types.VMC.HW.Display.L2 = line2
		global.Log.Infof("Display.L2=%s", line2)
	}
}

func (self *TextDisplay) Tick() {
	self.mu.Lock()
	defer self.mu.Unlock()

	atomic.AddUint32(&self.tick, 1)
	self.flush()
}

func (self *TextDisplay) Run() {
	self.mu.Lock()
	delay := self.tickd
	self.mu.Unlock()
	if delay == 0 {
		return
	}
	tmr := time.NewTicker(delay)
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
	return PadSpace(b, self.width)
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

func (self *TextDisplay) SetUpdateChan(ch chan<- State) {
	self.upd = ch
}

func (self *TextDisplay) State() State { return self.state.Copy() }

func (self *TextDisplay) flush() {
	var buf1 [MaxWidth]byte
	var buf2 [MaxWidth]byte
	b1 := buf1[:self.width]
	b2 := buf2[:self.width]
	tick := atomic.LoadUint32(&self.tick)
	n1 := scrollWrap(b1, self.state.L1, tick)
	n2 := scrollWrap(b2, self.state.L2, tick)

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
	if len(self.state.L1) > 0 {
		self.dev.CursorYX(1, 1)
		self.dev.Write(b1[:n1])
	}
	// no padding: "erase" modified area, for now - whole line
	if n2 < self.width {
		self.dev.CursorYX(2, 1)
		self.dev.Write(spaceBytes[:self.width])
	}
	if len(self.state.L2) > 0 {
		self.dev.CursorYX(2, 1)
		self.dev.Write(b2[:n2])
	}

	if self.upd != nil {
		self.upd <- self.state.Copy()
	}
}

type State struct {
	L1, L2 []byte
}

func (s *State) Clear() {
	s.L1 = nil
	s.L2 = nil
}

func (s State) Copy() State {
	return State{
		L1: append([]byte(nil), s.L1...),
		L2: append([]byte(nil), s.L2...),
	}
}

func (s State) Format(width uint32) string {
	return fmt.Sprintf("%s\n%s",
		PadSpace(s.L1, width),
		PadSpace(s.L2, width),
	)
}

func (s State) String() string {
	return fmt.Sprintf("%s\n%s", s.L1, s.L2)
}

func PadSpace(b []byte, width uint32) []byte {
	l := uint32(len(b))

	if l == 0 {
		return spaceBytes[:width]
	}
	if l >= width {
		return b
	}
	buf := make([]byte, 0, width)
	buf = append(append(buf, b...), spaceBytes[:width-l]...)
	return buf
}

// relies that len(buf) == display width
func scrollWrap(buf []byte, content []byte, tick uint32) uint32 {
	length := uint32(len(content))
	width := uint32(len(buf))
	gap := uint32(width / 2)
	n := 0
	if length <= width {
		n = copy(buf, content)
		copy(buf[n:], spaceBytes)
		return uint32(n)
	}

	offset := tick % (length + gap)
	if offset < length {
		n = copy(buf, content[offset:])
	} else {
		gap = gap - (offset - length)
	}
	n += copy(buf[n:], spaceBytes[:gap])
	n += copy(buf[n:], content[0:])
	return uint32(n)
}
