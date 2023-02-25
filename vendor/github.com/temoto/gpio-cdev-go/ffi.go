package gpio

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// struct gpiochip_info - Information about a certain GPIO chip
type ChipInfo struct {
	// the Linux kernel name of this GPIO chip
	Name [32]byte

	// a functional name for this GPIO chip, such as a product number, may be NULL
	Label [32]byte

	// number of GPIO lines on this chip
	Lines uint32
}

type LineFlag uint32

const (
	GPIOLINE_FLAG_KERNEL      LineFlag = 1 << 0 /* Line used by the kernel */
	GPIOLINE_FLAG_IS_OUT      LineFlag = 1 << 1
	GPIOLINE_FLAG_ACTIVE_LOW  LineFlag = 1 << 2
	GPIOLINE_FLAG_OPEN_DRAIN  LineFlag = 1 << 3
	GPIOLINE_FLAG_OPEN_SOURCE LineFlag = 1 << 4
)

// struct gpioline_info - Information about a certain GPIO line
type LineInfo struct {
	// the local offset on this GPIO device, fill this in when
	// requesting the line information from the kernel
	LineOffset uint32

	Flags LineFlag

	// the name of this GPIO line, such as the output pin of the line on the
	// chip, a rail or a pin header name on a board, as specified by the gpio
	// chip, may be NULL
	Name [32]byte

	// a functional name for the consumer of this GPIO line as set by
	// whatever is using it, will be NULL if there is no current user but may
	// also be NULL if the consumer doesn't set this up
	Consumer [32]byte
}

const GPIOHANDLES_MAX = 64

type RequestFlag uint32

const (
	GPIOHANDLE_REQUEST_INPUT       RequestFlag = 1 << 0
	GPIOHANDLE_REQUEST_OUTPUT      RequestFlag = 1 << 1
	GPIOHANDLE_REQUEST_ACTIVE_LOW  RequestFlag = 1 << 2
	GPIOHANDLE_REQUEST_OPEN_DRAIN  RequestFlag = 1 << 3
	GPIOHANDLE_REQUEST_OPEN_SOURCE RequestFlag = 1 << 4
)

// struct gpiohandle_request - Information about a GPIO handle request
type HandleRequest struct {
	// an array of desired lines, specified by offset index for the associated GPIO device
	LineOffsets [GPIOHANDLES_MAX]uint32

	// desired flags for the desired GPIO lines, such as
	// GPIOHANDLE_REQUEST_OUTPUT, GPIOHANDLE_REQUEST_ACTIVE_LOW etc, OR:ed
	// together. Note that even if multiple lines are requested, the same flags
	// must be applicable to all of them, if you want lines with individual
	// flags set, request them one by one. It is possible to select
	// a batch of input or output lines, but they must all have the same
	// characteristics, i.e. all inputs or all outputs, all active low etc
	Flags RequestFlag

	// if the GPIOHANDLE_REQUEST_OUTPUT is set for a requested
	// line, this specifies the default output value, should be 0 (low) or
	// 1 (high), anything else than 0 or 1 will be interpreted as 1 (high)
	// @consumer_label: a desired consumer label for the selected GPIO line(s)
	// such as "my-bitbanged-relay"
	DefaultValues [GPIOHANDLES_MAX]byte

	// a desired consumer label for the selected GPIO line(s)
	// such as "my-bitbanged-relay"
	ConsumerLabel [32]byte

	// number of lines requested in this request, i.e. the number of
	// valid fields in the above arrays, set to 1 to request a single line
	Lines uint32

	// if successful this field will contain a valid anonymous file handle
	// after a GPIO_GET_LINEHANDLE_IOCTL operation, zero or negative value
	// means error
	Fd int32
}

// struct gpiohandle_data - Information of values on a GPIO handle
type HandleData struct {
	// GET: contains the current state of a line,
	// SET: the desired target state
	Values [GPIOHANDLES_MAX]byte
}

type EventFlag uint32

const (
	GPIOEVENT_REQUEST_RISING_EDGE  EventFlag = 1 << 0
	GPIOEVENT_REQUEST_FALLING_EDGE EventFlag = 1 << 1
	GPIOEVENT_REQUEST_BOTH_EDGES   EventFlag = (1 << 0) | (1 << 1)
)

// struct gpioevent_request - Information about a GPIO event request

type EventRequest struct {
	// the desired line to subscribe to events from, specified by
	// offset index for the associated GPIO device
	LineOffset uint32

	// desired handle flags for the desired GPIO line, such as
	// GPIOHANDLE_REQUEST_ACTIVE_LOW or GPIOHANDLE_REQUEST_OPEN_DRAIN
	RequestFlags RequestFlag

	// desired flags for the desired GPIO event line, such as
	// GPIOEVENT_REQUEST_RISING_EDGE or GPIOEVENT_REQUEST_FALLING_EDGE
	EventFlags EventFlag

	// a desired consumer label for the selected GPIO line(s)
	// such as "my-listener"
	ConsumerLabel [32]byte

	// if successful this field will contain a valid anonymous file handle
	// after a GPIO_GET_LINEEVENT_IOCTL operation, zero or negative value means error
	Fd int32
}

type EventID uint32

const (
	GPIOEVENT_EVENT_RISING_EDGE  = 0x01
	GPIOEVENT_EVENT_FALLING_EDGE = 0x02
)

// struct gpioevent_data - The actual event being pushed to userspace
type EventData struct {
	// best estimate of time of event occurrence, in nanoseconds
	Timestamp uint64

	// event identifier (e.g. rising/falling edge)
	ID EventID

	_pad uint32 //lint:ignore U1000 .
}

func RawGetChipInfo(fd int, arg *ChipInfo) error {
	return ioctl(fd, GPIO_GET_CHIPINFO_IOCTL, uintptr(unsafe.Pointer(arg)))
}

func RawGetLineInfo(fd int, arg *LineInfo) error {
	return ioctl(fd, GPIO_GET_LINEINFO_IOCTL, uintptr(unsafe.Pointer(arg)))
}

func RawGetLineHandle(fd int, arg *HandleRequest) error {
	return ioctl(fd, GPIO_GET_LINEHANDLE_IOCTL, uintptr(unsafe.Pointer(arg)))
}

func RawGetLineEvent(fd int, arg *EventRequest) error {
	return ioctl(fd, GPIO_GET_LINEEVENT_IOCTL, uintptr(unsafe.Pointer(arg)))
}

func RawGetLineValues(fd int, arg *HandleData) error {
	return ioctl(fd, GPIOHANDLE_GET_LINE_VALUES_IOCTL, uintptr(unsafe.Pointer(arg)))
}

func RawSetLineValues(fd int, arg *HandleData) error {
	return ioctl(fd, uintptr(GPIOHANDLE_SET_LINE_VALUES_IOCTL), uintptr(unsafe.Pointer(arg)))
}

func ioctl(fd int, op, arg uintptr) error {
retry:
	r, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), op, arg)
	if errno == syscall.EINTR {
		goto retry
	}
	if errno != 0 {
		err := os.NewSyscallError("SYS_IOCTL", errno)
		// log.Printf("ioctl fd=%d op=%x arg=%x err=%v", fd, op, arg, err)
		return err
	} else if r != 0 {
		err := fmt.Errorf("SYS_IOCTL r=%d", r)
		// log.Printf("ioctl fd=%d op=%x arg=%x err=%v", fd, op, arg, err)
		return err
	}
	return nil
}
