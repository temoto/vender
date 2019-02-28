package mega

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/temoto/vender/crc"
)

//go:generate ./generate

const packetOverhead = 4

const ProtocolVersion = 3

type Packet struct {
	buf     [RESPONSE_MAX_LENGTH]byte
	Id      byte
	Header  byte
	Fields  Fields
	dataLen uint8
}

func NewPacket(id, header byte, data ...byte) Packet {
	p := Packet{Id: id, Header: header}
	p.Id = id
	p.Header = header
	p.dataLen = uint8(len(data))
	plen := packetOverhead + p.dataLen
	p.buf[0] = plen
	p.buf[1] = p.Id
	p.buf[2] = p.Header
	copy(p.buf[3:], data)
	p.buf[plen-1] = crc.CRC8_p93_n(0, p.buf[:plen-1])
	return p
}

func (self *Packet) Data() []byte {
	if self.dataLen == 0 {
		return nil
	}
	return self.buf[3 : 3+self.dataLen]
}

// Overwrites packet state
func (self *Packet) Parse(b []byte) error {
	if len(b) == 0 {
		return errors.NotValidf("packet empty")
	}
	length := b[0]
	if length < packetOverhead {
		return errors.NotValidf("packet=%x claims length=%d < min=%d", b, length, packetOverhead)
	}
	if int(length) > len(self.buf) {
		return errors.NotValidf("packet=%x claims length=%d > max=%d", b, length, len(self.buf))
	}
	if int(length) > len(b) {
		return errors.NotValidf("packet=%x claims length=%d > input=%d", b, length, len(b))
	}
	self.Fields = Fields{}
	b = b[:length]
	crcIn := b[length-1]
	crcLocal := crc.CRC8_p93_n(0, b[:length-1])
	if crcIn != crcLocal {
		return errors.NotValidf("packet=%x crc=%02x actual=%02x", b, crcIn, crcLocal)
	}
	self.Id = b[1]
	self.Header = b[2]
	switch Response_t(self.Header) {
	case RESPONSE_OK, RESPONSE_RESET, RESPONSE_ERROR:
	default:
		return errors.NotValidf("packet=%x header=%02x", b, byte(self.Header))
	}
	copy(self.buf[:], b)
	self.dataLen = length - packetOverhead

	err := self.Fields.Parse(self.Data())
	return err
}

func (self *Packet) Bytes() []byte {
	return self.buf[:packetOverhead+self.dataLen]
}

func (self *Packet) SimpleHex() string {
	b := self.Bytes()
	return hex.EncodeToString(b[2 : self.dataLen+3])
}

func (self *Packet) String() string {
	fields := self.Fields.String()
	return fmt.Sprintf("cmdid=%02x header=%s data=%s fields=%s",
		self.Id, Response_t(self.Header).String(), hex.EncodeToString(self.Data()), fields)
}

type ResetFlag uint8

const (
	ResetFlagPowerOn = ResetFlag(1 << iota)
	ResetFlagExternal
	ResetFlagBrownOut
	ResetFlagWatchdog
)

type Fields struct {
	Len             uint8
	tagOrder        [32]Field_t
	Protocol        uint8
	Error2s         []uint16
	ErrorNs         []string
	FirmwareVersion uint16
	Mcusr           byte
	Clock10u        uint32
	MdbResult       Mdb_result_t
	MdbError        byte
	MdbData         []byte
	MdbDuration     uint32
	MdbLength       uint8
	TwiData         []byte
	TwiLength       uint8
}

func (self *Fields) Parse(b []byte) error {
	*self = Fields{}
	if len(b) == 0 {
		return nil
	}
	bi := uint8(0)
	flen := uint8(0)
	var tag Field_t
	for int(bi) < len(b) {
		tag, flen = self.parseNext(b[bi:])
		if flen == 0 {
			// bi++ // try to parse rest
			return fmt.Errorf("mega Fields.Parse data=%x tag=%02x(%s) at=%x", b, byte(tag), tag.String(), b[bi:])
		} else {
			if !(tag == FIELD_ERROR2 && len(self.Error2s) == 0) {
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
	case FIELD_PROTOCOL:
		return fmt.Sprintf("protocol=%d", self.Protocol)
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
			es = append(es, e)
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
	case FIELD_MDB_LENGTH:
		return fmt.Sprintf("mdb_length=%d", self.MdbLength)
	case FIELD_TWI_DATA:
		return fmt.Sprintf("twi_data=%x", self.TwiData)
	case FIELD_TWI_LENGTH:
		return fmt.Sprintf("twi_length=%d", self.TwiLength)
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
	case FIELD_PROTOCOL:
		// TODO assert len(arg)>=1
		self.Protocol = arg[0]
		return tag, 1 + 1
	case FIELD_ERROR2:
		// TODO assert len(arg)>=2
		e16 := binary.BigEndian.Uint16(arg)
		self.Error2s = append(self.Error2s, e16)
		return tag, 1 + 2
	case FIELD_ERRORN:
		// TODO assert len(arg)>=1
		n := arg[0]
		// TODO assert len(arg)>=1+n
		es := string(arg[1 : 1+n])
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
	case FIELD_MDB_LENGTH:
		// TODO assert len(arg)>=1
		self.MdbLength = arg[0]
		return tag, 1 + 1
	case FIELD_TWI_DATA:
		// TODO assert len(arg)>=1
		n := arg[0]
		// TODO assert len(arg)>=1+n
		self.TwiData = arg[1 : 1+n]
		return tag, 1 + 1 + n
	case FIELD_TWI_LENGTH:
		// TODO assert len(arg)>=1
		self.TwiLength = arg[0]
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
