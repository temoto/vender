package text_display

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWrap(t *testing.T) {
	t.Parallel()

	const width uint32 = 16
	spaces := strings.Repeat(" ", MaxWidth*2)
	canonical := func(input string, tick uint32) string {
		gap := width / 2
		length := uint32(len(input))
		if length <= width {
			return (input + spaces)[:width]
		}
		help := input + spaces[:gap] + input
		offset := tick % (length + gap)
		return help[offset : offset+width]
	}

	type Case struct {
		name  string
		input string
	}
	cases := []Case{
		{"short", "foobar"},
		{"full", "full-length-line"},
		{"long1", "too-much-very-long-line"},
		{"long2", "too-much-very-long-line1;too-much-very-long-line2"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			for tick := uint32(0); tick < uint32(len(c.input)*3); tick++ {
				var buf [width]byte
				scrollWrap(buf[:], []byte(c.input), tick)
				expect := canonical(c.input, tick)
				result := string(buf[:])
				if result != expect {
					t.Errorf("input=(%d)'%s' tick=%d expected=(%d)'%s' actual=(%d)'%s'",
						len(c.input), c.input, tick, len(expect), expect, len(result), result)
				}
			}
		})
	}
}

func TestMessage(t *testing.T) {
	t.Parallel()

	d := NewMockTextDisplay(&TextDisplayConfig{Width: 8})
	ch := make(chan State, 1)
	d.SetUpdateChan(ch)
	d.SetLines("hello", "cursor\x00")
	assert.Equal(t, "hello   \ncursor", (<-ch).String())
	d.Message("padded", "msg", func() {
		assert.Equal(t, "padded  \nmsg     ", (<-ch).String())
	})
	assert.Equal(t, "hello   \ncursor", (<-ch).String())
}

func TestJustCenter(t *testing.T) {
	t.Parallel()

	d, err := NewTextDisplay(&TextDisplayConfig{Width: 8})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, []byte("longlong"), d.JustCenter([]byte("longlong")))
	assert.Equal(t, []byte("longlon"), d.JustCenter([]byte("longlon")))
	assert.Equal(t, []byte("  long  "), d.JustCenter([]byte("long")))
	assert.Equal(t, []byte("   1    "), d.JustCenter([]byte("1")))
}
