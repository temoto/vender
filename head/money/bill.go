package money

import (
	"context"

	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/vender/hardware/mdb/bill"
	"github.com/temoto/vender/hardware/money"
	"github.com/temoto/vender/log2"
)

type BillState struct {
	Log   *log2.Log
	alive *alive.Alive
	// TODO escrow currency.NominalGroup
	hw    bill.BillValidator
	state string
}

func (self *BillState) Init(ctx context.Context, parent *MoneySystem) error {
	self.Log = parent.Log
	self.Log.Debugf("head/money/bill init")
	if err := self.hw.Init(ctx); err != nil {
		return err
	}
	self.Start(ctx, parent)
	return nil
}

func (self *BillState) Start(ctx context.Context, parent *MoneySystem) {
	switch self.state {
	case "", "stopped": // OK
	case "starting", "running":
		self.Log.Debugf("double start, not a biggie")
		return
	default:
		panic("invalid state transition")
	}
	self.state = "starting"

	if err := self.hw.DoConfigBills.Do(ctx); err != nil {
		self.Log.Errorf("err=%v", errors.ErrorStack(err))
		self.state = "err"
		return
	}

	self.alive = alive.NewAlive()
	go self.hw.Run(ctx, self.alive, func(pi money.PollItem) { self.handlePollItem(ctx, parent, pi) })
	self.state = "running"
}

func (self *BillState) Stop(ctx context.Context) {
	switch self.state {
	case "running": // OK
	case "", "stopped":
		self.Log.Debugf("double stop, not a biggie")
		return
	default:
		panic("invalid state transition")
	}
	self.state = "stopping"
	self.hw.CommandBillType(0, 0)
	self.alive.Stop()
	self.alive.Wait()
	self.state = "stopped"
}

func (self *BillState) handlePollItem(ctx context.Context, m *MoneySystem, pi money.PollItem) {
	const logPrefix = "head/money/bill"

	switch pi.Status {
	case money.StatusRejected:
	case money.StatusDisabled:
		// TODO telemetry
	case money.StatusEscrow:
		// TODO self.hw.EscrowAccept / Reject
	case money.StatusWasReset:
		self.hw.DoIniter.Do(ctx)
	case money.StatusBusy:
	default:
		m.handleGenericPollItem(ctx, pi, logPrefix)
	}
}
