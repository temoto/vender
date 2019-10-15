package money

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/alive"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/hardware/input"
	"github.com/temoto/vender/hardware/money"
	tele_api "github.com/temoto/vender/head/tele/api"
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

	g := state.GetGlobal(ctx)
	maxConfig := currency.Amount(g.Config.Money.CreditMax)
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
					g.Error(errors.Annotatef(err, "money.bill escrow accept n=%s", currency.Amount(pi.DataNominal).FormatCtx(ctx)))
				}
			}

		case money.StatusCredit:
			itemTime := time.Now()
			self.lk.Lock()
			defer self.lk.Unlock()

			if err := self.billCashbox.Add(pi.DataNominal, uint(pi.DataCount)); err != nil {
				g.Error(errors.Annotatef(err, "money.bill cashbox.Add n=%v c=%d", pi.DataNominal, pi.DataCount))
				break
			}
			if err := self.billCredit.Add(pi.DataNominal, uint(pi.DataCount)); err != nil {
				g.Error(errors.Annotatef(err, "money.bill credit.Add n=%v c=%d", pi.DataNominal, pi.DataCount))
				break
			}
			self.Log.Debugf("money.bill credit amount=%s bill=%s total=%s",
				pi.Amount().FormatCtx(ctx), self.billCredit.Total().FormatCtx(ctx), self.locked_credit(true).FormatCtx(ctx))
			self.dirty += pi.Amount()
			alive.Stop()
			if out != nil {
				out <- Event{Created: itemTime, Kind: EventCredit, Amount: pi.Amount()}
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
			// XXX maybe this should be in coin driver
			g.Hardware.Input.Emit(input.Event{Source: input.MoneySourceTag, Key: input.MoneyKeyAbort})

		case money.StatusRejected:
			g.Tele.StatModify(func(s *tele_api.Stat) {
				s.CoinRejected[uint32(pi.DataNominal)] += uint32(pi.DataCount)
			})

		case money.StatusCredit:
			if err := self.billCashbox.Add(pi.DataNominal, uint(pi.DataCount)); err != nil {
				g.Error(errors.Annotatef(err, "%s cashbox.Add n=%v c=%d", tag, pi.DataNominal, pi.DataCount))
				break
			}
			if err := self.coinCredit.Add(pi.DataNominal, uint(pi.DataCount)); err != nil {
				g.Error(errors.Annotatef(err, "%s credit.Add n=%v c=%d", tag, pi.DataNominal, pi.DataCount))
				break
			}
			_ = self.coin.DoTubeStatus.Do(ctx)
			_ = self.coin.CommandExpansionSendDiagStatus(nil)
			self.dirty += pi.Amount()
			alive.Stop()
			if out != nil {
				out <- Event{Created: itemTime, Kind: EventCredit, Amount: pi.Amount()}
			}

		default:
			g.Error(fmt.Errorf("CRITICAL code error unhandled coin POLL item=%#v", pi))
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
