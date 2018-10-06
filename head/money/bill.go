package money

import (
	"context"
	"log"
	"sync"

	"github.com/temoto/alive"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/mdb/bill"
	"github.com/temoto/vender/hardware/money"
)

const (
	InternalScalingFactor = 100
)

type BillState struct {
	lk    sync.Mutex
	alive *alive.Alive
	// TODO escrow currency.NominalGroup
	hw bill.BillValidator
}

func (self *BillState) Init(ctx context.Context, m mdb.Mdber) error {
	self.lk.Lock()
	defer self.lk.Unlock()

	log.Printf("head/money/bill init")
	self.alive = alive.NewAlive()
	self.alive.Add(1)
	pch := make(chan money.PollResult, 2)
	if err := self.hw.Init(ctx, m); err != nil {
		return err
	}
	go self.hw.Run(ctx, self.alive, pch)
	go self.pollResultLoop(&Global, pch)
	return nil
}

func (self *BillState) Stop(ctx context.Context) {
	self.alive.Stop()
	self.alive.Wait()
}

func (self *BillState) pollResultLoop(m *MoneySystem, pch <-chan money.PollResult) {
	const logPrefix = "head/money/bill"
	h := func(m *MoneySystem, pr *money.PollResult, pi money.PollItem, hw Hardwarer) bool {
		switch pi.Status {
		case money.StatusRejected:
		case money.StatusDisabled:
			// TODO telemetry
		case money.StatusEscrow:
			// TODO self.hw.EscrowAccept / Reject
		case money.StatusWasReset:
			self.hw.InitSequence()
		case money.StatusBusy:
		default:
			return false
		}
		return true
	}
	onRestart := func(m *MoneySystem, hw Hardwarer) {
		self.hw.CommandReset()
		// InitSequence will be executed as reaction to StatusWasReset
		// self.hw.InitSequence()
	}
	pollResultLoop(m, pch, h, onRestart, &self.hw, logPrefix)
}
