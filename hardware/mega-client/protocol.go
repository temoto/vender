package mega

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/temoto/errors"
	"github.com/temoto/vender/crc"
	"github.com/temoto/vender/helpers"
)

//go:generate ./generate

const frameOverhead = 3
const paddingOverhead = 5
const totalOverheads = frameOverhead + paddingOverhead + /*reserve*/ 4

const ProtocolVersion = 4

func parseHeader(b []byte) (flag, version, length byte, err error) {
	flag = b[0] & PROTOCOL_HEADER_FLAG_MASK
	version = b[0] & PROTOCOL_HEADER_VERSION_MASK
	length = b[1]
	if version != ProtocolVersion {
		err = errors.NotValidf("frame=%x version=%d expected=%d", b, version, ProtocolVersion)
	}
	return
}

func parsePadding(b []byte, requireOK bool) (start int, code Errcode_t, err error) {
	start = -1
	pads := b[len(b)-4:]
	pad := pads[0]
	if !((pads[1] == pad) && (pads[2] == pad) && (pads[3] == pad)) {
		err = errors.NotValidf("frame=%x padding=%x", b, pads)
		return
	}
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] != pad {
			start = i + 1
			code = Errcode_t(b[i])
			break
		}
	}
	switch pad {
	case PROTOCOL_PAD_OK:
	case PROTOCOL_PAD_ERROR:
		if code == 0 {
			err = errors.Errorf("frame=%x pad=error code=0", b)
		} else if requireOK {
			err = errors.Errorf("frame=%x pad=error code=(%02x)%s", b, byte(code), code.String())
		}
	default:
		err = errors.NotValidf("frame=%x padding=%x", b, pads)
	}
	return
}

type Frame struct {
	Fields  Fields
	buf     [BUFFER_SIZE]byte
	Version byte
	Flag    byte
	crc     uint8
	Errcode Errcode_t
	plen    uint8
}

func NewCommand(cmd Command_t, data ...byte) Frame {
	f := Frame{
		Version: ProtocolVersion,
		Flag:    PROTOCOL_FLAG_PAYLOAD,
		plen:    uint8(1 /*command*/ + len(data)),
	}
	f.buf[0] = f.Flag | f.Version
	f.buf[1] = f.plen
	f.buf[2] = byte(cmd)
	copy(f.buf[3:], data)
	f.crc = crc.CRC8_p93_n(0, f.buf[1:2+f.plen])
	f.buf[2+f.plen] = f.crc
	return f
}

func (self *Frame) Bytes() []byte {
	return self.buf[:frameOverhead+self.plen]
}

func (self *Frame) Payload() []byte {
	if self.plen == 0 {
		return nil
	}
	return self.buf[2 : 2+self.plen]
}

func (self *Frame) ResponseKind() Response_t {
	if (self.Flag&PROTOCOL_FLAG_PAYLOAD == 0) || (self.plen == 0) {
		return 0
	}
	return Response_t(self.buf[2])
}

func (self *Frame) HeaderString() string {
	return fmt.Sprintf("%02x", self.buf[0])
}

func (self *Frame) CommandString() string {
	if self == nil {
		return ""
	}
	cmd := Command_t(self.buf[2])
	return fmt.Sprintf("%s %x debug=%x", cmd.String(), self.Payload(), self.Bytes())
}

func (self *Frame) ResponseString() string {
	kind := self.ResponseKind()
	fields := self.Fields.String()
	return fmt.Sprintf("%s %s debug=%x", kind.String(), fields, self.Bytes())
}

