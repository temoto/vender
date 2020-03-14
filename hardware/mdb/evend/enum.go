package evend

import (
	"context"
	"strconv"
	"sync"

	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/internal/state"
)

// Register devices
func Enum(ctx context.Context) error {
	const Nhoppers = 8
	const N = 7 + Nhoppers

	g := state.GetGlobal(ctx)
	wg := sync.WaitGroup{}
	wg.Add(N)
	errch := make(chan error, N)

	// TODO dev.init() without IO, then g.RegisterDevice(dev.Name, dev, dev.Probe)

	go helpers.WrapErrChan(&wg, errch, func() error {
		dev := &DeviceConveyor{}
		return g.RegisterDevice("evend.conveyor", dev, func() error { return dev.init(ctx) })
	})

	go helpers.WrapErrChan(&wg, errch, func() error {
		dev := &DeviceCup{}
		return g.RegisterDevice("evend.cup", dev, func() error { return dev.init(ctx) })
	})

	go helpers.WrapErrChan(&wg, errch, func() error {
		dev := &DeviceElevator{}
		return g.RegisterDevice("evend.elevator", dev, func() error { return dev.init(ctx) })
	})

	go helpers.WrapErrChan(&wg, errch, func() error {
		dev := &DeviceEspresso{}
		return g.RegisterDevice("evend.espresso", dev, func() error { return dev.init(ctx) })
	})

	for i := 1; i <= Nhoppers; i++ {
		i := i
		go helpers.WrapErrChan(&wg, errch, func() error {
			dev := &DeviceHopper{}
			addr := uint8(0x40 + (i-1)*8)
			suffix := strconv.Itoa(i)
			return g.RegisterDevice("evend.hopper"+suffix, dev, func() error { return dev.init(ctx, addr, suffix) })
		})
	}

	go helpers.WrapErrChan(&wg, errch, func() error {
		dev := &DeviceMixer{}
		return g.RegisterDevice("evend.mixer", dev, func() error { return dev.init(ctx) })
	})

	go helpers.WrapErrChan(&wg, errch, func() error {
		dev := &DeviceMultiHopper{}
		return g.RegisterDevice("evend.multihopper", dev, func() error { return dev.init(ctx) })
	})

	go helpers.WrapErrChan(&wg, errch, func() error {
		dev := &DeviceValve{}
		return g.RegisterDevice("evend.valve", dev, func() error { return dev.init(ctx) })
	})

	wg.Wait()
	close(errch)
	return helpers.FoldErrChan(errch)
}
