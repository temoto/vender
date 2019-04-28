package ui

import (
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/engine"
)

type Menu map[uint16]MenuItem

type MenuItem struct {
	Name  string
	D     engine.Doer
	Price currency.Amount
	Code  uint16
}

func (self Menu) Add(code uint16, name string, price currency.Amount, d engine.Doer) {
	self[code] = MenuItem{
		Code:  code,
		Name:  name,
		Price: price,
		D:     d,
	}
}

func (self Menu) MaxPrice() currency.Amount {
	max := currency.Amount(0)
	for _, item := range self {
		if (item.D.Validate() == nil) && (item.Price > max) {
			max = item.Price
		}
	}
	return max
}
