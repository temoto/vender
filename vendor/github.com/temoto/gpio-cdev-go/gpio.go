package gpio

import (
	"fmt"
	"sync/atomic"
	"syscall"

	"github.com/juju/errors"
)

type chip struct {
	fa              fdArc
	defaultConsumer string
	closed          uint32
	info            ChipInfo
}

// The entry point to this library.
// `path` is likely "/dev/gpiochipN"
// `defaultConsumer` will be used in absence of more specific consumer label
//   to OpenLines/GetLineEvent.
// Makes two syscalls: open(path), ioctl(GET_CHIPINFO)
// You must call Chiper.Close()
func Open(path, defaultConsumer string) (Chiper, error) {
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	chip := &chip{
		fa:              newFdArc(fd),
		defaultConsumer: defaultConsumer,
	}
	// runtime.SetFinalizer(chip, func(c *chip) { c.Close() })
	err = RawGetChipInfo(chip.fa.fd, &chip.info)
	return chip, err
}

func (c *chip) Close() error {
	if c == nil {
		return nil
	}
	if atomic.AddUint32(&c.closed, 1) == 1 {
		c.fa.decref()
		return c.fa.wait()
	}
	return ErrClosed
}

func (c *chip) Info() ChipInfo { return c.info }

func (c *chip) LineInfo(line uint32) (LineInfo, error) {
	linfo := LineInfo{LineOffset: line}
	err := RawGetLineInfo(c.fa.fd, &linfo)
	return linfo, err
}

func (c *chip) OpenLines(flag RequestFlag, consumerLabel string, offsets ...uint32) (Lineser, error) {
	const tag = "GET_LINEHANDLE"
	if !c.fa.incref() {
		return nil, ErrClosed
	}

	req := HandleRequest{
		Flags: flag,
		Lines: uint32(len(offsets)),
	}
	copy(req.ConsumerLabel[:], []byte(consumerLabel))
	copy(req.LineOffsets[:], offsets)

	err := RawGetLineHandle(c.fa.fd, &req)
	if err != nil {
		c.fa.decref()
		err = errors.Annotate(err, tag)
		return nil, err
	}
	if req.Fd <= 0 {
		c.fa.decref()
		err = errors.Errorf("%s ioctl=success fd=%d", tag, req.Fd)
		return nil, err
	}

	lh := &lines{
		chip:  c,
		fd:    int(req.Fd),
		count: req.Lines,
	}
	copy(lh.offsets[:], req.LineOffsets[:])
	// runtime.SetFinalizer(lh, func(l *lines) { l.Close() })
	return lh, nil
}

func (self *ChipInfo) String() string {
	return fmt.Sprintf("name=%s label=%s lines=%d",
		cstr(self.Name[:]), cstr(self.Label[:]), self.Lines)
}

func (li *LineInfo) ConsumerString() string { return cstr(li.Consumer[:]) }
func (li *LineInfo) NameString() string     { return cstr(li.Name[:]) }

func (li *LineInfo) String() string {
	return fmt.Sprintf("line=%d flags=%x name=%s consumer=%s",
		li.LineOffset, li.Flags, li.NameString(), li.ConsumerString())
}

type lines struct {
	chip    *chip
	fd      int
	offsets [GPIOHANDLES_MAX]uint32
	values  [GPIOHANDLES_MAX]byte
	count   uint32
	closed  uint32
}

func (self *lines) Close() error {
	if atomic.AddUint32(&self.closed, 1) == 1 {
		err := syscall.Close(self.fd)
		self.chip.fa.decref()
		return err
	}
	return ErrClosed
}

// offset -> idx in self.lines/values
func (self *lines) mustFindLine(line uint32) int {
	for i, l := range self.offsets {
		if uint32(i) >= self.count {
			break
		}
		if l == line {
			return i
		}
	}
	panic(fmt.Sprintf("code error invalid line=%d registered=%v", line, self.LineOffsets()))
}

// Returns line setter func which only changes internal buffer.
// Use `.Flush()` to change hardware state of all lines.
func (self *lines) SetFunc(line uint32) LineSetFunc {
	idx := self.mustFindLine(line)
	return func(value byte) {
		self.values[idx] = value
	}
}

func (self *lines) LineOffsets() []uint32 {
	return self.offsets[:self.count]
}

func (self *lines) Read() (HandleData, error) {
	data := HandleData{}
	err := RawGetLineValues(self.fd, &data)
	return data, err
}

func (self *lines) Flush() error {
	data := HandleData{Values: self.values}
	return RawSetLineValues(self.fd, &data)
}

// Changes internal buffer only, use `.Flush()` to apply to hardware.
func (self *lines) SetBulk(bs ...byte) { copy(self.values[:], bs) }

func cstr(bs []byte) string {
	length := 0
	for _, b := range bs {
		if b == 0 {
			break
		}
		length++
	}
	return string(bs[:length])
}
