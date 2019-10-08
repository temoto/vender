package mdb

import "fmt"

//go:generate stringer -type=DeviceState -trimprefix=Device
type DeviceState uint32

const (
	DeviceInvalid DeviceState = iota // new, not usable
	DeviceInited                     // after Init(), usable for Probe
	DeviceOnline                     // Probe success, calibration may be required
	DeviceReady                      // "ready for useful work", with RESET, configure, calibration done
	DeviceError                      // responds but doesn't work well
	DeviceOffline                    // does not respond
)

var (
	ErrOffline      = fmt.Errorf("offline")
	ErrStateInvalid = fmt.Errorf("CRITICAL code error state=invalid")
	ErrStateError   = fmt.Errorf("state=error")
)

func (s DeviceState) Ok() bool {
	switch s {
	case DeviceOnline, DeviceReady:
		return true
	}
	return false
}

func (s DeviceState) Online() bool {
	switch s {
	case DeviceOnline, DeviceReady, DeviceError:
		return true
	}
	return false
}
