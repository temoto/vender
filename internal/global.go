package glb

import (
	"github.com/temoto/vender/log2"
)

var log = log2.NewStderr(log2.LInfo)

func SetEnv(key string, val string) {
	log.Info("AAAAAAAAAAAAAA")
	os.Setenv(key, val)
}

func GetEnv(key string) string {
	return os.Getenv(key)
}
