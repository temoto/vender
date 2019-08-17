package ui

import (
	"context"
	"fmt"
	"strconv"

	"github.com/juju/errors"
	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
	"github.com/temoto/vender/state"
)

type Menu map[uint16]MenuItem

type MenuItem struct {
	Name  string
	D     engine.Doer
	Price currency.Amount
	Code  uint16
}

func (self *MenuItem) String() string {
	return fmt.Sprintf("menu code=%d price=%d(raw) name='%s'", self.Code, self.Price, self.Name)
}

func (self Menu) Init(ctx context.Context) error {
	config := state.GetGlobal(ctx).Config

	errs := make([]error, 0, 16)
	for _, x := range config.Engine.Menu.Items {
		codeInt, _ := strconv.Atoi(x.Code)
		self.Add(uint16(codeInt), x.Name, x.Price, x.Doer)
	}
	return helpers.FoldErrors(errs)
}

func (self Menu) Add(code uint16, name string, price currency.Amount, d engine.Doer) {
	self[code] = MenuItem{
		Code:  code,
		Name:  name,
		Price: price,
		D:     d,
	}
}

func (self Menu) MaxPrice(log *log2.Log) (currency.Amount, error) {
	max := currency.Amount(0)
	empty := true
	for _, item := range self {
		valErr := item.D.Validate()
		if valErr == nil {
			empty = false
			if item.Price > max {
				max = item.Price
			}
		} else {
			log.Errorf("menu valerr=%v %s", valErr, item.String())
		}
	}
	if empty {
		return 0, errors.Errorf("menu len=%d no valid items", len(self))
	}
	return max, nil
}
