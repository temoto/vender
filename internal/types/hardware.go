package types

import "fmt"

type DeviceOfflineError struct {
	Device Devicer
}

func (oe DeviceOfflineError) Error() string { return fmt.Sprintf("%s is offline", oe.Device.Name()) }

type Devicer interface {
	Name() string
}
