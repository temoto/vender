package hardware

import (
	"context"
	"sync"

	"github.com/temoto/vender/hardware/mdb/bill"
	"github.com/temoto/vender/hardware/mdb/coin"
	"github.com/temoto/vender/hardware/mdb/evend"
)

func Enum(ctx context.Context, fun func(d interface{})) {
	wg := sync.WaitGroup{}
	wg.Add(3)

	go func() {
		bill.Enum(ctx, fun)
		wg.Done()
	}()
	go func() {
		coin.Enum(ctx, fun)
		wg.Done()
	}()
	go func() {
		evend.Enum(ctx, fun)
		wg.Done()
	}()

	wg.Wait()
}
