package money

import (
	"context"
	"log"
	"sync"

	"github.com/temoto/alive"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/mdb/bill"
)

const (
	InternalScalingFactor = 100
)

type BillState struct {
	lk    sync.Mutex
	alive *alive.Alive
	// TODO bank   currency.NominalGroup
	// TODO escrow currency.NominalGroup
	hw bill.BillValidator
}

func (self *BillState) Init(ctx context.Context, m mdb.Mdber) error {
	self.lk.Lock()
	defer self.lk.Unlock()

	log.Printf("head/money/bill init")
	self.alive = alive.NewAlive()
	self.alive.Add(1)
	pch := make(chan bill.PollResult, 2)
	if err := self.hw.Init(ctx, m); err != nil {
		return err
	}
	go self.hw.Run(ctx, self.alive, pch)
	go self.pollResultLoop(pch)
	return nil
}

func (self *BillState) Stop(ctx context.Context) {
	self.alive.Stop()
	self.alive.Wait()
}

func (self *BillState) pollResultLoop(pch <-chan bill.PollResult) {
	for pr := range pch {
		// translate PollResult Status items into actions
		doRestartTransaction := false
		doRestartSubsystem := false
		for _, pi := range pr.Items {
			switch pi.Status {
			case bill.StatusInfo:
				log.Printf("bill info: %s", pi.String())
				// TODO telemetry
			case bill.StatusError:
				log.Printf("bill error: %s", pi.Error)
				// TODO telemetry
				doRestartTransaction = true
			case bill.StatusFatal:
				log.Printf("bill error: %s", pi.Error)
				// TODO telemetry
				doRestartTransaction = true
				doRestartSubsystem = true
			case bill.StatusRejected, bill.StatusBusy, bill.StatusDisabled:
				// TODO telemetry
			case bill.StatusEscrow:
				// TODO self.hw.EscrowAccept / Reject
			case bill.StatusCredit:
				events <- Event{created: pr.Time, name: EventCredit, amount: currency.Amount(pi.Nominal)}
			case bill.StatusWasReset:
				self.hw.InitSequence()
			}
		}
		if doRestartTransaction {
			// TODO
		}
		if doRestartSubsystem {
			self.hw.CommandReset()
		}
	}
}
