package helpers

import (
	"math/rand"
	"testing"
	"time"

	"github.com/juju/errors"
)

type FatalFunc func(...interface{})

type Fataler interface {
	Fatal(...interface{})
}

func RandUnix() *rand.Rand {
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}

func AssertEqual(t testing.TB, a, b interface{}) {
	t.Helper()
	switch a.(type) {
	case bool:
		if a.(bool) == b.(bool) {
			return
		}
	case int:
		if a.(int) == b.(int) {
			return
		}
	case uint:
		if a.(uint) == b.(uint) {
			return
		}
	case int32:
		if a.(int32) == b.(int32) {
			return
		}
	case uint32:
		if a.(uint32) == b.(uint32) {
			return
		}
	case string:
		if a.(string) == b.(string) {
			return
		}
	default:
		panic(errors.Errorf("code error AssertEqual does not support type of %#v", a))
	}
	t.Fatalf("assert equal fail\na=%#v\nb=%#v", a, b)
}
