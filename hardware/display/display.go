package display

import (
	"image"
	"image/color"
	"strings"
	"io/ioutil"

	"github.com/juju/errors"
	"github.com/skip2/go-qrcode"
	"github.com/temoto/vender/hardware/display/framebuffer"
)

type Display struct {
	fb   *framebuffer.Framebuffer
	pix  []color.RGBA
	size image.Point
}

func NewFb(dev string) (*Display, error) {
	fb, err := framebuffer.New(dev)
	if err != nil {
		return nil, errors.Annotatef(err, "framebuffer device=%s", dev)
	}
	size := fb.Size()
	d := &Display{
		fb:   fb,
		pix:  make([]color.RGBA, size.X*size.Y),
		size: size,
	}
	return d, nil
}

func NewMock(size image.Point) *Display {
	return &Display{
		pix:  make([]color.RGBA, size.X*size.Y),
		size: size,
	}
}

func (d *Display) Clear() error {
	for y := 0; y < d.size.Y; y++ {
		for x := 0; x < d.size.X; x++ {
			d.set(x, y, color.RGBA{0, 0, 0, 0xff})
		}
	}
	return d.Flush()
}

func (d *Display) Flush() error {
	if d.fb != nil {
		if err := d.fb.Update(d.pix); err != nil {
			return err
		}
		return d.fb.Flush()
	}
	return nil
}

func (d *Display) Picture(file string) error {
    input, err := ioutil.ReadFile(file)
    if err != nil {
            d.Clear()
            return errors.Annotate(err,"Picture ReadFile")
    }

    err = ioutil.WriteFile("/dev/fb0", input, 0644)
    if err != nil {
            d.Clear()
            return errors.Annotate(err,"Picture WriteFile")
    }
    return nil
}

func (d *Display) QR(text string, border bool, level qrcode.RecoveryLevel) error {
	qr, err := qrcode.New(text, level)
	if err != nil {
		return errors.Annotate(err, "QR")
	}
	qr.DisableBorder = !border
	minSize := minInt(d.size.X, d.size.Y)
	img := qr.Image(minSize).(*image.Paletted)
	if !img.Rect.In(image.Rectangle{Max: d.size}) {
		return errors.Errorf("QR image size=%s > display size=%s", img.Bounds().Max.String(), d.size.String())
	}
	// log.Printf("QR size=%s", img.Bounds().Max.String())
	d.palleted2(img)
	return d.Flush()
}

func (d *Display) String2() string {
	b := strings.Builder{}
	b.Grow((d.size.X + 1) * d.size.Y) // +1 for \n
	for y := 0; y < d.size.Y; y++ {
		for x := 0; x < d.size.X; x++ {
			c := d.get(x, y)
			if c.R == 0 && c.G == 0 && c.B == 0 {
				b.WriteString("  ")
			} else {
				b.WriteString("██")
			}
		}
		b.WriteRune('\n')
	}
	return b.String()
}

func (d *Display) palleted2(img *image.Paletted) {
	min, max := img.Bounds().Min, img.Bounds().Max
	bg := toRGBA(img.Palette[0])
	fg := toRGBA(img.Palette[1])
	for y := min.Y; y < max.Y; y++ {
		for x := min.X; x < max.X; x++ {
			palidx := img.Pix[img.PixOffset(x, y)]
			c := bg
			if palidx != 0 {
				c = fg
			}
			d.set(x, y, c)
		}
	}
}

func (d *Display) get(x, y int) color.RGBA    { return d.pix[y*d.size.X+x] }
func (d *Display) set(x, y int, c color.RGBA) { d.pix[y*d.size.X+x] = c }

func minInt(i1, i2 int) int {
	if i1 <= i2 {
		return i1
	}
	return i2
}

func toRGBA(c color.Color) color.RGBA {
	r, g, b, a := c.RGBA()
	return color.RGBA{uint8(r), uint8(g), uint8(b), uint8(a)}
}
