package kitchen

import (
	"context"
	"log"
	"sync"

	"github.com/temoto/vender/hardware/mdb/evend"
)

type KitchenSystem struct {
	lk     sync.Mutex
	events chan Event
	// TODO interfaces, be ready to swap devices performing same functions
	devCoffee   *evend.DeviceCoffee
	devConveyor *evend.DeviceConveyor
	devCup      *evend.DeviceCup
	devElevator *evend.DeviceElevator
	devHopper   *evend.DeviceHopper
	devMixer    *evend.DeviceMixer
	devValve    *evend.DeviceValve
}

func (self *KitchenSystem) String() string { return "kitchen" }
func (self *KitchenSystem) Start(ctx context.Context) error {
	self.lk.Lock()
	defer self.lk.Unlock()
	if self.events != nil {
		panic("double Start()")
	}

	// TODO read config

	self.events = make(chan Event, 2)

	self.devCoffee = new(evend.DeviceCoffee)
	if err := self.devCoffee.Init(ctx); err != nil {
		self.devCoffee = nil
		log.Printf("unable to init kitchen device coffee: %v", err)
	}

	self.devConveyor = new(evend.DeviceConveyor)
	if err := self.devConveyor.Init(ctx); err != nil {
		self.devConveyor = nil
		log.Printf("unable to init kitchen device conveyor: %v", err)
	}

	self.devCup = new(evend.DeviceCup)
	if err := self.devCup.Init(ctx); err != nil {
		self.devCup = nil
		log.Printf("unable to init kitchen device cup: %v", err)
	}

	self.devElevator = new(evend.DeviceElevator)
	if err := self.devElevator.Init(ctx); err != nil {
		self.devElevator = nil
		log.Printf("unable to init kitchen device elevator: %v", err)
	}

	self.devHopper = new(evend.DeviceHopper)
	if err := self.devHopper.Init(ctx); err != nil {
		self.devHopper = nil
		log.Printf("unable to init kitchen device hopper: %v", err)
	}

	self.devMixer = new(evend.DeviceMixer)
	if err := self.devMixer.Init(ctx); err != nil {
		self.devMixer = nil
		log.Printf("unable to init kitchen device mixer: %v", err)
	}

	self.devValve = new(evend.DeviceValve)
	if err := self.devValve.Init(ctx); err != nil {
		self.devValve = nil
		log.Printf("unable to init kitchen device valve: %v", err)
	}

	return nil
}
func (self *KitchenSystem) Validate(ctx context.Context) error { return nil }
func (self *KitchenSystem) Stop(ctx context.Context) error     { return nil }
