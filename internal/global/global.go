package global

import (
	"github.com/temoto/vender/log2"
	"os"
	"strconv"
)

var Log = *log2.NewStderr(log2.LDebug)

// Env later will be entry to external EEPROM (Save important when the power loss)
func init() {
	Log.SetFlags(0)
	// os.Clearenv()
}

func SetEnv(key string, val string) {
	os.Setenv(key, val)
}

func SetEnvB(key string, val bool) {
	os.Setenv(key, strconv.FormatBool(val))
}

func SetEnvI(key string, val int) {
	SetEnv(key, strconv.Itoa(int(val)))
}

func ChSetEnv(key string, val string) bool {
	if os.Getenv(key) != val {
		Log.Infof("%s=%s", key, val)
		SetEnv(key, val)
		return true
	}
	return false
}

func ChSetEnvB(key string, val bool) bool {
	if GetEnvB(key) != val {
		Log.Infof("%s=%v", key, val)
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
}
