package types

import (
	"context"
	"fmt"

	tele_api "github.com/temoto/vender/tele"
)

var ErrInterrupted = fmt.Errorf("scheduler interrupted, ignore like EPIPE")

type TaskFunc = func(context.Context) error

type Scheduler interface {
	// Schedule(context.Context, tele_api.Priority, TaskFunc) <-chan error
	ScheduleSync(context.Context, tele_api.Priority, TaskFunc) error
}
