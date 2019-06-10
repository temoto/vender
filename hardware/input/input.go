// Abstract input events
package input

import (
	"fmt"
	"sync"

	"github.com/juju/errors"
	"github.com/temoto/vender/log2"
)

type Key uint16

type Event struct {
	Source string
	Key    Key
	Up     bool
}

func (e *Event) IsZero() bool  { return e.Key == 0 }
func (e *Event) IsDigit() bool { return e.Key >= '0' && e.Key <= '9' }

func Drain(ch <-chan Event) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

type Source interface {
	Read() (Event, error)
	String() string
}

type EventFunc func(Event)
type sub struct {
	name string
	ch   chan<- Event
	fun  EventFunc
	stop <-chan struct{}
}

type Dispatch struct {
	Log  *log2.Log
	bus  chan Event
	mu   sync.Mutex
	subs map[string]*sub
	stop <-chan struct{}
}

func NewDispatch(log *log2.Log, stop <-chan struct{}) *Dispatch {
	return &Dispatch{
		Log:  log,
		bus:  make(chan Event),
		subs: make(map[string]*sub, 16),
		stop: stop,
	}
}

func (self *Dispatch) SubscribeChan(name string, substop <-chan struct{}) chan Event {
	target := make(chan Event)
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

func (self *Dispatch) Run(sources []Source) {
	for _, source := range sources {
		go self.readSource(source)
	}

	for {
		select {
		case event := <-self.bus:
			self.mu.Lock()
			for _, sub := range self.subs {
				self.subFire(sub, event)
			}
			self.mu.Unlock()

		case <-self.stop:
			Drain(self.bus)
			return
		}
	}
}

func (self *Dispatch) Emit(event Event) {
	select {
	case self.bus <- event:
	case <-self.stop:
		return
	}
}

func (self *Dispatch) subFire(sub *sub, event Event) {
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
	if _, ok := self.subs[s.name]; ok {
		panic("code error input duplicate subscribe name=" + s.name)
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
		self.Emit(event)
	}
}
