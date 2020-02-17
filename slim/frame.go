package slim

import (
	"bytes"
	"crypto/hmac"
	v2hash "crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/juju/errors"
)

var zeros [128]byte // used to destroy temporary secret

var (
	ErrFrameInvalid     = fmt.Errorf("frame is invalid")
	ErrFrameLenOverflow = fmt.Errorf("frame is too large")
	ErrWeakSecret       = fmt.Errorf("secret must be >= 8 bytes")
)

type ID = interface{}

type GetSecretFunc func(id ID, f *Frame) ([]byte, error)

const (
	V2Magic       = uint16(0x7302)
	V2HeaderFixed = 2 /*magic*/ + 2 /*total length*/ + 2 /*seq*/ + 1 /*flag*/
	V2SigSize     = v2hash.Size

	V2FlagSession   = byte(1 << 0)
	V2FlagAck       = byte(1 << 1)
	V2FlagSig       = byte(1 << 2)
	V2FlagKeepalive = byte(1 << 3) // no payload, skips application callback
)

// Frame binary representation: field:size in bytes
// magic:2 length:2 seq:2 flags:1 [session:8] [ackseq:2 ackbitmap:4] payload:var [sig:32]
// Payload length is calculated from total length and optional fields.
type Frame struct {
	Session   uint64
	Sig       uint64
	Payload   []byte
	GetSecret GetSecretFunc
	OpaqueID  ID // will be passed to GetSecret
	Acks      uint32
	Seq       uint16
	AckSeq    uint16
	Flags     byte

	length uint16 // optimize DecodeFixedHeader+Umarshal in stream
}

func (f *Frame) CheckFlag(x byte) bool { return f.Flags&x != 0 }
func (f *Frame) SetFlag(x byte)        { f.Flags |= x }

// Zero all fields except GetSecret.
// OpaqueID will be cleared.
func (f *Frame) Reset() {
	*f = Frame{GetSecret: f.GetSecret}
}

func (f *Frame) Marshal() ([]byte, error) {
	f.ImplyFlags()
	length := f.Size()
	if length > math.MaxUint16 {
		return nil, ErrFrameLenOverflow
	}
	buf := make([]byte, length)
	binary.BigEndian.PutUint16(buf[0:2], V2Magic)
	binary.BigEndian.PutUint16(buf[2:4], uint16(length))
	binary.BigEndian.PutUint16(buf[4:6], f.Seq)
	buf[6] = f.Flags
	authLen := uint16(7)
	if f.CheckFlag(V2FlagSession) {
		binary.BigEndian.PutUint64(buf[authLen:], f.Session)
		authLen += 8
	}
	if f.CheckFlag(V2FlagAck) {
		binary.BigEndian.PutUint16(buf[authLen:], f.AckSeq)
		binary.BigEndian.PutUint32(buf[authLen+2:], f.Acks)
		authLen += 6
	}
	// log.Printf("frame.Marshal beforeP buf=(%d/%d)%x", len(buf), cap(buf), buf)
	plen := copy(buf[authLen:], f.Payload)
	authLen += uint16(plen)
	// log.Printf("frame.Marshal beforeS buf=(%d/%d)%x", len(buf), cap(buf), buf)
	if f.CheckFlag(V2FlagSig) {
		sig, err := f.sign(buf[:authLen])
		if err != nil {
			return nil, errors.Annotate(err, "sign")
		} else {
			copy(buf[authLen:], sig)
		}
	}
	// log.Printf("frame.Marshal signed  buf=(%d/%d)%x", len(buf), cap(buf), buf)
	return buf, nil
}

func (f *Frame) DecodeFixedHeader(b []byte) error {
	if len(b) < V2HeaderFixed {
		return errors.Annotate(io.ErrUnexpectedEOF, "header fixed")
	}
	magic := binary.BigEndian.Uint16(b[0:2])
	if magic != V2Magic {
		return errors.Annotate(ErrFrameInvalid, "wrong magic")
	}
	f.length = binary.BigEndian.Uint16(b[2:4])
	if f.length < V2HeaderFixed {
		return errors.Annotatef(ErrFrameInvalid, "length=%d", f.length)
	}
	f.Seq = binary.BigEndian.Uint16(b[4:6])
	f.Flags = b[6]
	// log.Printf("DecodeFixedHeader b=(%d/%d)%x magic=%04x length=%d seq=%d flags=%02x", len(b), cap(b), b, magic, f.length, f.Seq, f.Flags)
	return nil
}

