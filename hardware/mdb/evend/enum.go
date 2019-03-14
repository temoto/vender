package evend

import (
	"context"
	"strconv"
	"sync"
)

// Init, register devices
func Enum(ctx context.Context, fun func(d interface{})) {
	const Nhoppers = 8

	wg := sync.WaitGroup{}
	wg.Add(6 + Nhoppers)

	if fun == nil {
		fun = enumIgnore
	}

	go func() {
		d := new(DeviceCoffee)
		if err := d.Init(ctx); err == nil {
			fun(d)
		}
		wg.Done()
	}()

	go func() {
		d := new(DeviceConveyor)
		if err := d.Init(ctx); err == nil {
			fun(d)
		}
		wg.Done()
	}()

	go func() {
		d := new(DeviceCup)
		if err := d.Init(ctx); err == nil {
			fun(d)
		}
		wg.Done()
	}()

	go func() {
		d := new(DeviceElevator)
		if err := d.Init(ctx); err == nil {
			fun(d)
		}
		wg.Done()
	}()

	for i := 1; i <= Nhoppers; i++ {
		i := i
		go func() {
			d := new(DeviceHopper)
			addr := uint8(0x40 + (i-1)*8)
			if err := d.Init(ctx, addr, strconv.Itoa(i)); err == nil {
				fun(d)
			}
			wg.Done()
		}()
	}

	go func() {
		d := new(DeviceMixer)
		if err := d.Init(ctx); err == nil {
			fun(d)
		}
		wg.Done()
	}()

	go func() {
		d := new(DeviceValve)
		if err := d.Init(ctx); err == nil {
			fun(d)
		}
		wg.Done()
	}()

	wg.Wait()
}
func enumIgnore(d interface{}) {}
