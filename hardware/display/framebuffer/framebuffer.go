package framebuffer

//go:generate sh -ec "go tool cgo -godefs _defs.go >defs.gen.go && go fmt ."

import (
	"encoding/binary"
	"image"
	"image/color"
	"os"
	"syscall"
	"unsafe"

	"github.com/juju/errors"
)

type Framebuffer struct {
	buf    []byte
	dev    *os.File
	finfo  fixedScreenInfo
	vinfo  variableScreenInfo
}

func New(dev string) (*Framebuffer, error) {
	devFile, err := os.OpenFile(dev, os.O_RDWR, os.ModeDevice)
	if err != nil {
		return nil, errors.Annotate(err, "open")
	}
	fb := &Framebuffer{dev: devFile}
	fd := fb.dev.Fd()

	if err = ioctl(fd, getFixedScreenInfo, uintptr(unsafe.Pointer(&fb.finfo))); err != nil {
		fb.dev.Close()
		return nil, errors.Annotate(err, "getFixedScreenInfo")
	}

	if err = ioctl(fd, getVariableScreenInfo, uintptr(unsafe.Pointer(&fb.vinfo))); err != nil {
		fb.dev.Close()
		return nil, errors.Annotate(err, "getVariableScreenInfo")
	}

	fb.buf = make([]byte, fb.vinfo.Xres*fb.vinfo.Yres*(fb.vinfo.Bits_per_pixel/8))

	return fb, nil
}

func (fb *Framebuffer) Close() {
	fb.dev.Close()
}

func (fb *Framebuffer) Flush() error {
	_, err := fb.dev.WriteAt(fb.buf, 0)
	return err
}

func (fb *Framebuffer) Size() image.Point {
	return image.Point{X: int(fb.vinfo.Xres), Y: int(fb.vinfo.Yres)}
}

// Sets all pixels in internal buffer, call Flush() to write to hardware.
func (fb *Framebuffer) Update(cs []color.RGBA) error {
	cs = cs[:fb.vinfo.Xres*fb.vinfo.Yres]
	wordSize := fb.vinfo.Bits_per_pixel / 8
	switch {
	case fb.vinfo.Red == rgb565.Red && fb.vinfo.Green == rgb565.Green && fb.vinfo.Blue == rgb565.Blue:
		for i, c := range cs {
			offset := uint32(i) * wordSize
			word := encode565(c)
			binary.BigEndian.PutUint16(fb.buf[offset:], uint16(word))
		}
		return nil

	default:
		return errors.NotSupportedf("color model")
	}
}

var rgb565 = variableScreenInfo{
	Red:   bitField{Offset: 11, Length: 5, Right: 0},
	Green: bitField{Offset: 5, Length: 6, Right: 0},
	Blue:  bitField{Offset: 0, Length: 5, Right: 0},
}

func encode565(c color.RGBA) uint16 {
	return (uint16(c.R) & 0xf8 << 8) | (uint16(c.G) & 0xfc << 3) | (uint16(c.B) & 0xf8 >> 3)
}

func ioctl(fd uintptr, cmd uintptr, data uintptr) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, cmd, data); errno != 0 {
		return os.NewSyscallError("ioctl", errno)
	}
	return nil
}
