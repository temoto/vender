package engine_config

import (
	"fmt"

	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/engine"
)

type Config struct {
	Aliases        []Alias  `hcl:"alias"`
	OnStart        []string `hcl:"on_start"`
	OnMenuError    []string `hcl:"on_menu_error"`
	OnServiceBegin []string `hcl:"on_service_begin"`
	OnServiceEnd   []string `hcl:"on_service_end"`
	Inventory      Inventory
	Menu           struct {
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

type Inventory struct {
	Persist bool    `hcl:"persist"`
	Stocks  []Stock `hcl:"stock"`
}

type Stock struct { //nolint:maligned
	Name        string  `hcl:"name,key"`
	Check       bool    `hcl:"check"`
	Min         float32 `hcl:"min"`
	HwRate      float32 `hcl:"hw_rate"`
	SpendRate   float32 `hcl:"spend_rate"`
	RegisterAdd string  `hcl:"register_add"`
}

func (self *Stock) String() string {
	return fmt.Sprintf("inventory.%s check=%t hw_rate=%f spend_rate=%f min=%f",
		self.Name, self.Check, self.HwRate, self.SpendRate, self.Min)
}
