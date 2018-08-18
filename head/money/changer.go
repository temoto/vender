package money

import (
	"context"
	"log"
	"sync"

	"github.com/temoto/alive"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/mdb/coin"
)

type ChangerState struct {
	lk    sync.Mutex
	alive *alive.Alive
	bank  currency.NominalGroup
	hw    coin.CoinAcceptor
}

func (self *ChangerState) Init(ctx context.Context) error {
	self.lk.Lock()
	defer self.lk.Unlock()

	log.Printf("cs init")
	if err := self.hw.Init(ctx); err != nil {
		return err
	}

	self.alive = alive.NewAlive()
	self.alive.Add(1)
	pch := make(chan coin.PollResult, 2)
	go self.hw.Loop(ctx, self.alive, pch)
	go func() {
		for pr := range pch {
			for _, pi := range pr.Items {
				switch pi.Status {
				case coin.StatusInfo:
					log.Printf("coin info: %s", pi.String())
					// TODO telemetry
				case coin.StatusError:
					log.Printf("coin error: %s", pi.Error)
					// TODO telemetry
					// TODO restart transaction
				case coin.StatusFatal:
					log.Printf("coin error: %s", pi.Error)
					// TODO telemetry
					// TODO restart transaction
					// TODO restart money subsystem
				case coin.StatusRejected, coin.StatusWasReset:
					// TODO telemetry
				case coin.StatusDeposited:
					events <- Event{created: pr.Time, name: "credit", amount: currency.Amount(pi.Nominal)}
				}
			}
		}
	}()
	return nil
}

func (self *ChangerState) Stop(ctx context.Context) {
	self.alive.Stop()
	self.alive.Wait()
}
