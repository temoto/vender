package helpers

import (
	"bytes"
	"fmt"
	"math/rand"
	"runtime"
	"testing"
	"time"

	"github.com/temoto/errors"
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
	case uint8:
		if a.(uint8) == b.(uint8) {
			return
		}
	case uint16:
		if a.(uint16) == b.(uint16) {
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
	case []byte:
		if bytes.Equal(a.([]byte), b.([]byte)) {
			return
		}
	default:
		panic(errors.Errorf("code error AssertEqual does not support type of %#v", a))
	}
	location := formatCaller(2)
	t.Fatalf("%s assert equal fail\na=%#v\nb=%#v", location, a, b)
}

func CheckErr(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("unexpected error=%v", err)
	}
}

func formatCaller(depth int) string {
	_, file, line, ok := runtime.Caller(depth)
	if !ok {
		file = "???"
		line = 0
	}
	return fmt.Sprintf("%s:%d", file, line)
}
