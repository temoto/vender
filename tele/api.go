package tele

import (
	"context"
	"fmt"

	"github.com/temoto/vender/log2"
	tele_config "github.com/temoto/vender/tele/config"
)

//go:generate protoc --go_out=./ tele.proto

type VMID int32

var (
	ErrUnexpectedPacket = fmt.Errorf("unexpected packet")
	ErrNotAuthorized    = fmt.Errorf("not authorized")
)

type Clienter interface {
	Init(context.Context, *log2.Log, tele_config.Config) error
	Error(error)
	StatModify(func(*Stat))
}

func NewClientStub() Clienter { return stub{} }

type stub struct{}

func (stub) Init(context.Context, *log2.Log, tele_config.Config) error { return nil }
func (stub) Error(e error)                                             {}
func (stub) StatModify(func(*Stat))                                    {}
