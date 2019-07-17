package engine_config

import (
	"fmt"

	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/engine"
)

type Config struct {
	Aliases     []Alias  `hcl:"alias"`
	OnStart     []string `hcl:"on_start"`
	OnMenuError []string `hcl:"on_menu_error"`
	Inventory   struct {
		Stocks []StockItem `hcl:"stock"`
	}
	Menu struct {
		Items []*MenuItem `hcl:"item"`
	}
}

type Alias struct {
	Name     string `hcl:"name,key"`
	Scenario string `hcl:"scenario"`

	Doer engine.Doer `hcl:"-"`
}

type MenuItem struct {
	Code      string `hcl:"code,key"`
	Name      string `hcl:"name"`
	XXX_Price int    `hcl:"price"` // use scaled `Price`, this is for decoding config only
	Scenario  string `hcl:"scenario"`

	Price currency.Amount `hcl:"-"`
	Doer  engine.Doer     `hcl:"-"`
}

func (self *MenuItem) String() string { return fmt.Sprintf("menu.%s %s", self.Code, self.Name) }

type StockItem struct { //nolint:maligned
	Name     string   `hcl:"name,key"`
	Disabled bool     `hcl:"disable"`
	Min      int32    `hcl:"min"`
	Rate     float32  `hcl:"rate"`
	Sources  []string `hcl:"sources"`
	Strategy string   `hcl:"strategy"` // even|order
	Register string   `hcl:"register"`
}

func (self *StockItem) String() string {
	return fmt.Sprintf("inventory.%s enabled=%t rate=%f min=%d", self.Name, !self.Disabled, self.Rate, self.Min)
}
