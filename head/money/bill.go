package money

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/temoto/alive"
	"github.com/temoto/vender/hardware/mdb"
)

type BillState struct {
	lk     sync.Mutex
	alive  *alive.Alive
	bank   NominalGroup
	escrow NominalGroup
	mdb    mdb.Mdber
}

var (
	bill BillState
)

func (self *BillState) Init(ctx context.Context) error {
	self.lk.Lock()
	defer self.lk.Unlock()

	log.Printf("bill init")

	m, err := mdb.NewMDB(mdb.NewFileUart(), "/dev/ttyAMA0", 9600)
	if err != nil {
		return err
	}
	m.SetDebug(true)
	m.BreakCustom(200, 500)
	self.mdb = m

	self.alive = alive.NewAlive()
	self.alive.Add(1)
	go self.Loop(ctx)
	return nil
}

func (self *BillState) Loop(ctx context.Context) {
	defer self.alive.Done()
	defer self.alive.Stop()

	for self.alive.IsRunning() {
		time.Sleep(300 * time.Millisecond)
	}
}

func (self *BillState) Stop(ctx context.Context) {
	self.alive.Stop()
	self.alive.Wait()
}

func (self *BillState) MdbPoll() error {
	request := mdb.PacketFromBytes([]byte{0x33})
	response := new(mdb.Packet)
	err := self.mdb.Tx(request, response)
	if err != nil {
		log.Printf("bill init err: %s", err)
		return err
	}
	log.Printf("bill poll: %s", response.Format())
	return nil
}
