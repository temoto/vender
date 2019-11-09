package helpers

import (
	"math/rand"
	"time"
)

type FatalFunc func(...interface{})

type Fataler interface {
	Fatal(...interface{})
}

func RandUnix() *rand.Rand {
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}