// Overwrites frame state
func (self *Frame) Parse(b []byte) error {
	if len(b) < totalOverheads {
		return errors.NotValidf("input length too small")
	}

	var padStart int
	var err error
	padStart, self.Errcode, err = parsePadding(b, true)
	if err != nil {
		return err
	}

	b = b[:padStart-1]
	if len(b) < frameOverhead {
		return errors.NotValidf("frame=%x before padding is too short min=%d", b, frameOverhead)
	}

	if self.Flag, self.Version, self.plen, err = parseHeader(b); err != nil {
		return err
	}
	if int(self.plen) > len(b)-frameOverhead {
		return errors.NotValidf("frame=%x claims length=%d > input-overhead=%d", b, self.plen, len(b)-frameOverhead)
	}

	crcInput := b[2+self.plen]
	crcLocal := crc.CRC8_p93_n(0, b[1:2+self.plen])
	if crcInput != crcLocal {
		return errors.NotValidf("frame=%x crc=%02x actual=%02x", b, crcInput, crcLocal)
	}
	self.crc = crcLocal

	copy(self.buf[:], b[:frameOverhead+self.plen])
	if (self.Flag&PROTOCOL_FLAG_PAYLOAD == 0) && (self.plen > 0) {
		return errors.NotValidf("frame=%x FLAG_PAYLOAD=no payload len=%d", b, self.plen)
	}

	return nil
}

func (self *Frame) ParseFields() error {
	self.Fields = Fields{}
	if self.plen == 0 {
		return nil
	}
	switch self.ResponseKind() {
	case RESPONSE_OK, RESPONSE_RESET, RESPONSE_TWI_LISTEN, RESPONSE_ERROR:
	default:
		return errors.NotValidf("frame=%x response=%s", self.Bytes(), self.ResponseKind())
	}
	return self.Fields.Parse(self.Payload()[1:])
}

type ResetFlag uint8

const (
	ResetFlagPowerOn = ResetFlag(1 << iota)
	ResetFlagExternal
	ResetFlagBrownOut
	ResetFlagWatchdog
)

// Sorry for inhumane field order, it's used often and probably worth align optimisation.
type Fields struct {
	ErrorNs         [][]byte
	Error2s         []uint16
	MdbData         []byte
	TwiData         []byte
	tagOrder        [32]Field_t
	Clock10u        uint32
	MdbDuration     uint32
	FirmwareVersion uint16
	Len             uint8
	Mcusr           byte
	MdbResult       Mdb_result_t
	MdbError        byte
	MdbLength       uint8
	TwiAddr         byte
}

func (self *Fields) Parse(b []byte) error {
	*self = Fields{}
	if len(b) == 0 {
		return nil
	}
	bi := uint8(0)
	var flen uint8
	var tag Field_t
	for int(bi) < len(b) {
		tag, flen = self.parseNext(b[bi:])
		if flen == 0 {
			// bi++ // try to parse rest
			return fmt.Errorf("mega Fields.Parse data=%x tag=%02x(%s) at=%x", b, byte(tag), tag.String(), b[bi:])
		} else {
			if !((tag == FIELD_ERROR2 && len(self.Error2s) == 0) || (tag == FIELD_ERRORN && len(self.ErrorNs) == 0)) {
				self.tagOrder[self.Len] = tag
				self.Len++
			}
			bi += flen
		}
	}
	return nil
}

