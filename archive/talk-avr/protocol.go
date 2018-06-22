package talkavr

import (
	"errors"
	"github.com/temoto/vender/crc"
	"io"
)

type Packet struct {
	Header byte
	Data   []byte
	Crc8   byte
}

func (p *Packet) Length() int { return len(p.Data) + 3 }

func (p *Packet) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(p.Bytes())
	return int64(n), err
}

func (p *Packet) Bytes() []byte {
	length := byte(p.Length())
	out := make([]byte, length)
	out[0] = length
	out[1] = p.Header
	for i, b := range p.Data {
		out[i+2] = b
	}
	out[length-1] = p.Crc8
	return out
}

func (p *Packet) Read(b []byte) (int, error) {
	if len(b) < 1 {
		return 0, io.EOF
	}
	length := int(b[0])
	if len(b) < length {
		return 0, io.EOF
	}
	header := b[1]
	data := b[2 : length-2]
	crcIn := b[length-1]
	crcLocal := crc.CRC8_p93_2n(header, data)
	if crcIn != crcLocal {
		return 0, errors.New("talkavr: CRC8 mismatch")
	}
	p.Header = header
	p.Data = make([]byte, len(data))
	copy(p.Data, data)
	p.Crc8 = crcIn
	return length, nil
}

const (
	Command_Poll            byte = 0x01
	Command_Config          byte = 0x02
	Command_Reset           byte = 0x03
	Command_Debug           byte = 0x04
	Command_MDB_Bus_Reset   byte = 0x07
	Command_MDB_Transaction byte = 0x08
)

var Response_BeeBee = []byte{0xbe, 0xeb, 0xee}

const (
	// slave ok
	Response_OK          byte = 0x01
	Response_Config      byte = 0x02
	Response_Debug       byte = 0x04
	Response_TWI         byte = 0x05
	Response_MDB_Started byte = 0x08
	Response_MDB_Success byte = 0x09
	// slave error
	Response_Error              byte = 0x80
	Response_Bad_Packet         byte = 0x81
	Response_Invalid_CRC        byte = 0x82
	Response_Buffer_Overflow    byte = 0x83
	Response_Unknown_Command    byte = 0x84
	Response_Corruption         byte = 0x85
	Response_Not_Implemented    byte = 0x86
	Response_MDB_Busy           byte = 0x88
	Response_MDB_Protocol_Error byte = 0x89
	Response_MDB_Invalid_CHK    byte = 0x8a
	Response_MDB_NACK           byte = 0x8b
	Response_MDB_Timeout        byte = 0x8c
	Response_UART_Chatterbox    byte = 0x90
	Response_UART_Read_Error    byte = 0x91
)

func NewPacket(header byte, data []byte) Packet {
	p := Packet{
		Header: header,
		Data:   data,
	}
	p.Crc8 = crc.CRC8_p93_2n(header, data)
	return p
}

func CommandPollSimple() Packet {
	return NewPacket(Command_Poll, nil)
}

func CommandPollLimit(readLimit byte) Packet {
	return NewPacket(Command_Poll, []byte{readLimit})
}

func CommandConfig(data []byte) Packet {
	return NewPacket(Command_Config, data)
}

func CommandReset() Packet {
	return NewPacket(Command_Reset, nil)
}

func CommandDebug() Packet {
	return NewPacket(Command_Debug, nil)
}

func CommandMDBBusReset() Packet {
	return NewPacket(Command_MDB_Bus_Reset, nil)
}

func CommandMDBFlags(addChk, verifyChk, repeat bool, data []byte) Packet {
	// bit0 add auto CHK
	// bit1 verify response CHK
	// bit2 repeat on timeout
	header := Command_MDB_Transaction
	if addChk {
		header |= (1 << 0)
	}
	if verifyChk {
		header |= (1 << 1)
	}
	if repeat {
		header |= (1 << 2)
	}
	return NewPacket(header, data)
}

func CommandMDBFull(data []byte) Packet {
	return CommandMDBFlags(true, true, true, data)
}

func ParseResponse(data []byte) ([]Packet, error) {
	ps := make([]Packet, 0, 10)
	var p Packet
	// data[0] is total length of response
	offset := 1
	for {
		n, err := p.Read(data[offset:])
		if err != nil {
			return ps, err
		}
		offset += n
		ps = append(ps, p)
	}
}
