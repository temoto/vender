package ui

import (
	"fmt"
	"testing"

	"github.com/temoto/vender/helpers"
)

func TestFormatScale(t *testing.T) {
	t.Parallel()

	type Case struct {
		value  uint8
		min    uint8
		max    uint8
		expect string
	}
	alpha := []byte{'0', '1', '2', '3'}
	cases := []Case{
		{0, 0, 0, "000000"},
		{1, 0, 7, "300000"},
		{2, 0, 7, "320000"},
		{3, 0, 7, "332000"},
		{4, 0, 7, "333100"},
		{5, 0, 7, "333310"},
		{6, 0, 7, "333330"},
		{7, 0, 7, "333333"},
	}

	for _, c := range cases {
		c := c
		t.Run(fmt.Sprintf("scale:%d[%d..%d]", c.value, c.min, c.max), func(t *testing.T) {
			result := string(formatScale(c.value, c.min, c.max, alpha))
			helpers.AssertEqual(t, c.expect, result)
		})
	}
}
