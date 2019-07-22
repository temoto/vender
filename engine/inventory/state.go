package inventory

import (
	"github.com/golang/protobuf/proto"
	"github.com/temoto/errors"
	"github.com/temoto/vender/state/persist"
)

//go:generate protoc --go_out=./ state.proto

func (self *Inventory) UnmarshalBinary(b []byte) error {
	var state State
	if err := proto.Unmarshal(b, &state); err != nil {
		return errors.Trace(err)
	}
	self.mu.Lock()
	defer self.mu.Unlock()
	for _, stockState := range state.Stocks {
		if stock, ok := self.ss[stockState.Name]; ok {
			if stockState.Enabled {
				stock.Enable()
			} else {
				stock.Disable()
			}
			stock.Set(stockState.Value)
		}
	}
	return nil
}

func (self *Inventory) MarshalBinary() ([]byte, error) {
	self.mu.RLock()
	defer self.mu.RUnlock()
	state := State{Stocks: make([]*State_Stock, 0, len(self.ss))}
	for _, stock := range self.ss {
		state.Stocks = append(state.Stocks, &State_Stock{
			Name:    stock.Name,
			Enabled: stock.Enabled(),
			Value:   stock.Value(),
		})
	}
	return proto.Marshal(&state)
}

var _ persist.Stater = &Inventory{}
