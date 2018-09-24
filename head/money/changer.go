package money

import (
	"context"
	"log"
	"sync"

	"github.com/temoto/alive"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/mdb/coin"
)

type ChangerState struct {
	lk    sync.Mutex
	alive *alive.Alive
	// TODO bank  currency.NominalGroup
	hw coin.CoinAcceptor
}

func (self *ChangerState) Init(ctx context.Context, m mdb.Mdber) error {
	self.lk.Lock()
	defer self.lk.Unlock()

	log.Printf("head/money/changer init")
	self.alive = alive.NewAlive()
	self.alive.Add(1)
	pch := make(chan coin.PollResult, 2)
	if err := self.hw.Init(ctx, m); err != nil {
		return err
	}
	go self.hw.Run(ctx, self.alive, pch)
	go self.pollResultLoop(pch)
	return nil
}

func (self *ChangerState) Stop(ctx context.Context) {
	self.alive.Stop()
	self.alive.Wait()
}

func (self *ChangerState) pollResultLoop(pch <-chan coin.PollResult) {
	for pr := range pch {
		// translate PollResult Status items into actions
		doRestartTransaction := false
		doRestartSubsystem := false
		for _, pi := range pr.Items {
			switch pi.Status {
			case coin.StatusInfo:
				log.Printf("coin info: %s", pi.String())
				// TODO telemetry
			case coin.StatusError:
				log.Printf("coin error: %s", pi.Error)
				// TODO telemetry
				doRestartTransaction = true
			case coin.StatusFatal:
				log.Printf("coin error: %s", pi.Error)
				// TODO telemetry
				doRestartTransaction = true
				doRestartSubsystem = true
			case coin.StatusRejected, coin.StatusWasReset:
				// TODO telemetry
			case coin.StatusSlugs:
				// TODO telemetry
			case coin.StatusDeposited:
				events <- Event{created: pr.Time, name: EventCredit, amount: currency.Amount(pi.Nominal)}
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
