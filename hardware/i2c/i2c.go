package i2c

// Thanks to
// https://github.com/kidoman/embd and https://bitbucket.org/gmcbay/i2c

import (
	"fmt"
	"log"
	"os"
	"sync"
	"syscall"
	"time"
)

const (
	delay = 20 * time.Microsecond
)

const (
	// as defined in /usr/include/linux/i2c-dev.h
	I2C_SLAVE = 0x0703
	I2C_SMBUS = 0x0720
	// as defined in /usr/include/linux/i2c.h
	I2C_SMBUS_WRITE          = 0
	I2C_SMBUS_READ           = 1
	I2C_SMBUS_I2C_BLOCK_DATA = 8
	I2C_SMBUS_BLOCK_MAX      = 32
)

type i2c_msg struct {
	addr  uint16
	flags uint16
	len   uint16
	buf   uintptr
}

type i2c_rdwr_ioctl_data struct {
	msgs uintptr
	nmsg uint32
}

type i2cBus struct {
	busNo       byte
	file        *os.File
	addr        byte
	lk          sync.Mutex
	initialized bool
}

// I2CBus interface is used to interact with the I2C bus.
type I2CBus interface {
	ReadByteAt(addr byte) (value byte, err error)
	WriteByteAt(addr, value byte) error

	ReadBytesAt(addr byte, buf []byte) (n int, err error)
	WriteBytesAt(addr byte, buf []byte) error

	Close() error
}

func NewI2CBus(busNo byte) I2CBus {
	return &i2cBus{busNo: busNo}
}

func (b *i2cBus) init() error {
	if b.initialized {
		return nil
	}

	var err error
	if b.file, err = os.OpenFile(fmt.Sprintf("/dev/i2c-%d", b.busNo), os.O_RDWR, os.ModeExclusive); err != nil {
		return err
	}
	log.Printf("i2c: bus %v initialized", b.busNo)
	b.initialized = true

	return nil
}

func (b *i2cBus) setAddress(addr byte) error {
	if addr != b.addr {
		log.Printf("i2c: setting bus %v address to %#02x", b.busNo, addr)
		if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, b.file.Fd(), I2C_SLAVE, uintptr(addr)); errno != 0 {
			return syscall.Errno(errno)
		}

		b.addr = addr
	}

	return nil
}

func (b *i2cBus) ReadByteAt(addr byte) (byte, error) {
	buf := []byte{0}
	n, err := b.ReadBytesAt(addr, buf)
	if err != nil {
		return 0, err
	}
	if n != 1 {
		return 0, fmt.Errorf("i2c: Unexpected number (%v) of bytes read", n)
	}
	return buf[0], nil
}

func (b *i2cBus) ReadBytesAt(addr byte, buf []byte) (n int, err error) {
	b.lk.Lock()
	defer b.lk.Unlock()

	if err = b.init(); err != nil {
		return 0, err
	}
	if err = b.setAddress(addr); err != nil {
		return 0, err
	}
	n, _ = b.file.Read(buf)

	return n, nil
}

func (b *i2cBus) WriteByteAt(addr, value byte) error {
	return b.WriteBytesAt(addr, []byte{value})
}

func (b *i2cBus) WriteBytesAt(addr byte, buf []byte) (err error) {
	b.lk.Lock()
	defer b.lk.Unlock()

	if err = b.init(); err != nil {
		return err
	}
	if err = b.setAddress(addr); err != nil {
		return err
	}
	_, err = b.file.Write(buf)
	if err != nil {
		return err
	}

	return nil
}

func (b *i2cBus) Close() error {
	b.lk.Lock()
	defer b.lk.Unlock()

	if !b.initialized {
		return nil
	}

	return b.file.Close()
}
