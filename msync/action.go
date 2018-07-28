package msync

import (
	"errors"
	"fmt"
	"sync"
)

type Stateful interface {
	Start(args interface{})
	Pause()
	Abort() error
	ErrorWaiter
	Chan() chan error
}

type Doable interface {
	fmt.Stringer
	Stateful
}

type ActionFunc func(w *MultiWait, args interface{}) error
type Action struct {
	Run  ActionFunc
	Name string
	w    *MultiWait
}

var Abort = errors.New("Abort")
var actionMap = map[string]*Action{}
var actionMapLock sync.Mutex

func NewAction(name string, run ActionFunc) *Action {
	return &Action{
		Name: name,
		Run:  run,
	}
}

func (a Action) String() string {
	if a.Name == "" {
		a.Name = "Action"
	}
	return a.Name
}

func (a *Action) Start(args interface{}) {
	a.w = NewMultiWait()
	go func() {
		a.w.Done(a.Run(a.w, args))
	}()
}

func (*Action) Pause() { panic("base Action.Pause() does nothing") }

func (a *Action) Abort() error {
	a.w.Done(Abort)
	return a.Wait()
}

func (a *Action) Wait() error { return a.w.WaitDone() }

func (a *Action) Chan() chan error {
	ch := make(chan error)
	go func() {
		ch <- a.w.WaitDone()
	}()
	return ch
}

func (a *Action) RegisterGlobal() {
	if a.Name == "" {
		panic("Cannot register action with empty Name")
	}
	actionMapLock.Lock()
	defer actionMapLock.Unlock()
	if _, ok := actionMap[a.Name]; ok {
		panic("There is already action registered for name '" + a.Name + "'")
	}
	actionMap[a.Name] = a
}

func MustGlobalAction(name string) *Action {
	actionMapLock.Lock()
	defer actionMapLock.Unlock()
	a, ok := actionMap[name]
	if !ok {
		panic("There is no action registered for name '" + name + "'")
	}
	return a
}
