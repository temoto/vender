package display

import (
	"image"
	"strings"
	"testing"

	"github.com/skip2/go-qrcode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQR(t *testing.T) {
	d := NewMock(image.Point{X: 37, Y: 37})
	require.NoError(t, d.Clear())
	assert.Equal(t, strings.Repeat(strings.Repeat("  ", d.size.X)+"\n", d.size.Y), d.String2())

	qrText := "t=20200211T1825&s=23.00&fn=9998887776665555&i=15&fp=0000000000&n=1"
	require.NoError(t, d.QR(qrText, false, qrcode.High))
	qr, err := qrcode.New(qrText, qrcode.High)
	require.NoError(t, err)
	qr.DisableBorder = true
	assert.Equal(t, qr.ToString(false), d.String2())

	require.NoError(t, d.Clear())
	assert.Equal(t, strings.Repeat(strings.Repeat("  ", d.size.X)+"\n", d.size.Y), d.String2())
}
