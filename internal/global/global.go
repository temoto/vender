package global

import (
	"fmt"
	"github.com/temoto/vender/log2"
	"os"
	"strconv"
	"sync"
	"time"
)

var Log = *log2.NewStderr(log2.LDebug)
var GBL *globalStruct = nil

type globalStruct struct {
	Client  cliStruct
	Display displayStruct
	HW      hardwareStruct
}

type displayStruct struct {
	L1 string
	L2 string
}
type cliStruct struct {
	Working  bool
	WorkTime time.Time
	Input    string
}
type hardwareStruct struct {
	EvendInput bool
}

// Env later will be entry to external EEPROM (Save important when the power loss)
func init() {
	Log.SetFlags(0)
	_ = GG()
	// os.Clearenv()
}

func GG() *globalStruct {
	var doOnce sync.Once
	doOnce.Do(func() {
		GBL = new(globalStruct)
	})
	return GBL
}

func SetEnv(key string, val string) {
	os.Setenv(key, val)
	Log.Infof("%s=%s", key, val)
}

func SetEnvB(key string, val bool) {
	os.Setenv(key, strconv.FormatBool(val))
}

func SetEnvI(key string, val int) {
	SetEnv(key, strconv.Itoa(int(val)))
}

func ChSetEnv(key string, val string) bool {
	if os.Getenv(key) != val {
		SetEnv(key, val)
		return true
	}
	return false
}

func ChSetEnvB(key string, val bool) bool {
	if GetEnvB(key) != val {
		SetEnv(key, strconv.FormatBool(val))
		return true
	}
	return false
}

func GetEnv(key string) string {
	return os.Getenv(key)
}

func GetEnvB(key string) bool {
	v, _ := strconv.ParseBool(os.Getenv(key))
	return v
}

func ShowEnvs() {
	for _, e := range os.Environ() {
		Log.Infof(e)
	}
	Log.Infof("GBL=%+v".GBL)
}
