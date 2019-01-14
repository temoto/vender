package mega

import (
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

func ParseResponse(b []byte, fun func(p Packet)) error {
	if len(b) == 0 {
		return errors.NotValidf("response empty")
	}
	total := b[0]
	if total == 0 {
		return errors.NotValidf("response=%02x claims length=0", b)
	}
	if total > RESPONSE_MAX_LENGTH {
		return errors.NotValidf("response=%02x claims length=%d > MAX=%d", b, total, RESPONSE_MAX_LENGTH)
	}
	if int(total) > len(b) {
		return errors.NotValidf("response=%02x claims length=%d > buffer=%d", b, total, len(b))
	}
	b = b[:total]
	var err error
	var offset uint8 = 1
	for offset < total {
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
