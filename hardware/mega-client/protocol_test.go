package mega

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewCommand(t *testing.T) {
	t.Parallel()
	f := NewCommand(COMMAND_MDB_TRANSACTION_SIMPLE, 0xe1)
	expect := []byte{0x44, 0x02, 0x08, 0xe1, 0x1f}
	actual := f.Bytes()
	if !bytes.Equal(actual, expect) {
		t.Errorf("frame.Bytes() expected=%x actual=%x", expect, actual)
	}

	wire := bytes.Repeat([]byte{PROTOCOL_PAD_OK}, totalOverheads)
	copy(wire, actual)
	wire[len(actual)] = 0 // padding errcode
	f2 := new(Frame)
	err := f2.Parse(wire)
	if err != nil {
		t.Fatalf("Parse() back err=%v", err)
	}
}

func TestParse(t *testing.T) {
	t.Parallel()
	type Case struct {
		name      string
		input     string
		expectErr string
		expect    *Frame
	}
	cases := []Case{
		{"input-under-length", "", "input length too small not valid", nil},
		{"empty-ok", "040000000101010101010101", "", newResponse(0)},
		{"long", "04f1aaff000101010101010101", "frame=04f1aaff claims length=241 > input-overhead=1 not valid", nil},
		{"corrupted", "0401f4f2e7a9c2d9f5b4a62314", "frame=0401f4f2e7a9c2d9f5b4a62314 padding=b4a62314 not valid", nil},
		{"wrong-padding", "4403019cffffffffffffffffffffff", "frame=4403019cffffffffffffffffffffff padding=ffffffff not valid", nil},
		{"invalid-header", "440100b800010101010101010101", "frame=440100b8 response=Response_t(0) not valid", nil},
		{"ok", "4401012b00010101010101010101", "", newResponse(RESPONSE_OK)},
		{"reset", "4409020200000101050302f90001010101", "", newResponse(
			RESPONSE_RESET,
			tv{FIELD_CLOCK10U, uint32(0)},
			tv{FIELD_FIRMWARE_VERSION, uint16(0x0105)},
			tv{FIELD_MCUSR, byte(ResetFlagExternal)},
		)},
		{"status", "4409010204d40101050302d70001010101", "", newResponse(
			RESPONSE_OK,
			tv{FIELD_CLOCK10U, uint32(12360)},
			tv{FIELD_FIRMWARE_VERSION, uint16(0x0105)},
			tv{FIELD_MCUSR, byte(ResetFlagExternal)},
		)},
		{"mdb-read-unexpected", "440c01025cf61010ff125cd81100df0001010101", "", newResponse(
			RESPONSE_OK,
			tv{FIELD_CLOCK10U, uint32(237980)},
			tv{FIELD_MDB_RESULT, uint16(0xff<<8) | uint16(MDB_RESULT_UART_READ_UNEXPECTED)},
			tv{FIELD_MDB_DURATION10U, uint32(237680)},
			tv{FIELD_MDB_DATA, []byte{}},
		)},
		{"mdb-success-empty", "440901025446100100110029000101010101010101", "", newResponse(
			RESPONSE_OK,
			tv{FIELD_CLOCK10U, uint32(215740)},
			tv{FIELD_MDB_RESULT, uint16(MDB_RESULT_SUCCESS)},
			tv{FIELD_MDB_DATA, []byte{}},
		)},
		{"twi-listen", "44080102040521020031ae00010101010101010101", "", newResponse(
			RESPONSE_OK,
			tv{FIELD_CLOCK10U, uint32(10290)},
			tv{FIELD_TWI_DATA, []byte{0x00, 0x31}},
		)},
	}
	rand.New(rand.NewSource(time.Now().UnixNano())).Shuffle(len(cases), func(i int, j int) { cases[i], cases[j] = cases[j], cases[i] })
	reDebug := regexp.MustCompile(` debug=[[:xdigit:]]*$`)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			input, err := hex.DecodeString(c.input)
			if err != nil {
				t.Fatalf("invalid input=%s err='%v'", c.input, err)
			}
			var f Frame
			errString := ""
			err = f.Parse(input)
			if err == nil {
				err = f.ParseFields()
			}
			if err != nil {
				errString = err.Error()
			} else {
				trimActual := reDebug.ReplaceAllString(f.ResponseString(), "")
				trimExpect := reDebug.ReplaceAllString(c.expect.ResponseString(), "")
				assert.Equal(t, trimExpect, trimActual, "input=%s", c.input)
			}
			if errString != c.expectErr {
				t.Errorf("input=%s error expected='%v' actual='%v'", c.input, c.expectErr, err)
			}
		})
	}
}

