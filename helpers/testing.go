package helpers

import (
	"fmt"
	"math/rand"
	"runtime"
	"time"
)

type FatalFunc func(...interface{})

type Fataler interface {
	Fatal(...interface{})
}

func RandUnix() *rand.Rand {
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}

func formatCaller(depth int) string {
	_, file, line, ok := runtime.Caller(depth)
	if !ok {
		file = "???"
		line = 0
	}
	return fmt.Sprintf("%s:%d", file, line)
}
