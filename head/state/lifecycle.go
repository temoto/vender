// Package state let subsystems register lifecycle callbacks.
package state

import (
	"context"
	"fmt"

	"github.com/temoto/vender/helpers/actionlist"
	"github.com/temoto/vender/log2"
)

// Implemented by subsystems
type Systemer interface {
	String() string
	Start(context.Context) error
	Validate(context.Context) error
	Stop(context.Context) error
}

type Lifecycle struct {
	Log        *log2.Log
	OnValidate actionlist.List
	OnStart    actionlist.List
	OnStop     actionlist.List
}

func NewLifecycle(log *log2.Log) *Lifecycle {
	self := &Lifecycle{
		Log: log,
	}
	return self
}

func (self *Lifecycle) RegisterValidate(fun actionlist.Func, tag string) {
	self.OnValidate.Append(fun, tag+":validate")
}
func (self *Lifecycle) RegisterStart(fun actionlist.Func, tag string) {
	self.OnStart.Append(fun, tag+":start")
}
func (self *Lifecycle) RegisterStop(fun actionlist.Func, tag string) {
	self.OnStop.Append(fun, tag+":stop")
}
func (self *Lifecycle) RegisterSystem(s Systemer) {
	self.OnValidate.Append(s.Validate, fmt.Sprintf("sys:%s:validate", s.String()))
	self.OnStart.Append(s.Start, fmt.Sprintf("sys:%s:start", s.String()))
	self.OnStop.Append(s.Stop, fmt.Sprintf("sys:%s:stop", s.String()))
}

func (self *Lifecycle) Restart(ctx context.Context) {
	self.Log.Debugf("restart requested")
	self.OnStop.Do(ctx)
}
