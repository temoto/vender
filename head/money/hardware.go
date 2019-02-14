package money

import (
	"context"
	"log"

	"github.com/temoto/vender/hardware/money"
)

func handleGenericPollItem(ctx context.Context, pi money.PollItem, logPrefix string) {
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
