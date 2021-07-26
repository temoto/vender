package tele

import (
	"context"

	"github.com/temoto/vender/log2"
	tele_config "github.com/temoto/vender/tele/config"
)

// alexm (install protobuf)
// go get -u github.com/golang/protobuf/{proto,protoc-gen-go}
// sudo apt-get install golang-goprotobuf-dev
// run go generate - not no work working under the root user
//go:generate protoc --go_out=./ tele.proto

// Teler interface Telemetry client, vending machine side.
// Not for external public usage.
type Teler interface {
	Init(context.Context, *log2.Log, tele_config.Config) error
	Close()
	State(State)
	Error(error)
	StatModify(func(*Stat))
	Report(ctx context.Context, serviceTag bool) error
	Transaction(*Telemetry_Transaction)
}

type stub struct{}

func (stub) Init(context.Context, *log2.Log, tele_config.Config) error {
	return nil
}
func (stub) Close()                                            {}
func (stub) State(State)                                       {}
func (stub) Error(error)                                       {}
func (stub) StatModify(func(*Stat))                            {}
func (stub) Report(ctx context.Context, serviceTag bool) error { return nil }
func (stub) Transaction(*Telemetry_Transaction)                {}

func NewStub() Teler { return stub{} }
