package money

import (
	"context"
	"log"

	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/mdb/bill"
	"github.com/temoto/vender/hardware/money"
)

type BillState struct {
	alive *alive.Alive
	// TODO escrow currency.NominalGroup
	hw    bill.BillValidator
	state string
}

func (self *BillState) Init(ctx context.Context, parent *MoneySystem, m mdb.Mdber) error {
	log.Printf("head/money/bill init")
	if err := self.hw.Init(ctx, m); err != nil {
		return err
	}
	self.Start(ctx, parent)
	return nil
}

func (self *BillState) Start(ctx context.Context, parent *MoneySystem) {
	switch self.state {
	case "", "stopped": // OK
	case "starting", "running":
		log.Printf("double start, not a biggie")
		return
	default:
		panic("invalid state transition")
	}
	self.state = "starting"

	if err := self.hw.DoConfigBills.Do(ctx); err != nil {
		log.Printf("err=%v", errors.ErrorStack(err))
		self.state = "err"
		return
	}

	pch := make(chan money.PollResult, 2)
	self.alive = alive.NewAlive()
	self.alive.Add(1)
	go self.hw.Run(ctx, self.alive, pch)
	go self.pollResultLoop(ctx, parent, pch)
	go func() {
		<-self.alive.WaitChan()
		close(pch)
	}()
	self.state = "running"
}

func (self *BillState) Stop(ctx context.Context) {
	switch self.state {
	case "running": // OK
	case "", "stopped":
		log.Printf("double stop, not a biggie")
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

func (self *BillState) pollResultLoop(ctx context.Context, m *MoneySystem, pch <-chan money.PollResult) {
	const logPrefix = "head/money/bill"
	h := func(m *MoneySystem, pr *money.PollResult, pi money.PollItem) bool {
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
			return false
		}
		return true
	}
	genericPollResultLoop(ctx, m, pch, h, self.hw.NewRestarter(), logPrefix)
}
