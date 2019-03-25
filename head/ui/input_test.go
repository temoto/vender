package ui

import "testing"

func TestInput(t *testing.T) {
	src := make(chan uint16, 32)
	src <- 0x0031
}
