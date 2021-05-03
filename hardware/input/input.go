// Abstract input events
package input

import (
	"fmt"
	"sync"

	"github.com/juju/errors"
	"github.com/temoto/vender/internal/global"
	"github.com/temoto/vender/internal/types"
	"github.com/temoto/vender/log2"
)

func Drain(ch <-chan types.InputEvent) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

type Source interface {
	Read() (types.InputEvent, error)
	String() string
}

type EventFunc func(types.InputEvent)
type sub struct {
	name string
	ch   chan<- types.InputEvent
	fun  EventFunc
	stop <-chan struct{}
}

type Dispatch struct {
	Log  *log2.Log
	bus  chan types.InputEvent
	mu   sync.Mutex
	subs map[string]*sub
	stop <-chan struct{}
}

func NewDispatch(log *log2.Log, stop <-chan struct{}) *Dispatch {
	return &Dispatch{
		Log:  log,
		bus:  make(chan types.InputEvent),
		subs: make(map[string]*sub, 16),
		stop: stop,
	}
}

func (self *Dispatch) Enable(e bool) {
	global.GBL.HW.EvendInput = e
	global.Log.Infof("evendInput = %v", e)
}

func (self *Dispatch) SubscribeChan(name string, substop <-chan struct{}) chan types.InputEvent {
	target := make(chan types.InputEvent)
	sub := &sub{
		name: name,
		ch:   target,
		stop: substop,
	}
	self.safeSubscribe(sub)
	return target
}

func (self *Dispatch) SubscribeFunc(name string, fun EventFunc, substop <-chan struct{}) {
	sub := &sub{
		name: name,
		fun:  fun,
		stop: substop,
	}
	self.safeSubscribe(sub)
}

func (self *Dispatch) Unsubscribe(name string) {
	self.mu.Lock()
	defer self.mu.Unlock()
	if sub, ok := self.subs[name]; ok {
		self.subClose(sub)
	} else {
		panic("code error input sub not found name=" + name)
	}
}

func (self *Dispatch) Run(sources []Source) {
	for _, source := range sources {
		go self.readSource(source)
	}

	for {
		select {
		case event := <-self.bus:
			handled := false
			self.mu.Lock()
			for _, sub := range self.subs {
				self.subFire(sub, event)
				handled = true
			}
			self.mu.Unlock()
			if !handled {
				// TODO emit sound/etc notification
				self.Log.Errorf("input is not handled event=%#v", event)
			}

		case <-self.stop:
			Drain(self.bus)
			return
		}
	}
}

func (self *Dispatch) Emit(event types.InputEvent) {
	select {
	case self.bus <- event:
		self.Log.Debugf("input emit=%#v", event)
	case <-self.stop:
		return
	}
}

func (self *Dispatch) subFire(sub *sub, event types.InputEvent) {
	select {
	case <-sub.stop:
		self.subClose(sub)
		return
	default:
	}

	if sub.ch == nil && sub.fun == nil {
		panic(fmt.Sprintf("input sub=%s ch=nil fun=nil", sub.name))
	}
	if sub.fun != nil {
		sub.fun(event)
	}
	if sub.ch != nil {
		select {
		case sub.ch <- event:
		case <-sub.stop:
			self.subClose(sub)
		}
	}
}

func (self *Dispatch) subClose(s *sub) {
	if s.ch != nil {
		close(s.ch)
	}
	delete(self.subs, s.name)
}

func (self *Dispatch) safeSubscribe(s *sub) {
	self.mu.Lock()
	if existing, ok := self.subs[s.name]; ok {
		select {
		case <-s.stop:
			panic("code error input subscribe already closed name=" + s.name)
		case <-existing.stop:
			self.subClose(existing)
		default:
			panic("code error input duplicate subscribe name=" + s.name)
		}
	}
	self.subs[s.name] = s
	self.mu.Unlock()
}

func (self *Dispatch) readSource(source Source) {
	tag := source.String()
	for {
		event, err := source.Read()
		if err != nil {
			err = errors.Annotatef(err, "input source=%s", tag)
			self.Log.Fatal(errors.ErrorStack(err))
		}
		if global.GBL.HW.EvendInput || event.Source == "dev-input-event" {
			self.Emit(event)
		} else {
			self.Log.Debugf("keyboard disable. ignore event =%#v", event)
		}
	}
}