func TestParseFields(t *testing.T) {
	t.Parallel()
	type Case struct {
		name   string
		input  string
		expect string
	}
	cases := []Case{
		Case{"status", "4409010204d40101050302d70001010101", "clock10u=12360us,firmware=0105,reset=+EXT"},
		// Case{"mdb-e0", "4409010a01000c015a0b00a600010101010101", "mdb_result=SUCCESS:00,mdb_duration=3460us,mdb_data="},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			input, err := hex.DecodeString(c.input)
			if err != nil {
				t.Fatalf("invalid input=%s err='%v'", c.input, err)
			}

			f := new(Frame)
			err = f.Parse(input)
			if err == nil {
				err = f.ParseFields()
			}
			if err != nil {
				t.Fatalf("f.Parse input=%s err=%v", c.input, err)
			}
			fstr := f.Fields.String()
			if fstr != c.expect {
				t.Errorf("fields expected='%s' actual='%s'", c.expect, fstr)
			}
		})
	}
}

func TestFieldCover(t *testing.T) {
	t.Parallel()
	fs := Fields{}
	invalidTagPrefix := "!ERROR:invalid-tag"
	if !strings.HasPrefix(fs.FieldString(FIELD_INVALID), invalidTagPrefix) {
		t.Fatalf("test code error: invalid tag prefix")
	}

	for f8 := uint8(1); f8 != 0; f8++ {
		f := Field_t(f8)
		fstr := f.String()
		should := !strings.HasPrefix(fstr, "Field_t(")
		hasString := !strings.HasPrefix(fs.FieldString(f), invalidTagPrefix)
		data := [BUFFER_SIZE]byte{}
		data[0] = f8
		hasParse := false
		if pf, _ := fs.parseNext(data[:]); pf != FIELD_INVALID {
			hasParse = true
		}
		if should && !hasString {
			t.Errorf("tag=%02x name=%s defined in firmware but no String() in client", f8, fstr)
		}
		if should && !hasParse {
			t.Errorf("tag=%02x name=%s defined in firmware but no parser in client", f8, fstr)
		}
		if !should && (hasParse || hasString) {
			t.Errorf("tag=%02x not defined in firmware (anymore?)", f8)
		}
	}
}

func BenchmarkParse(b *testing.B) {
	type Case struct {
		name  string
		input string
	}
	cases := []Case{
		Case{"reset", "4409020200000101050302f90001010101"},
		Case{"status", "4409010204d40101050302d70001010101"},
		Case{"mdb-long", "441c0102141b1001001208a2111006000b0100010a06d807362800000701080001010101"},
	}

	mkBench := func(input []byte) func(b *testing.B) {
		return func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(input)))
			f := Frame{}
			b.ResetTimer()
			for i := 1; i <= b.N; i++ {
				err := f.Parse(input)
				if err != nil {
					b.Fatal(err)
				}
			}
		}
	}

	for _, c := range cases {
		c := c
		inputBytes, err := hex.DecodeString(c.input)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(c.name, mkBench(inputBytes))
	}
}

type tv struct {
	tag   Field_t
	value interface{}
}

func newResponse(header Response_t, fields ...tv) *Frame {
	f := &Frame{Version: ProtocolVersion}
	if header != 0 || len(fields) > 0 {
		f.Flag |= PROTOCOL_FLAG_PAYLOAD
		f.plen = 1
	}
	f.buf[0] = f.Flag | f.Version
	f.buf[1] = f.plen
	f.buf[2] = byte(header)
	for _, ff := range fields {
		switch ff.tag {
		case FIELD_CLOCK10U:
			f.Fields.Clock10u = ff.value.(uint32)
		case FIELD_ERROR2:
			f.Fields.Error2s = ff.value.([]uint16)
		case FIELD_ERRORN:
			f.Fields.ErrorNs = ff.value.([][]byte)
		case FIELD_FIRMWARE_VERSION:
			f.Fields.FirmwareVersion = ff.value.(uint16)
		case FIELD_MCUSR:
			f.Fields.Mcusr = ff.value.(byte)
		case FIELD_MDB_RESULT:
			f.Fields.MdbError = byte(ff.value.(uint16) >> 8)
			f.Fields.MdbResult = Mdb_result_t(ff.value.(uint16) & 0xff)
		case FIELD_MDB_DATA:
			b := ff.value.([]byte)
			f.Fields.MdbLength = uint8(len(b))
			f.Fields.MdbData = b
		case FIELD_MDB_DURATION10U:
			f.Fields.MdbDuration = ff.value.(uint32)
		case FIELD_TWI_DATA:
			f.Fields.TwiData = ff.value.([]byte)
		default:
			panic(fmt.Sprintf("code error ff.t=%v", ff.tag))
		}
		f.Fields.tagOrder[f.Fields.Len] = ff.tag
		f.Fields.Len++
	}
	return f
}
