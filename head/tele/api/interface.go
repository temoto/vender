package tele_api

import (
	"context"

	tele_config "github.com/temoto/vender/head/tele/config"
	"github.com/temoto/vender/log2"
)

//go:generate protoc --go_out=./ tele.proto

type Teler interface {
	Init(context.Context, *log2.Log, tele_config.Config) error
	State(State)
	Error(error)
	StatModify(func(*Stat))
	Transaction(Telemetry_Transaction)
}

type stub struct{}

func (stub) Init(context.Context, *log2.Log, tele_config.Config) error {
	return nil
}
func (stub) State(State)                       {}
func (stub) Error(error)                       {}
func (stub) StatModify(func(*Stat))            {}
func (stub) Transaction(Telemetry_Transaction) {}

func NewStub() Teler { return stub{} }
