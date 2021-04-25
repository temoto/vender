package global

import (
	"github.com/temoto/vender/log2"
	"sync"
	"time"
)

var Log = *log2.NewStderr(log2.LDebug)
var GBL *globalStruct = nil

type globalStruct struct {
	Version string
	State   string
	Client  cliStruct
	Display displayStruct
	HW      hardwareStruct
	Tele    telestruckt
}

type telestruckt struct {
	Working bool
}

type displayStruct struct {
	L1 string
	L2 string
}
type cliStruct struct {
	Working  bool
	WorkTime time.Time
	Input    string
	Light    bool
}
type hardwareStruct struct {
	Temperature      int
	TemperatureValid bool
	EvendInput       bool
	Elevator         uint8
}

// Env later will be entry to external EEPROM (Save important when the power loss)
func init() {
	Log.SetFlags(0)
	_ = GG()
	GBL.HW.Elevator = 255
	// os.Clearenv()
}

func GG() *globalStruct {
	var doOnce sync.Once
	doOnce.Do(func() {
		GBL = new(globalStruct)
	})
	return GBL
}

func ShowEnvs() {
	Log.Infof("GBL=%+v", GBL)
}
