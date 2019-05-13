package evend

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/temoto/vender/engine"
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
		d := new(DeviceEspresso)
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

	e := engine.GetEngine(ctx)
	e.RegisterNewSeq("@cup_dispense",
		e.MustResolveOrLazy("mdb.evend.conveyor_move_cup"),
		e.MustResolveOrLazy("mdb.evend.cup_dispense_proper"),
	)
	e.RegisterNewSeq("@cup_serve",
		e.MustResolveOrLazy("@ponr"),
		e.MustResolveOrLazy("mdb.evend.elevator_move_cup"),
		e.MustResolveOrLazy("mdb.evend.conveyor_move_elevator"),
		e.MustResolveOrLazy("mdb.evend.elevator_move_conveyor"),
		e.MustResolveOrLazy("mdb.evend.conveyor_move_cup"),
		e.MustResolveOrLazy("mdb.evend.elevator_move_ready"),
	)
	for i := 1; i <= Nhoppers; i++ {
		e.RegisterNewSeq(fmt.Sprintf("@hopper%d(?)", i),
			e.MustResolveOrLazy(fmt.Sprintf("mdb.evend.conveyor_move_hopper(%d)", i)),
			e.MustResolveOrLazy(fmt.Sprintf("mdb.evend.hopper%d_run(?)", i)),
		)
	}
	for _, kind := range []string{"cold", "hot"} {
		e.RegisterNewSeq(fmt.Sprintf("@water_%s(?)", kind),
			e.MustResolveOrLazy("mdb.evend.conveyor_move_cup"),
			e.MustResolveOrLazy(fmt.Sprintf("mdb.evend.valve_pour_%s(?)", kind)),
		)
	}
	e.RegisterNewSeq("@espresso(?)",
		e.MustResolveOrLazy("mdb.evend.conveyor_move_cup"),
		e.MustResolveOrLazy("mdb.evend.espresso_grind"),
		e.MustResolveOrLazy("mdb.evend.espresso_press"),
		e.MustResolveOrLazy("mdb.evend.valve_pour_coffee(?)"),
		e.MustResolveOrLazy("mdb.evend.espresso_dispose"),
	)
}
func enumIgnore(d interface{}) {}
