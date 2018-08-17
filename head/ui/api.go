package ui

import (
	"fmt"
	"log"
)

func Logf(format string, a ...interface{}) {
	s := fmt.Sprintf(format, a...)
	log.Printf("ui: Log() %s", s)
	display.WriteString(s, 0, 0)
}
