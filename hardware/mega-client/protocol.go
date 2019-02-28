package mega

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/temoto/vender/crc"
)

//go:generate ./generate

const packetOverhead = 4

type Packet struct {
	buf     [RESPONSE_MAX_LENGTH]byte
	Id      byte
	Header  byte
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
func (self *Packet) Parse(b []byte) (uint8, error) {
	if len(b) == 0 {
		return 0, errors.NotValidf("packet empty")
	}
	length := b[0]
	if length < packetOverhead {
		return length, errors.NotValidf("packet=%02x claims length=%d < min=%d", b, length, packetOverhead)
	}
	if int(length) > len(self.buf) {
		return length, errors.NotValidf("packet=%02x claims length=%d > max=%d", b, length, len(self.buf))
	}
	if int(length) > len(b) {
		return length, errors.NotValidf("packet=%02x claims length=%d > input=%d", b, length, len(b))
	}
	b = b[:length]
	crcIn := b[length-1]
	crcLocal := crc.CRC8_p93_n(0, b[:length-1])
	if crcIn != crcLocal {
		return length, errors.NotValidf("packet=%02x crc=%02x actual=%02x", b, crcIn, crcLocal)
	}
	self.Id = b[1]
	self.Header = b[2]
	if strings.HasPrefix(Response_t(self.Header).String(), "Response_t(") {
		return length, errors.NotValidf("packet=%02x header=%02x", b, byte(self.Header))
	}
	copy(self.buf[:], b)
	self.dataLen = length - packetOverhead
	return length, nil
}

func (self *Packet) Bytes() []byte {
	return self.buf[:packetOverhead+self.dataLen]
}

func (self *Packet) SimpleHex() string {
	b := self.Bytes()
	return hex.EncodeToString(b[2 : self.dataLen+3])
}

func (self *Packet) ParseFields() (Fields, error) {
	f := Fields{}
	err := f.Parse(self.Data())
	return f, err
}

func (self *Packet) String() string {
	info := ""
	switch Response_t(self.Header) {
	case RESPONSE_STATUS, RESPONSE_JUST_RESET, RESPONSE_DEBUG:
		fields, err := self.ParseFields()
		if err != nil {
			info = "!ERROR:ParseFields err=" + err.Error()
		} else {
			info = fields.String()
		}
	}
	return fmt.Sprintf("cmdid=%02x header=%s data=%s info=%s",
		self.Id, Response_t(self.Header).String(), hex.EncodeToString(self.Data()), info)
}

func ParseResponse(b []byte, fun func(p Packet)) error {
	if len(b) == 0 {
		return errors.NotValidf("response empty")
	}
	total := b[0]
	if total == 0 {
		return nil
	}
	if total > RESPONSE_MAX_LENGTH {
		return errors.NotValidf("response=%02x claims length=%d > max=%d", b, total, RESPONSE_MAX_LENGTH)
	}
	bufLen := len(b) - 1
	if int(total) > bufLen {
		return errors.NotValidf("response=%02x claims length=%d > input=%d", b, total, bufLen)
	}
	b = b[:total+1]
	var err error
	var offset uint8 = 1
	var plen byte
	for offset <= total {
		p := Packet{}
		if plen, err = p.Parse(b[offset:]); err != nil {
			return err
		}
		if plen == 0 {
			panic("dead loop plen=0")
		}
		fun(p)
		offset += plen
	}
	return nil
}

type ResetFlag uint8

const (
	ResetFlagPowerOn = ResetFlag(1 << iota)
	ResetFlagExternal
	ResetFlagBrownOut
	ResetFlagWatchdog
)

type Fields struct {
	Len              uint8
	tagOrder         [9]Field_t
	Protocol         uint8
	FirmwareVersion  uint16
	Mcusr            byte
	QueueMaster      uint8
	QueueTwi         uint8
	MdbProtocolState Mdb_state_t
	MdbStat_b        [1]byte
	TwiStat_b        [8]byte
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
			return fmt.Errorf("mega Fields.Parse data=%02x tag=%02x rest=%02x", b, tag, b[bi:])
		} else {
			self.tagOrder[self.Len] = tag
			self.Len++
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
	case FIELD_BEEBEE:
		return "beebee-mark"
	case FIELD_MCUSR:
		return "reset=" + mcusrString(self.Mcusr)
	case FIELD_QUEUE_MASTER:
		return fmt.Sprintf("master.length=%d", self.QueueMaster)
	case FIELD_QUEUE_TWI:
		return fmt.Sprintf("twi.length=%d", self.QueueTwi)
	case FIELD_MDB_PROTOCOL_STATE:
		return fmt.Sprintf("mdb.state=%s", self.MdbProtocolState.String())
	case FIELD_MDB_STAT:
		// TODO use parsed mdb stat
		return fmt.Sprintf("mdb_stat=%02x", self.MdbStat_b)
	case FIELD_TWI_STAT:
		// TODO use parsed twi stat
		return fmt.Sprintf("twi_stat=%02x", self.TwiStat_b)
	default:
		return fmt.Sprintf("!ERROR:invalid-tag:%02x", tag)
	}
}

func (self *Fields) parseNext(b []byte) (Field_t, uint8) {
	tag := Field_t(b[0])
	arg := b[1:]
	switch tag {
	case FIELD_PROTOCOL:
		// assert len(arg)>=1
		self.Protocol = arg[0]
		return tag, 1 + 1
	case FIELD_FIRMWARE_VERSION:
		self.FirmwareVersion = binary.BigEndian.Uint16(arg)
		return tag, 1 + 2
	case FIELD_BEEBEE:
		if bytes.Equal(arg[:3], []byte{0xbe, 0xeb, 0xee}) {
			return tag, 1 + 3
		} else {
			return FIELD_INVALID, 0
		}
	case FIELD_MCUSR:
		self.Mcusr = arg[0]
		return tag, 1 + 1
	case FIELD_QUEUE_MASTER:
		self.QueueMaster = arg[0]
		return tag, 1 + 1
	case FIELD_QUEUE_TWI:
		self.QueueTwi = arg[0]
		return tag, 1 + 1
	case FIELD_MDB_PROTOCOL_STATE:
		self.MdbProtocolState = Mdb_state_t(arg[0])
		return tag, 1 + 1
	case FIELD_MDB_STAT:
		n := arg[0]
		// assert n==len(self.MdbStat_b)
		// TODO parse mdb stat
		copy(self.MdbStat_b[:], arg[1:])
		return tag, 1 + n
	case FIELD_TWI_STAT:
		n := arg[0]
		// assert n==len(self.TwiStat_b)
		// TODO parse mdb stat
		copy(self.TwiStat_b[:], arg[1:])
		return tag, 1 + n
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
