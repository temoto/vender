package i2c

// Thanks to
// https://github.com/kidoman/embd and https://bitbucket.org/gmcbay/i2c

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"github.com/juju/errors"
)

const (
	// as defined in /usr/include/linux/i2c-dev.h
	I2C_RETRIES     = 0x0701 /* number of times a device address should be polled when not acknowledging */
	I2C_TIMEOUT     = 0x0702 /* set timeout in units of 10 ms */
	I2C_SLAVE       = 0x0703 /* Use this slave address */
	I2C_FUNCS       = 0x0705 /* Get the adapter functionality mask */
	I2C_SLAVE_FORCE = 0x0706 /* Use this slave address, even if it is already in use by a driver! */
	I2C_RDWR        = 0x0707 /* Combined R/W transfer (one STOP only) */
	I2C_SMBUS       = 0x0720 /* SMBus transfer */

	// i2c_msg flags
	// as defined in /usr/include/linux/i2c.h
	I2C_M_RD           = 0x0001 /* read data, from slave to master */
	I2C_M_TEN          = 0x0010 /* this is a ten bit chip address */
	I2C_M_RECV_LEN     = 0x0400 /* length will be first received byte */
	I2C_M_NO_RD_ACK    = 0x0800 /* if I2C_FUNC_PROTOCOL_MANGLING */
	I2C_M_IGNORE_NAK   = 0x1000 /* if I2C_FUNC_PROTOCOL_MANGLING */
	I2C_M_REV_DIR_ADDR = 0x2000 /* if I2C_FUNC_PROTOCOL_MANGLING */
	I2C_M_NOSTART      = 0x4000 /* if I2C_FUNC_NOSTART */
	I2C_M_STOP         = 0x8000 /* if I2C_FUNC_PROTOCOL_MANGLING */
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
	Init() error
	Close() error
	Tx(addr byte, bw []byte, br []byte) error
}

func NewI2CBus(busNo byte) I2CBus {
	return &i2cBus{busNo: busNo}
}

func (b *i2cBus) Init() error {
	b.lk.Lock()
	defer b.lk.Unlock()
	return b.init()
}

func (b *i2cBus) init() error {
	if b.initialized {
		return nil
	}

	var err error
	if b.file, err = os.OpenFile(fmt.Sprintf("/dev/i2c-%d", b.busNo), os.O_RDWR, os.ModeExclusive); err != nil {
		return err
	}
	b.initialized = true

	return nil
}

func (b *i2cBus) Tx(addr byte, bw []byte, br []byte) error {
	b.lk.Lock()
	defer b.lk.Unlock()

	if err := b.init(); err != nil {
		return err
	}

	nmsg := uint32(0)
	msgs := [2]i2c_msg{}
	if bw != nil {
		msgs[nmsg] = i2c_msg{
			addr: uint16(addr), flags: 0,
			buf: uintptr(unsafe.Pointer(&bw[0])), len: uint16(len(bw)),
		}
		nmsg++
	}
	if br != nil {
		msgs[nmsg] = i2c_msg{
			addr: uint16(addr), flags: I2C_M_RD,
			buf: uintptr(unsafe.Pointer(&br[0])), len: uint16(len(br)),
		}
		nmsg++
	}
	if nmsg == 0 {
		return errors.Errorf("i2cBus.Tx both bw=br=nil nothing to do")
	}

	rdwr_data := i2c_rdwr_ioctl_data{
		msgs: uintptr(unsafe.Pointer(&msgs[0])),
		nmsg: nmsg,
	}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL,
		uintptr(b.file.Fd()), uintptr(I2C_RDWR), uintptr(unsafe.Pointer(&rdwr_data)))
	if errno != 0 {
		return syscall.Errno(errno)
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
