package telenet

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/golang/protobuf/proto"
	"github.com/juju/errors"
	"github.com/temoto/vender/tele"
)

var (
	ErrFrameInvalid             = fmt.Errorf("frame is invalid")
	ErrFrameLargeNotImplemented = fmt.Errorf("large frame support is not implemented yet")
)

// Frame wraps Packet with header: magic number and length
const (
	FrameV2MagicSmall = uint16(0x7602)
	FrameV2MagicLarge = uint16(0x5602)

	FrameV2HeaderSizeSmall = 2 + 2 // uint16 magic + uint16 length
	FrameV2HeaderSizeLarge = 2 + 4 // uint16 magic + uint32 length
)

func FrameDecode(b []byte, max uint32) (magic uint16, frameLen uint32, err error) {
	if len(b) == 0 {
		return 0, 0, io.EOF
	}
	if len(b) < FrameV2HeaderSizeSmall {
		return 0, 0, io.ErrUnexpectedEOF
	}
	magic = binary.BigEndian.Uint16(b)
	switch magic {
	case FrameV2MagicSmall:
		frameLen = uint32(binary.BigEndian.Uint16(b[2:]))

	case FrameV2MagicLarge:
		frameLen = binary.BigEndian.Uint32(b[2:])

	default:
		return 0, 0, ErrFrameInvalid
	}
	if frameLen > max {
		return magic, frameLen, errors.Errorf("frameLen=%d exceeds max=%d", frameLen, max)
	}
	return magic, frameLen, nil
}

func FrameMarshal(p *tele.Packet) ([]byte, error) {
	plen := proto.Size(p)
	if plen >= math.MaxUint16 {
		return nil, ErrFrameLargeNotImplemented
	}
	b := make([]byte, FrameV2HeaderSizeSmall, FrameV2HeaderSizeLarge+plen)
	pbuf := proto.NewBuffer(b)
	err := pbuf.Marshal(p)
	if err != nil {
		return nil, err
	}
	b = pbuf.Bytes()
	// log.Printf("FrameMarshal plen=%d b=(%d)%x", plen, len(b), b)
	binary.BigEndian.PutUint16(b, FrameV2MagicSmall)
	binary.BigEndian.PutUint16(b[2:], uint16(plen))
	return b, nil
}

func NewPacketHello(seq uint32, timestamp int64, authid string, vmid tele.VMID, secret []byte) (tele.Packet, error) {
	p := tele.Packet{
		Seq:    seq,
		AuthId: authid,
		VmId:   int32(vmid),
		Time:   timestamp,
		Hello:  true,
	}
	var err error
	p.Auth1, err = Auth1(&p, secret)
	if err != nil {
		return tele.Packet{}, errors.Annotate(err, "auth1")
	}
	return p, nil
}

func NewPacketAck(origin *tele.Packet) tele.Packet {
	return tele.Packet{
		Seq: origin.Seq,
		Ack: true,
	}
}