// If error != nil, frame is likely in broken state.
func (f *Frame) Unmarshal(buf []byte) error {
	// log.Printf("unmarshal buf=(%d/%d)%x", len(buf), cap(buf), buf)
	if f.length == 0 {
		if err := f.DecodeFixedHeader(buf); err != nil {
			return err
		}
	}
	if int(f.length) > len(buf) {
		return errors.Annotatef(io.ErrUnexpectedEOF, "length=%d", f.length)
	}
	authLen := uint16(V2HeaderFixed)
	b := buf[7:f.length]
	// log.Printf("unmarshal var b=(%d/%d)%x", len(b), cap(b), b)
	if f.CheckFlag(V2FlagSession) {
		f.Session = binary.BigEndian.Uint64(b)
		b = b[8:]
		authLen += 8
	}
	if f.CheckFlag(V2FlagAck) {
		f.AckSeq = binary.BigEndian.Uint16(b)
		f.Acks = binary.BigEndian.Uint32(b[2:])
		b = b[6:]
		authLen += 6
	}
	plen := uint16(f.length - authLen)
	if f.CheckFlag(V2FlagSig) {
		plen -= V2SigSize
	}
	authLen += plen
	// log.Printf("unmarshal payload b=(%d/%d)%x plen=%d authlen=%d", len(b), cap(b), b, plen, authLen)
	f.Payload = b[:plen]

	if f.CheckFlag(V2FlagSig) {
		if int(authLen+V2SigSize) > len(buf) {
			return errors.Annotate(io.ErrUnexpectedEOF, "signature")
		}
		declared := buf[authLen : authLen+V2SigSize]
		actual, err := f.sign(buf[:authLen])
		// log.Printf("unmarshal declared=(%d)%x actual=(%d)%x", len(declared), declared, len(actual), actual)
		if err != nil {
			return errors.Annotate(err, "actual sig")
		} else if !bytes.Equal(actual, declared) {
			return errors.NotValidf("signature")
		}
	}
	return nil
}

func (f *Frame) Size() int {
	s := int(V2HeaderFixed)
	if f.CheckFlag(V2FlagSession) {
		s += 8
	}
	if f.CheckFlag(V2FlagAck) {
		s += 6
	}
	s += len(f.Payload)
	if f.CheckFlag(V2FlagSig) {
		s += V2SigSize
	}
	return s
}

func (f *Frame) String() string {
	b := strings.Builder{}
	b.WriteString(fmt.Sprintf("(seq=%d flags=", f.Seq))
	if f.CheckFlag(V2FlagSession) {
		b.WriteByte('i')
	}
	if f.CheckFlag(V2FlagAck) {
		b.WriteByte('a')
	}
	if f.CheckFlag(V2FlagSig) {
		b.WriteByte('s')
	}
	if f.CheckFlag(V2FlagKeepalive) {
		b.WriteByte('k')
	}

	if f.Session != 0 || f.CheckFlag(V2FlagSession) {
		b.WriteString(fmt.Sprintf(" session=%x", f.Session))
	}
	if f.AckSeq != 0 || f.Acks != 0 || f.CheckFlag(V2FlagAck) {
		b.WriteString(fmt.Sprintf(" ackseq=%d acks=%x", f.AckSeq, f.Acks))
	}

	b.WriteString(fmt.Sprintf(" payload=(%d)%x)", len(f.Payload), f.Payload))

	return b.String()
}

func (f *Frame) ImplyFlags() {
	if f.Session != 0 {
		f.SetFlag(V2FlagSession)
	}
	if f.AckSeq != 0 || f.Acks != 0 {
		f.SetFlag(V2FlagAck)
	}
}

func (f *Frame) sign(b []byte) ([]byte, error) {
	if f.GetSecret == nil {
		return nil, errors.Errorf("code error frame.GetSecret is not set")
	}
	secret, err := f.GetSecret(f.OpaqueID, f)
	if err != nil {
		return nil, errors.Annotate(err, "getsecret")
	}
	sig, err := V2Sign(b, secret)
	copy(secret, zeros[:]) // destroy secret ASAP
	return sig, err
}

// Returns HMAC-SHA256 of data.
// `secret` must be at least 8 bytes.
func V2Sign(data, secret []byte) ([]byte, error) {
	if len(secret) < 8 {
		return nil, ErrWeakSecret
	}
	h := hmac.New(v2hash.New, secret)
	if _, err := h.Write(data); err != nil {
		return nil, err
	}
	var buf [V2SigSize]byte
	return h.Sum(buf[:0]), nil
}
