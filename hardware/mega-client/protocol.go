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

type Packet struct {
	length byte
	Id     byte
	Header byte
	Data   []byte
}

func (self *Packet) Parse(b []byte) error {
	if len(b) == 0 {
		return errors.NotValidf("packet empty")
	}
	self.length = b[0]
	if self.length < 4 {
		return errors.NotValidf("packet=%02x claims length=%d < min=4", b, self.length)
	}
	if int(self.length) > len(b) {
		return errors.NotValidf("packet=%02x claims length=%d > buffer=%d", b, self.length, len(b))
	}
	b = b[:self.length]
	crcIn := b[self.length-1]
	crcLocal := crc.CRC8_p93_n(0, b[:self.length-1])
	if crcIn != crcLocal {
		return errors.NotValidf("packet=%02x crc=%02x actual=%02x", b, crcIn, crcLocal)
	}
	self.Id = b[1]
	self.Header = b[2]
	if strings.HasPrefix(Response_t(self.Header).String(), "Response_t(") {
		return errors.NotValidf("packet=%02x header=%02x", b, byte(self.Header))
	}
	dataLength := self.length - 4
	if dataLength > 0 {
		self.Data = b[3 : 3+dataLength] // GC concern, maybe copy?
	}
	return nil
}

func (self *Packet) Error() string {
	if self.Header&RESPONSE_MASK_ERROR == 0 {
		return ""
	}
	return fmt.Sprintf("%s(%02x)", Response_t(self.Header).String(), self.Data)
}

func (self *Packet) Bytes() []byte {
	plen := 4 + len(self.Data)
	b := make([]byte, plen)
	b[0] = byte(plen)
	b[1] = self.Id
	b[2] = byte(self.Header)
	copy(b[3:], self.Data)
	b[plen-1] = crc.CRC8_p93_n(0, b[:plen-1])
	return b
}

func (self *Packet) SimpleHex() string {
	b := self.Bytes()
	plen := len(b)
	if plen < 4 {
		panic(fmt.Sprintf("code error mega packet='%02x'", b))
	}
	return hex.EncodeToString(b[2 : plen-1])
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

func parseTagged(b []byte) string {
	ss := make([]string, 0, 8)
	idx := uint8(0)
	for int(idx) < len(b) {
		tag := Field_t(b[idx])
		idx++
		switch tag {
		case FIELD_PROTOCOL:
			ss = append(ss, fmt.Sprintf("protocol=%d", b[idx]))
			idx++
		case FIELD_FIRMWARE_VERSION:
			ss = append(ss, fmt.Sprintf("firmware=%02x%02x", b[idx], b[idx+1]))
			idx += 2
		case FIELD_BEEBEE:
			arg := b[idx : idx+3]
			if bytes.Equal(arg, []byte{0xbe, 0xeb, 0xee}) {
				ss = append(ss, "beebee-mark")
			} else {
				ss = append(ss, fmt.Sprintf("ERR:invalid-beebee:%02x", arg))
			}
			idx += 3
		case FIELD_MCUSR:
			ss = append(ss, "reset="+mcusrString(b[idx]))
			idx++
		case FIELD_QUEUE_MASTER:
			ss = append(ss, fmt.Sprintf("master.length=%d", b[idx]))
			idx++
		case FIELD_QUEUE_TWI:
			ss = append(ss, fmt.Sprintf("twi.length=%d", b[idx]))
			idx++
		case FIELD_MDB_PROTOTCOL_STATE:
			ss = append(ss, fmt.Sprintf("mdb.state=%d", b[idx]))
			idx++
		case FIELD_MDB_STAT:
			n := b[idx]
			idx++
			ss = append(ss, fmt.Sprintf("mdb_stat=%02x", b[idx:idx+n]))
			idx += n
		case FIELD_TWI_STAT:
			n := b[idx]
			idx++
			ss = append(ss, fmt.Sprintf("twi_stat=%02x", b[idx:idx+n]))
			idx += n
		default:
			// TODO return err
			ss = append(ss, fmt.Sprintf("ERR:unknown-tag:%02x", tag))
		}
	}
	return strings.Join(ss, ",")
}

func (self *Packet) String() string {
	info := ""
	switch Response_t(self.Header) {
	case RESPONSE_STATUS, RESPONSE_JUST_RESET, RESPONSE_DEBUG:
		info = parseTagged(self.Data)
	}
	return fmt.Sprintf("cmdid=%02x header=%s data=%s info=%s",
		self.Id, Response_t(self.Header).String(), hex.EncodeToString(self.Data), info)
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
		return errors.NotValidf("response=%02x claims length=%d > MAX=%d", b, total, RESPONSE_MAX_LENGTH)
	}
	bufLen := len(b) - 1
	if int(total) > bufLen {
		return errors.NotValidf("response=%02x claims length=%d > buffer=%d", b, total, bufLen)
	}
	b = b[:total+1]
	var err error
	var offset uint8 = 1
	for offset <= total {
		p := Packet{}
		if err = p.Parse(b[offset:]); err != nil {
			return err
		}
		if p.length == 0 {
			panic("dead loop packet.length=0")
		}
		fun(p)
		offset += p.length
	}
	return nil
}
