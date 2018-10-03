package money

import (
	"log"
	"time"

	"github.com/temoto/vender/hardware/money"
)

type Hardwarer interface {
	InitSequence() error
	CommandReset() error
}

type HardwareStatusHandler func(m *MoneySystem, pr *money.PollResult, pi money.PollItem, hw Hardwarer) bool
type HardwareFunc func(m *MoneySystem, hw Hardwarer)

func pollResultLoop(m *MoneySystem, pch <-chan money.PollResult, customItem HardwareStatusHandler, onRefund, onRestart HardwareFunc, hw Hardwarer, logPrefix string) {
	for pr := range pch {
		handlePollResult(m, &pr, customItem, hw, onRefund, onRestart, logPrefix)
	}
}

func handlePollResult(m *MoneySystem, pr *money.PollResult, customItem HardwareStatusHandler, hw Hardwarer, onRefund, onRestart HardwareFunc, logPrefix string) {
	if pr.SingleStatus() == money.StatusBusy {
		log.Printf("%s poll busy", logPrefix)
		time.Sleep(pr.Delay)
		return
	}

	doRefund := false
	doRestart := false
	for _, pi := range pr.Items {
		if customItem != nil && customItem(m, pr, pi, hw) {
			continue
		}

		switch pi.Status {
		case money.StatusInfo:
			log.Printf("%s info: %s", logPrefix, pi.String())
			// TODO telemetry
		case money.StatusError:
			log.Printf("%s error: %s", logPrefix, pi.Error)
			// TODO telemetry
			doRefund = true
		case money.StatusFatal:
			log.Printf("%s fatal: %s", logPrefix, pi.Error)
			// TODO telemetry
			doRefund = true
			doRestart = true
		case money.StatusBusy:
			// TODO telemetry
		case money.StatusCredit:
			m.Events() <- Event{created: pr.Time, name: EventCredit, amount: pi.Amount()}
			// TODO telemetry
		default:
			panic("unhandled poll item status: " + pi.String())
		}
	}
	if doRefund {
		onRefund(m, hw)
	}
	if doRestart {
		onRestart(m, hw)
	}
}
