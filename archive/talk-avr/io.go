package talkavr

import (
	"errors"
	"sync"

	"github.com/temoto/vender/hardware/i2c"
)

type Slave struct {
	lk      sync.Mutex
	bus     i2c.I2CBus
	Address byte
}

func NewSlave(busNo byte, address byte) *Slave {
	return &Slave{
		bus:     i2c.NewI2CBus(busNo),
		Address: address,
	}
}

func (s *Slave) TwiTransaction(send, read []byte) error {
	// 4, self.Address<<1,
	if len(send) > 0 {
		// 7, len(send), *send
	}
	// 6, readLimit, 0
	if cap(read) > 0 && len(read) == 0 {
		// log debug info
		return errors.New("talkavr: TwiTranscation returned 0 bytes, expected more")
	}
	return nil
}

func (s *Slave) Talk(outps []Packet) ([]Packet, error) {
	s.lk.Lock()
	defer s.lk.Unlock()

	outbs := make([]byte, 0, 4*len(outps))
	for _, p := range outps {
		outbs = append(outbs, p.Bytes()...)
	}
	inbs := make([]byte, 0, len(outbs)+4)
	err := s.TwiTransaction(outbs, inbs)
	if err != nil {
		return nil, err
	}
	if len(inbs) < int(inbs[0]) {
		inbs = make([]byte, 0, inbs[0]+4)
		err = s.TwiTransaction(nil, inbs)
		if err != nil {
			return nil, err
		}
	}

	return ParseResponse(inbs)
}
