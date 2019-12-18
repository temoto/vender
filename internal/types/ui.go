package types

import "context"

type UIer interface {
	Loop(context.Context)
	Scheduler
}
