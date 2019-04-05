package ui

import (
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/engine"
)

type Menu map[uint16]MenuItem

type MenuItem struct {
	Code  uint16
	Name  string
	Price currency.Amount
	D     engine.Doer
}

func (self Menu) Add(code uint16, name string, price currency.Amount, d engine.Doer) {
	self[code] = MenuItem{code, name, price, d}
}

func (self Menu) MaxPrice() currency.Amount {
	max := currency.Amount(0)
	for _, item := range self {
		if item.Price > max {
			max = item.Price
		}
	}
	return max
}
