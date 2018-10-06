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

func pollResultLoop(m *MoneySystem, pch <-chan money.PollResult, customItem HardwareStatusHandler, onRestart HardwareFunc, hw Hardwarer, logPrefix string) {
	for pr := range pch {
		handlePollResult(m, &pr, customItem, hw, onRestart, logPrefix)
	}
}

func handlePollResult(m *MoneySystem, pr *money.PollResult, customItem HardwareStatusHandler, hw Hardwarer, onRestart HardwareFunc, logPrefix string) {
	if pr.SingleStatus() == money.StatusBusy {
		log.Printf("%s poll busy", logPrefix)
		time.Sleep(pr.Delay)
		return
	}

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
			log.Printf("%s error: %v", logPrefix, pi.Error)
			// TODO telemetry
		case money.StatusFatal:
			log.Printf("%s fatal: %v", logPrefix, pi.Error)
			// TODO telemetry
		case money.StatusBusy:
		default:
			panic("unhandled poll item status: " + pi.String())
		}
	}
	if doRestart {
		onRestart(m, hw)
	}
}
