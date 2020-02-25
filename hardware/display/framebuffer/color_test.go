package framebuffer

import (
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRGB565(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input  color.RGBA
		expect uint16
	}{
		{color.RGBA{0, 0, 0, 0}, 0},
		{color.RGBA{0, 0, 0, 0xff}, 0},
		{color.RGBA{0xff, 0xff, 0xff, 0xff}, 0xffff},
		{color.RGBA{0xff, 0x00, 0x00, 0xff}, 0xf800},
		{color.RGBA{0x00, 0xff, 0x00, 0xff}, 0x07e0},
		{color.RGBA{0x00, 0x00, 0xff, 0xff}, 0x001f},
		{color.RGBA{0x0c, 0x0c, 0x0c, 0xff}, 0x0861},
	}
	for _, c := range cases {
		assert.Equal(t, c.expect, encode565(c.input), c.input)
	}
}
