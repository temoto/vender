package hardware

import (
	"context"
	"sync"

	"github.com/temoto/vender/hardware/mdb/bill"
	"github.com/temoto/vender/hardware/mdb/coin"
	"github.com/temoto/vender/hardware/mdb/evend"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/state"
)

func Enum(ctx context.Context) error {
	const N = 3
	errch := make(chan error, N+1)
	wg := sync.WaitGroup{}
	wg.Add(N)

	go helpers.WrapErrChan(&wg, errch, func() error { return bill.Enum(ctx) })
	go helpers.WrapErrChan(&wg, errch, func() error { return coin.Enum(ctx) })
	go helpers.WrapErrChan(&wg, errch, func() error { return evend.Enum(ctx) })

	wg.Wait()
	errch <- state.GetGlobal(ctx).CheckDevices()
	close(errch)
	return helpers.FoldErrChan(errch)
}
