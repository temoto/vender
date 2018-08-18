package money

import (
	"context"
	"log"
	"sync"

	"github.com/temoto/alive"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/mdb/bill"
)

const (
	InternalScalingFactor = 100
)

type BillState struct {
	lk     sync.Mutex
	alive  *alive.Alive
	bank   currency.NominalGroup
	escrow currency.NominalGroup
	hw     bill.BillValidator
}

func (self *BillState) Init(ctx context.Context) error {
	self.lk.Lock()
	defer self.lk.Unlock()

	log.Printf("bs init")
	if err := self.hw.Init(ctx); err != nil {
		return err
	}

	self.alive = alive.NewAlive()
	self.alive.Add(1)
	pch := make(chan bill.PollResult, 2)
	go self.hw.Loop(ctx, self.alive, pch)
	go func() {
		for pr := range pch {
			for _, pi := range pr.Items {
				switch pi.Status {
				case bill.StatusInfo:
					log.Printf("bill info: %s", pi.String())
					// TODO telemetry
				case bill.StatusError:
					log.Printf("bill error: %s", pi.Error)
					// TODO telemetry
					// TODO restart transaction
				case bill.StatusFatal:
					log.Printf("bill error: %s", pi.Error)
					// TODO telemetry
					// TODO restart transaction
					// TODO restart money subsystem
				case bill.StatusRejected, bill.StatusBusy, bill.StatusWasReset, bill.StatusDisabled:
					// TODO telemetry
				case bill.StatusEscrow:
					// TODO self.hw.EscrowAccept / Reject
				case bill.StatusCredit:
					events <- Event{created: pr.Time, name: "credit", amount: currency.Amount(pi.Nominal)}
				}
			}
		}
	}()
	return nil
}

func (self *BillState) Stop(ctx context.Context) {
	self.alive.Stop()
	self.alive.Wait()
}
