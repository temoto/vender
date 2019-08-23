package input

const MoneySourceTag = "money"

const (
	MoneyKeyAbort Key = 27
)

func IsMoneyAbort(e *Event) bool { return e.Source == MoneySourceTag && e.Key == MoneyKeyAbort }
