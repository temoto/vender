package money

import (
	"context"

	"github.com/temoto/vender/hardware/money"
)

func (self *MoneySystem) handleGenericPollItem(ctx context.Context, pi money.PollItem, logPrefix string) {
	switch pi.Status {
	case money.StatusInfo:
		self.Log.Debugf("%s info: %s", logPrefix, pi.String())
		// TODO telemetry
	case money.StatusError:
		self.Log.Errorf("%s error: %v", logPrefix, pi.Error)
		// TODO telemetry
	case money.StatusFatal:
		self.Log.Errorf("%s fatal: %v", logPrefix, pi.Error)
		// TODO telemetry
	case money.StatusBusy:
	default:
		panic("unhandled poll item status: " + pi.String())
	}
}
