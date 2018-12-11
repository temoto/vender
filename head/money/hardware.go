package money

import (
	"context"
	"log"

	"github.com/temoto/vender/hardware/money"
	"github.com/temoto/vender/helpers/msync"
)

type HardwareStatusHandler func(m *MoneySystem, pr *money.PollResult, pi money.PollItem) bool
type HardwareFunc func(m *MoneySystem, restart msync.Doer)

func genericPollResultLoop(ctx context.Context, m *MoneySystem, pch <-chan money.PollResult,
	customItem HardwareStatusHandler, restart msync.Doer, logPrefix string,
) {
	for pr := range pch {
		handlePollResult(ctx, m, &pr, customItem, restart, logPrefix)
	}
}

func handlePollResult(ctx context.Context, m *MoneySystem, pr *money.PollResult,
	customItem HardwareStatusHandler, restart msync.Doer, logPrefix string,
) {
	// if pr.SingleStatus() == money.StatusBusy {
	//	log.Printf("%s poll busy", logPrefix)
	//	time.Sleep(pr.Delay)
	//	return
	// }

	doRestart := false
	for _, pi := range pr.Items {
		if customItem != nil && customItem(m, pr, pi) {
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
		restart.Do(ctx)
	}
}
