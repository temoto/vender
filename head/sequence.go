package main

import (
	"fmt"
	"log"
	"sync"
)

type SeqMsgTag int

const (
	SeqMsgInvalid SeqMsgTag = iota
	SeqMsgState
	SeqMsgActionDone
)

func (t SeqMsgTag) String() string {
	switch t {
	case SeqMsgInvalid:
		return "invalid"
	case SeqMsgState:
		return "set-state"
	case SeqMsgActionDone:
		return "action-done"
	}
	panic(fmt.Sprintf("Invalid SeqMsgTag value %d", t))
}

type SeqMsg struct {
	Tag  SeqMsgTag
	Data interface{}
	Wait SignalError
}

type SeqState int

const (
	SeqStateInvalid SeqState = iota
	SeqStatePause
	SeqStateRun
	SeqStateEnd
	SeqStateError
)

func (s SeqState) String() string {
	switch s {
	case SeqStateInvalid:
		return "invalid"
	case SeqStatePause:
		return "pause"
	case SeqStateRun:
		return "run"
	case SeqStateEnd:
		return "end"
	case SeqStateError:
		return "error"
	}
	panic(fmt.Sprintf("Invalid SeqState value %d", s))
}

type Sequence struct {
	lk      sync.Mutex
	cin     chan SeqMsg
	cout    chan SeqMsg
	state   SeqState
	actions []Doable
	current Doable
	w       *MultiWait
	name    string
}

func NewSequence(name string) *Sequence {
	s := &Sequence{
		cin:     make(chan SeqMsg, 100),
		cout:    make(chan SeqMsg, 100),
		state:   SeqStatePause,
		actions: make([]Doable, 0, 4),
		w:       NewMultiWait(),
		name:    name,
	}
	return s
}

func (s *Sequence) run() {
	s.lk.Lock()
	actch := make(chan Doable, len(s.actions))
	for _, a := range s.actions {
		actch <- a
	}
	s.lk.Unlock()
	close(actch)

seq:
	for a := range actch {
	waitrun:
		for {
			switch s.State() {
			case SeqStateEnd, SeqStateError:
				break seq
			case SeqStateRun:
				break waitrun
			}
			s.w.WaitTouch()
		}

		log.Printf("sequence %s next action %s", s.name, a.String())
		a.Start(nil)
		s.current = a
		a.Wait()
		s.current = nil
	}

	s.SetState(SeqStateEnd)
	s.w.Done(nil)
}

func (s *Sequence) Append(action Doable) {
	s.lk.Lock()
	s.actions = append(s.actions, action)
	s.lk.Unlock()
}

func (s *Sequence) Current() (out Doable) {
	s.lk.Lock()
	out = s.current
	s.lk.Unlock()
	return
}

func (s *Sequence) Start() {
	s.lk.Lock()
	if s.state != SeqStatePause {
		s.w.Reset()
	}
	s.lk.Unlock()
	go s.run()
	s.SetState(SeqStateRun)
	s.w.Touch()
}

func (s *Sequence) Pause() {
	s.SetState(SeqStatePause)
	s.w.Touch()
}

func (s *Sequence) Abort() error {
	s.SetState(SeqStateError)
	current := s.Current()
	if current != nil {
		current.Abort()
	}
	s.w.Touch()
	return s.w.WaitDone()
}

func (s *Sequence) Wait() error {
	s.w.WaitDone()
	return nil
}

func (s *Sequence) String() string { return s.name }

func (s *Sequence) State() (out SeqState) {
	s.lk.Lock()
	out = s.state
	s.lk.Unlock()
	return
}
func (s *Sequence) SetState(next SeqState) {
	s.lk.Lock()
	s.state = next
	s.lk.Unlock()
}
