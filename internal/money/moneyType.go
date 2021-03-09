package moneyType

type MoneySystem struct { //nolint:maligned
	Log   *log2.Log
	lk    sync.RWMutex
	dirty currency.Amount // uncommited

	bill        bill.Biller
	billCashbox currency.NominalGroup
	billCredit  currency.NominalGroup
Bb   bool
	coin        coin.Coiner
	coinCashbox currency.NominalGroup
	coinCredit  currency.NominalGroup

	giftCredit currency.Amount
}
