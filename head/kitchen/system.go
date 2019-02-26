package kitchen

import (
	"context"
	"strconv"
	"sync"

	"github.com/temoto/vender/hardware/mdb/evend"
	"github.com/temoto/vender/log2"
)

type KitchenSystem struct {
	log *log2.Log
	lk  sync.Mutex
	// TODO interfaces, be ready to swap devices performing same functions
	devCoffee   *evend.DeviceCoffee
	devConveyor *evend.DeviceConveyor
	devCup      *evend.DeviceCup
	devElevator *evend.DeviceElevator
	devHoppers  [8]*evend.DeviceHopper
	devMixer    *evend.DeviceMixer
	devValve    *evend.DeviceValve
}

func (self *KitchenSystem) String() string { return "kitchen" }
func (self *KitchenSystem) Start(ctx context.Context) error {
	self.lk.Lock()
	defer self.lk.Unlock()

	// TODO read config
	self.log = log2.ContextValueLogger(ctx, log2.ContextKey)

	self.devCoffee = new(evend.DeviceCoffee)
	if err := self.devCoffee.Init(ctx); err != nil {
		self.devCoffee = nil
		self.log.Errorf("unable to init kitchen device coffee: %v", err)
	}

	self.devConveyor = new(evend.DeviceConveyor)
	if err := self.devConveyor.Init(ctx); err != nil {
		self.devConveyor = nil
		self.log.Errorf("unable to init kitchen device conveyor: %v", err)
	}

	self.devCup = new(evend.DeviceCup)
	if err := self.devCup.Init(ctx); err != nil {
		self.devCup = nil
		self.log.Errorf("unable to init kitchen device cup: %v", err)
	}

	self.devElevator = new(evend.DeviceElevator)
	if err := self.devElevator.Init(ctx); err != nil {
		self.devElevator = nil
		self.log.Errorf("unable to init kitchen device elevator: %v", err)
	}

	for i := range self.devHoppers {
		self.devHoppers[i] = new(evend.DeviceHopper)
		addr := uint8(0x40 + i*8)
		if err := self.devHoppers[i].Init(ctx, addr, strconv.Itoa(i+1)); err != nil {
			self.devHoppers[i] = nil
			self.log.Errorf("unable to init kitchen device hopper: %v", err)
		}
	}

	self.devMixer = new(evend.DeviceMixer)
	if err := self.devMixer.Init(ctx); err != nil {
		self.devMixer = nil
		self.log.Errorf("unable to init kitchen device mixer: %v", err)
	}

	self.devValve = new(evend.DeviceValve)
	if err := self.devValve.Init(ctx); err != nil {
		self.devValve = nil
		self.log.Errorf("unable to init kitchen device valve: %v", err)
	}

	return nil
}
func (self *KitchenSystem) Validate(ctx context.Context) error { return nil }
func (self *KitchenSystem) Stop(ctx context.Context) error     { return nil }
