package mega

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/temoto/vender/crc"
)

//go:generate ./generate

type ResetFlag uint8

const (
	ResetFlagPowerOn = ResetFlag(1 << iota)
	ResetFlagExternal
	ResetFlagBrownOut
	ResetFlagWatchdog
)

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

func mcusrString(b byte) string {
	s := ""
	if ResetFlag(b)&ResetFlagPowerOn != 0 {
		s += "+PO"
	}
	if ResetFlag(b)&ResetFlagExternal != 0 {
		s += "+EXT"
	}
	if ResetFlag(b)&ResetFlagBrownOut != 0 {
		s += "+BO"
	}
	if ResetFlag(b)&ResetFlagWatchdog != 0 {
		s += "+WD(PROBLEM)"
	}
	return s
}

func (f Field_t) parseAppend(b []byte, arg []byte) ([]byte, uint8) {
	switch f {
	case FIELD_PROTOCOL:
		b = append(b, fmt.Sprintf("protocol=%d", arg[0])...)
		return b, 1
	case FIELD_FIRMWARE_VERSION:
		b = append(b, fmt.Sprintf("firmware=%02x%02x", arg[0], arg[1])...)
		return b, 2
	case FIELD_BEEBEE:
		if bytes.Equal(arg[:3], []byte{0xbe, 0xeb, 0xee}) {
			b = append(b, "beebee-mark"...)
		} else {
			b = append(b, fmt.Sprintf("ERR:invalid-beebee:%02x", arg[:3])...)
		}
		return b, 3
	case FIELD_MCUSR:
		b = append(b, "reset="+mcusrString(arg[0])...)
		return b, 1
	case FIELD_QUEUE_MASTER:
		b = append(b, fmt.Sprintf("master.length=%d", arg[0])...)
		return b, 1
	case FIELD_QUEUE_TWI:
		b = append(b, fmt.Sprintf("twi.length=%d", arg[0])...)
		return b, 1
	case FIELD_MDB_PROTOTCOL_STATE:
		b = append(b, fmt.Sprintf("mdb.state=%d", arg[0])...)
		return b, 1
	case FIELD_MDB_STAT:
		n := arg[0]
		b = append(b, fmt.Sprintf("mdb_stat=%02x", arg[1:1+n])...)
		return b, n
	case FIELD_TWI_STAT:
		n := arg[0]
		b = append(b, fmt.Sprintf("twi_stat=%02x", arg[1:1+n])...)
		return b, n
	default:
		return b, 0
	}
}

func ParseFields(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	idx := uint8(0)
	flen := uint8(0)
	result := make([]byte, 0, 128)
	for int(idx) < len(b) {
		if idx > 0 {
			result = append(result, ',')
		}
		tag := Field_t(b[idx])
		idx++
		result, flen = tag.parseAppend(result, b[idx:])
		if flen == 0 {
			// TODO return err
			result = append(result, fmt.Sprintf("ERR:unknown-tag:%02x", tag)...)
		}
		idx += flen
	}
	return string(result)
}

func (self *Packet) String() string {
	info := ""
	switch Response_t(self.Header) {
	case RESPONSE_STATUS, RESPONSE_JUST_RESET, RESPONSE_DEBUG:
		info = ParseFields(self.Data())
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