func (self Fields) String() string {
	buf := make([]byte, 0, 128)
	for i := uint8(0); i < self.Len; i++ {
		tag := self.tagOrder[i]
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, self.FieldString(tag)...)
	}
	return string(buf)
}
func (self Fields) FieldString(tag Field_t) string {
	switch tag {
	case FIELD_FIRMWARE_VERSION:
		// TODO check/ensure byte order
		return fmt.Sprintf("firmware=%04x", self.FirmwareVersion)
	case FIELD_ERROR2:
		es := []string{}
		for _, e16 := range self.Error2s {
			code := Errcode_t(e16 >> 8)
			arg := e16 & 0xff
			es = append(es, fmt.Sprintf("%s:%02x", code.String(), arg))
		}
		return fmt.Sprintf("error2=%s", strings.Join(es, "|"))
	case FIELD_ERRORN:
		es := []string{}
		for _, e := range self.ErrorNs {
			es = append(es, helpers.HexSpecialBytes(e))
		}
		return fmt.Sprintf("errorn=%s", strings.Join(es, "|"))
	case FIELD_MCUSR:
		return "reset=" + mcusrString(self.Mcusr)
	case FIELD_MDB_RESULT:
		return fmt.Sprintf("mdb_result=%s:%02x", self.MdbResult.String(), self.MdbError)
	case FIELD_MDB_DATA:
		return fmt.Sprintf("mdb_data=%x", self.MdbData)
	case FIELD_MDB_DURATION10U:
		return fmt.Sprintf("mdb_duration=%dus", self.MdbDuration)
	case FIELD_TWI_DATA:
		return fmt.Sprintf("twi_data=%x", self.TwiData)
	case FIELD_TWI_ADDR:
		return fmt.Sprintf("twi_addr=%d", self.TwiAddr)
	case FIELD_CLOCK10U:
		return fmt.Sprintf("clock10u=%dus", self.Clock10u)
	default:
		return fmt.Sprintf("!ERROR:invalid-tag:%02x", tag)
	}
}

func (self *Fields) parseNext(b []byte) (Field_t, uint8) {
	tag := Field_t(b[0])
	arg := b[1:]
	switch tag {
	case FIELD_ERROR2:
		// TODO assert len(arg)>=2
		e16 := binary.BigEndian.Uint16(arg)
		self.Error2s = append(self.Error2s, e16)
		return tag, 1 + 2
	case FIELD_ERRORN:
		// TODO assert len(arg)>=1
		n := arg[0]
		// TODO assert len(arg)>=1+n
		es := arg[1 : 1+n]
		self.ErrorNs = append(self.ErrorNs, es)
		return tag, 1 + 1 + n
	case FIELD_FIRMWARE_VERSION:
		// TODO assert len(arg)>=2
		self.FirmwareVersion = binary.BigEndian.Uint16(arg)
		return tag, 1 + 2
	case FIELD_MCUSR:
		// TODO assert len(arg)>=1
		self.Mcusr = arg[0]
		return tag, 1 + 1
	case FIELD_MDB_RESULT:
		// TODO assert len(arg)>=4
		self.MdbResult = Mdb_result_t(arg[0])
		self.MdbError = arg[1]
		return tag, 1 + 2
	case FIELD_MDB_DATA:
		// TODO assert len(arg)>=1
		n := arg[0]
		// TODO assert len(arg)>=1+n
		self.MdbData = arg[1 : 1+n]
		return tag, 1 + 1 + n
	case FIELD_MDB_DURATION10U:
		// TODO assert len(arg)>=2
		self.MdbDuration = uint32(binary.BigEndian.Uint16(arg)) * 10
		return tag, 1 + 2
	case FIELD_TWI_DATA:
		// TODO assert len(arg)>=1
		n := arg[0]
		// TODO assert len(arg)>=1+n
		self.TwiData = arg[1 : 1+n]
		return tag, 1 + 1 + n
	case FIELD_TWI_ADDR:
		// TODO assert len(arg)>=1
		self.TwiAddr = arg[0]
		return tag, 1 + 1
	case FIELD_CLOCK10U:
		// TODO assert len(arg)>=2
		self.Clock10u = uint32(binary.BigEndian.Uint16(arg)) * 10
		return tag, 1 + 2
	default:
		return FIELD_INVALID, 0
	}
}

func mcusrString(b byte) string {
	s := ""
	r := ResetFlag(b)
	if r&ResetFlagPowerOn != 0 {
		s += "+PO"
	}
	if r&ResetFlagExternal != 0 {
		s += "+EXT"
	}
	if r&ResetFlagBrownOut != 0 {
		s += "+BO"
	}
	if r&ResetFlagWatchdog != 0 {
		s += "+WD(PROBLEM)"
	}
	return s
}
