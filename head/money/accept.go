package money

import (
	"context"
	"time"

	"github.com/temoto/alive"
	"github.com/temoto/errors"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/money"
	"github.com/temoto/vender/head/tele"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/state"
)

func (self *MoneySystem) SetAcceptMax(ctx context.Context, limit currency.Amount) error {
	errs := []error{
		self.bill.AcceptMax(limit).Do(ctx),
		self.coin.AcceptMax(limit).Do(ctx),
	}
	err := helpers.FoldErrors(errs)
	if err != nil {
		err = errors.Annotatef(err, "SetAcceptMax limit=%s", limit.FormatCtx(ctx))
	}
	return err
}

func (self *MoneySystem) AcceptCredit(ctx context.Context, maxPrice currency.Amount, stopAccept <-chan struct{}, out chan<- Event) error {
	const tag = "money.accept-credit"

	config := state.GetGlobal(ctx).Config
	maxConfig := currency.Amount(config.Money.CreditMax)
	// Accept limit = lesser of: configured max credit or highest menu price.
	limit := maxConfig

	self.lk.Lock()
	available := self.locked_credit(true)
	self.lk.Unlock()
	if available != 0 && limit >= available {
		limit -= available
	}
	if available >= maxPrice {
		limit = 0
	}
	self.Log.Debugf("%s maxConfig=%s maxPrice=%s available=%s -> limit=%s",
		tag, maxConfig.FormatCtx(ctx), maxPrice.FormatCtx(ctx), available.FormatCtx(ctx), limit.FormatCtx(ctx))

	err := self.SetAcceptMax(ctx, limit)
	if err != nil {
		return err
	}

	alive := alive.NewAlive()
	alive.Add(2)
	go self.bill.Run(ctx, alive, func(pi money.PollItem) bool {
		switch pi.Status {
		case money.StatusEscrow:
			if pi.DataCount == 1 {
				if err := self.bill.DoEscrowAccept.Do(ctx); err != nil {
					self.Log.Errorf("money.bill escrow accept n=%s err=%v", currency.Amount(pi.DataNominal).FormatCtx(ctx), err)
				}
			}

		case money.StatusCredit:
			itemTime := time.Now()
			self.lk.Lock()
			defer self.lk.Unlock()

			if err := self.billCredit.Add(pi.DataNominal, uint(pi.DataCount)); err != nil {
				self.Log.Errorf("money.bill credit.Add n=%v c=%d err=%v", pi.DataNominal, pi.DataCount, err)
				break
			}
			self.Log.Debugf("money.bill credit amount=%s bill=%s total=%s",
				pi.Amount().FormatCtx(ctx), self.billCredit.Total().FormatCtx(ctx), self.locked_credit(true).FormatCtx(ctx))
			self.dirty += pi.Amount()
			alive.Stop()
			if out != nil {
				out <- Event{created: itemTime, name: EventCredit, amount: pi.Amount()}
			}
		}
		return false
	})
	go self.coin.Run(ctx, alive, func(pi money.PollItem) bool {
		itemTime := time.Now()
		self.lk.Lock()
		defer self.lk.Unlock()

		switch pi.Status {
		case money.StatusDispensed:
			self.Log.Debugf("%s manual dispense: %s", tag, pi.String())
			_ = self.coin.DoTubeStatus.Do(ctx)
			_ = self.coin.CommandExpansionSendDiagStatus(nil)
		case money.StatusReturnRequest:
			alive.Stop()
			if out != nil {
				out <- Event{created: itemTime, name: EventAbort}
			}
		case money.StatusRejected:
			state.GetGlobal(ctx).Tele.StatModify(func(s *tele.Stat) {
				s.CoinRejected[uint32(pi.DataNominal)] += uint32(pi.DataCount)
			})
		case money.StatusCredit:
			err := self.coinCredit.Add(pi.DataNominal, uint(pi.DataCount))
			if err != nil {
				self.Log.Errorf("%s credit.Add n=%v c=%d err=%v", tag, pi.DataNominal, pi.DataCount, err)
			}
			_ = self.coin.DoTubeStatus.Do(ctx)
			_ = self.coin.CommandExpansionSendDiagStatus(nil)
			self.dirty += pi.Amount()
			alive.Stop()
			if out != nil {
				out <- Event{created: itemTime, name: EventCredit, amount: pi.Amount()}
			}
		default:
			panic("unhandled coin POLL item: " + pi.String())
		}
		return false
	})

	select {
	case <-alive.WaitChan():
		return nil
	case <-stopAccept:
		alive.Stop()
		alive.Wait()
		return self.SetAcceptMax(ctx, 0)
	}
}
