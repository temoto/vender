package gpio

// From <include/uapi/linux/gpio.h>
// tested to be same on 386,arm,arm64,amd64
// see ioctl_linux_test.go to see how they were generated
const (
	GPIO_GET_CHIPINFO_IOCTL          uintptr = 0x8044b401
	GPIO_GET_LINEINFO_IOCTL          uintptr = 0xc048b402
	GPIO_GET_LINEHANDLE_IOCTL        uintptr = 0xc16cb403
	GPIO_GET_LINEEVENT_IOCTL         uintptr = 0xc030b404
	GPIOHANDLE_GET_LINE_VALUES_IOCTL uintptr = 0xc040b408
	GPIOHANDLE_SET_LINE_VALUES_IOCTL uintptr = 0xc040b409
)
