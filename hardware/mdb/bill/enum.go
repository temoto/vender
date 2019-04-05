package bill

import "context"

func Enum(ctx context.Context, fun func(d interface{})) {
	d := new(BillValidator)
	if err := d.Init(ctx); err == nil {
		if fun != nil {
			fun(d)
		}
	}
}
