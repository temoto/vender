package gpio

// This code is Copyright (c) 2014 Mark Wolfe and licenced under the MIT licence. All rights not explicitly granted in the MIT license are reserved.
// https://github.com/paypal/gatt/commit/ffdee90ddb4ade889d993e0fd82afcc47fe65c4d#diff-6580ad406a4f6ab990f69b982bdc945a

const (
	typeBits   = 8
	numberBits = 8
	sizeBits   = 14

	directionNone  = 0
	directionWrite = 1
	directionRead  = 2

	numberShift    = 0
	typeShift      = numberShift + numberBits
	sizeShift      = typeShift + typeBits
	directionShift = sizeShift + sizeBits
)

func ioc(dir, t, nr, size uintptr) uintptr {
	return (dir << directionShift) | (t << typeShift) | (nr << numberShift) | (size << sizeShift)
}

// ioNone used for a simple ioctl that sends nothing but the type and number, and receives back nothing but an (integer) retval.
// unused func ioNone(t, nr uintptr) uintptr { return ioc(directionNone, t, nr, 0) }

// ioR used for an ioctl that reads data from the device driver. The driver will be allowed to return sizeof(data_type) bytes to the user.
func ioR(t, nr, size uintptr) uintptr {
	return ioc(directionRead, t, nr, size)
}

// ioW used for an ioctl that writes data to the device driver.
// unused func ioW(t, nr, size uintptr) uintptr { return ioc(directionWrite, t, nr, size) }

// ioWR  a combination of ioR and ioW. That is, data is both written to the driver and then read back from the driver by the client.
func ioWR(t, nr, size uintptr) uintptr {
	return ioc(directionRead|directionWrite, t, nr, size)
}
