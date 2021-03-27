package global

import (
	"github.com/temoto/vender/log2"
	"os"
)

var Log = log2.NewStderr(log2.LInfo)

// Env later will be entry to external EEPROM (Save important when the power loss)

func SetEnv(key string, val string) {
	if os.Getenv(key) != val {
		Log.Infof("key:(%s) value:(%s) \033[0m\n", key, val)
	}
	os.Setenv(key, val)
}

func GetEnv(key string) string {
	return os.Getenv(key)
}

func ShowEnvs() {
	for _, e := range os.Environ() {
		Log.Info(e)
	}
}
