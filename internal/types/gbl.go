package types

import (
	"fmt"
	"time"

	"github.com/temoto/vender/log2"
)

var Log = *log2.NewStderr(log2.LDebug)
var VMC *VMCType = nil

type VMCType struct {
	Version string
	Lock    bool
	State   uint32
	Client  struct {
		WorkTime time.Time
		Input    string
		Light    bool
	}
	HW struct {
		Input   bool
		Display struct {
			L1 string
			L2 string
		}
		Elevator struct {
			Position uint8
		}
		Temperature int
	}
	MonSys struct {
		BillOn  bool
		BillRun bool
	}
}

// Amount is integer counting lowest currency unit, e.g. $1.20 = 120
// type Amount uint32

// type UI struct { //nolint:maligned
// 	FrontMaxPrice Amount
// 	FrontResult   UIMenuResult
// }

// type UIMenuResult struct {
// 	Item  MenuItem
// 	Cream uint8
// 	Sugar uint8
// }

// type MenuItem struct {
// 	Name  string
// 	D     Doer
// 	Price Amount
// 	Code  string
// }

// type Doer interface {
// 	Validate() error
// 	Do(context.Context) error
// 	String() string // for logs
// }

func init() {
	Log.SetFlags(0)
	VMC = new(VMCType)
}

func SetLight(v bool) {
	if VMC.Client.Light == v {
		return
	}
	VMC.Client.Light = v
	Log.Infof("light = %v", v)
}

func ShowEnvs() string {
	s := fmt.Sprintf("GBL=%+v", VMC)
	Log.Info(s)
	return s
}
