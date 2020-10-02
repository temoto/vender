package engine_config

import (
	"fmt"

	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/internal/engine"
)

type Config struct {
	Aliases        []Alias  `hcl:"alias"`
	OnBoot         []string `hcl:"on_boot"`
	OnMenuError    []string `hcl:"on_menu_error"`
	OnServiceBegin []string `hcl:"on_service_begin"`
	OnServiceEnd   []string `hcl:"on_service_end"`
	OnFrontBegin   []string `hcl:"on_front_begin"`
	OnBroken       []string `hcl:"on_broken"`
	Inventory      Inventory
	Menu           struct {
		Items []*MenuItem `hcl:"item"`
	}
	Profile struct {
		Regexp    	string `hcl:"regexp"`
		MinUs     	int    `hcl:"min_us"`
		LogFormat 	string `hcl:"log_format"`
		StateScript string `hcl:"state_script"` // env ${data}, ${state} (StateBoot, StateBroken, BeginProcess, EndProcess )
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

type Inventory struct { //nolint:maligned
	Persist     bool    `hcl:"persist"`
	Stocks      []Stock `hcl:"stock"`
	TeleAddName bool    `hcl:"tele_add_name"` // send stock names to telemetry; false to save network usage
}

type Stock struct { //nolint:maligned
	Name        string  `hcl:"name,key"`
	Code        int     `hcl:"code"`
	Check       bool    `hcl:"check"`
	Min         float32 `hcl:"min"`
	HwRate      float32 `hcl:"hw_rate"`
	SpendRate   float32 `hcl:"spend_rate"`
	RegisterAdd string  `hcl:"register_add"`
}

func (self *Stock) String() string {
	return fmt.Sprintf("inventory.%s #%d check=%t hw_rate=%f spend_rate=%f min=%f",
		self.Name, self.Code, self.Check, self.HwRate, self.SpendRate, self.Min)
}
