package mega

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/temoto/vender/crc"
)

//go:generate c-for-go -nocgo mega.c-for-go.yaml
//go:generate mv mega/const.go const.gen.go
//go:generate mv mega/types.go types.gen.go
//go:generate stringer -type=Command_t -trimprefix=Command_
//go:generate stringer -type=Response_t -trimprefix=Response_
//go:generate stringer -type=MDB_State_t -trimprefix=MDB_State_

type Packet struct {
	length byte
	Header Response_t
	Data   []byte
}

func (self *Packet) Parse(b []byte) error {
	if len(b) == 0 {
		return errors.NotValidf("packet empty")
	}
	self.length = b[0]
	if self.length < 3 {
		return errors.NotValidf("packet=%02x claims length=%d < min=3", b, self.length)
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
	self.Header = Response_t(b[1])
	if strings.HasPrefix(self.Header.String(), "Response_t(") {
		return errors.NotValidf("packet=%02x header=%02x", b, byte(self.Header))
	}
	dataLength := self.length - 3
	if dataLength > 0 {
		self.Data = b[2 : 2+dataLength] // GC concern, maybe copy?
	}
	return nil
}

func (self *Packet) Error() string {
	if self.Header&0x80 == 0 {
		return ""
	}
	return fmt.Sprintf("%s(%02x)", self.Header.String(), self.Data)
}

func (self *Packet) Hex() string {
	tmp := make([]byte, 1+len(self.Data))
	tmp[0] = byte(self.Header)
	copy(tmp[1:], self.Data)
	return hex.EncodeToString(tmp)
}

func (self *Packet) String() string {
	info := ""
	switch self.Header {
	case Response_Debug:
		if bytes.Equal(self.Data, []byte{0xbe, 0xeb, 0xee}) {
			info = "just reset"
			break
		}
		if len(self.Data) < 15 {
			info = "invalid format"
			break
		}
		mcusr := self.Data[14]
		resetReason := ""
		if mcusr&(1<<0) != 0 {
			resetReason += "+PO"
		}
		if mcusr&(1<<1) != 0 {
			resetReason += "+EXT"
		}
		if mcusr&(1<<2) != 0 {
			resetReason += "+BO"
		}
		if mcusr&(1<<3) != 0 {
			resetReason += "+WD(PROBLEM)"
		}
		info = fmt.Sprintf("MDB=%s TWI=%v reset=%s",
			MDB_State_t(self.Data[2]).String(),
			self.Data[5:5+8],
			resetReason,
		)
	}
	return fmt.Sprintf("header=%s data=%s info=%s", self.Header.String(), hex.EncodeToString(self.Data), info)
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
