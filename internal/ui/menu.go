package ui

import (
	"context"
	"fmt"

	"github.com/temoto/vender/currency"
	"github.com/temoto/vender/internal/engine"
	"github.com/temoto/vender/internal/state"
	"github.com/temoto/vender/internal/types"
	tele_api "github.com/temoto/vender/tele"
)

type Menu map[string]MenuItem

type MenuItem struct {
	Name  string
	D     engine.Doer
	Price currency.Amount
	Code  string
}

func (self *MenuItem) String() string {
	return fmt.Sprintf("menu code=%s price=%d(raw) name='%s'", self.Code, self.Price, self.Name)
}

func FillMenu(ctx context.Context) {
	config := state.GetGlobal(ctx).Config

	for _, x := range config.Engine.Menu.Items {
		// self.Add(x.Code, x.Name, x.Price, x.Doer)
		types.UI.Menu[x.Code] = types.MenuItemType{
			Name:  x.Name,
			D:     x.Doer,
			Price: x.Price,
			Code:  x.Code,
		}
	}
}
func (self Menu) Init(ctx context.Context) error {
	config := state.GetGlobal(ctx).Config

	for _, x := range config.Engine.Menu.Items {
		// self.Add(x.Code, x.Name, x.Price, x.Doer)
		types.UI.Menu[x.Code] = types.MenuItemType{
			Name:  x.Name,
			D:     x.Doer,
			Price: x.Price,
			Code:  x.Code,
		}
	}
	return nil
}

// func (self Menu) Add(code string, name string, price currency.Amount, d engine.Doer) {
// 	self[code] = MenuItem{
// 		Code:  code,
// 		Name:  name,
// 		Price: price,
// 		D:     d,
// 	}
// }

func (self *UI) Cook(ctx context.Context, code string, Cream uint8, Sugar uint8, payMethod tele_api.PaymentMethod) error {
	// g := state.GetGlobal(ctx)
	// cmenu := ui.menu[code]

	// // moneysys := money.GetGlobal(ctx)
	// teletx := &tele_api.Telemetry_Transaction{
	// 	Code:          cmenu.Code,
	// 	Options:       []int32{int32(Cream), int32(Sugar)},
	// 	Price:         uint32(cmenu.Price),
	// 	PaymentMethod: payMethod,
	// }
	// if err := cmenu.D.Validate(); err != nil {
	// 	g.Log.Errorf("ui-front selected=%s Validate err=%v", cmenu.String(), err)
	// 	return err
	// }

	// itemCtx := context.WithValue(ctx, "run/current-price", cmenu.Price)

	// if tuneCream := ScaleTuneRate(Cream, MaxCream, DefaultCream); tuneCream != 1 {
	// 	const name = "cream"
	// 	var err error
	// 	g.Log.Debugf("ui-front tuning stock=%s tune=%v", name, tuneCream)
	// 	if itemCtx, err = g.Inventory.WithTuning(itemCtx, name, tuneCream); err != nil {
	// 		g.Log.Errorf("ui-front tuning stock=%s err=%v", name, err)
	// 	}
	// }
	// if tuneSugar := ScaleTuneRate(Sugar, MaxSugar, DefaultSugar); tuneSugar != 1 {
	// 	const name = "sugar"
	// 	var err error
	// 	g.Log.Debugf("ui-front tuning stock=%s tune=%v", name, tuneSugar)
	// 	if itemCtx, err = g.Inventory.WithTuning(itemCtx, name, tuneSugar); err != nil {
	// 		g.Log.Errorf("ui-front tuning stock=%s err=%v", name, err)
	// 	}
	// }
	// g.Hardware.HD44780.Display.SetLines(g.Config.UI.Front.MsgMaking1, g.Config.UI.Front.MsgMaking2)

	// err := g.Engine.Exec(itemCtx, cmenu.D)
	// if err != nil {
	// 	return err
	// }
	// g.Tele.Transaction(teletx)
	return nil
}
