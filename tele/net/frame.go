package telenet

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"

	"github.com/golang/protobuf/proto"
	"github.com/juju/errors"
	"github.com/temoto/vender/tele"
)

var (
	ErrFrameInvalid     = fmt.Errorf("frame is invalid")
	ErrFrameLenOverflow = fmt.Errorf("frame is too large")
)

// Frame wraps Packet with header
const (
	FrameV2Magic      = uint16(0x7602)
	FrameV2HeaderSize = 2 /*magic*/ + 2 /*length*/ + 1 /*flag*/

	FrameV2FlagHmac = byte(0x01)
)

func FrameMarshal(f *tele.Frame, secret []byte) ([]byte, error) {
	flen := FrameV2HeaderSize
	withHmac := false
	if f.Packet != nil && (f.Packet.Hello || f.Packet.AuthId != "") {
		withHmac = true
		flen += 8
	}
	flen += proto.Size(f)
	if flen >= math.MaxUint16 {
		return nil, ErrFrameLenOverflow
	}
	b := make([]byte, FrameV2HeaderSize, flen)
	pbuf := proto.NewBuffer(b)
	if err := pbuf.Marshal(f); err != nil {
		return nil, err
	}
	b = pbuf.Bytes()
	binary.BigEndian.PutUint16(b[0:], FrameV2Magic)
	binary.BigEndian.PutUint16(b[2:], uint16(flen))
	log.Printf("Frame.Marshal beforeHM b=(%d/%d)%x", len(b), cap(b), b)
	if withHmac {
		b[4] |= FrameV2FlagHmac
		hmac, err := Auth1(b, secret)
		if err != nil {
			return nil, errors.Annotate(err, "auth1")
		}
		var hmacbs [8]byte
		binary.BigEndian.PutUint64(hmacbs[:], hmac)
		b = append(b, hmacbs[:]...)
	}
	log.Printf("Frame.Marshal frame=%s b=(%d/%d)%x", f, len(b), cap(b), b)
	return b, nil
}

func FrameUnmarshal(b []byte, frame *tele.Frame, getSecret func(*tele.Frame) ([]byte, error)) error {
	if len(b) < FrameV2HeaderSize {
		return errors.Annotate(io.ErrUnexpectedEOF, "header")
	}
	if magic := binary.BigEndian.Uint16(b[0:]); magic != FrameV2Magic {
		return ErrFrameInvalid
	}
	frameLen := binary.BigEndian.Uint16(b[2:])
	if frameLen < FrameV2HeaderSize {
		return errors.Errorf("frameLen=%d invalid", frameLen)
	}
	flag := b[4]

	authLen := 0
	pbuf := b[FrameV2HeaderSize:]
	// log.Printf("decoder# pbuf=(%d)%x", len(pbuf), pbuf)
	if flag&FrameV2FlagHmac != 0 {
		if getSecret == nil {
			return errors.Errorf("code error getSecret is not set")
		}
		if authLen = len(b) - 8; authLen <= 0 {
			return errors.Errorf("missing hmac")
		}
		pbuf = b[FrameV2HeaderSize:authLen]
	}
	// log.Printf("decoder: pbuf=(%d)%x", len(pbuf), pbuf)
	if err := proto.Unmarshal(pbuf, frame); err != nil {
		// log.Printf("decoder: frame=%s err=%v", frame, err)
		return errors.Annotate(err, "unmarshal")
	}
	if flag&FrameV2FlagHmac != 0 {
		declared := binary.BigEndian.Uint64(b[authLen:])
		secret, err := getSecret(frame)
		if err != nil {
			return errors.Annotate(err, "getsecret")
		}
		actual, err := Auth1(b[:authLen], secret)
		// log.Printf("decoder: auth=(%d)%x declared=%016x actual=%016x", authLen, b[:authLen], declared, actual)
		if err != nil {
			return errors.Annotate(err, "actual hmac")
		} else if declared != actual {
			return errors.Errorf("invalid hmac")
		}
	}
	return nil
}

func NewPacketHello(timestamp int64, authid string, vmid tele.VMID) tele.Packet {
	return tele.Packet{
		AuthId: authid,
		VmId:   int32(vmid),
		Time:   timestamp,
		Hello:  true,
	}
}

func NewFrame(seq uint16, p *tele.Packet) *tele.Frame {
	return &tele.Frame{
		Seq:    uint32(seq),
		Packet: p,
	}
}
