package currency

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strings"

	"github.com/temoto/errors"
)

// Amount is integer counting lowest currency unit, e.g. $1.20 = 120
type Amount uint32

func (self Amount) Format100I() string { return fmt.Sprint(float32(self) / 100) }
func (self Amount) FormatCtx(ctx context.Context) string {
	// XXX FIXME
	return self.Format100I()
}

// Nominal is value of one coin or bill
type Nominal Amount

var (
	ErrNominalInvalid = errors.New("Nominal is not valid for this group")
	ErrNominalCount   = errors.New("Not enough nominals for this amount")
)

// NominalGroup operates money comprised of multiple nominals, like coins or bills.
// coin1 : 3
// coin5 : 1
// coin10: 4
// total : 48
type NominalGroup struct {
	values map[Nominal]uint
}

func (self *NominalGroup) Copy() *NominalGroup {
	ng2 := &NominalGroup{
		values: make(map[Nominal]uint, len(self.values)),
	}
	for k, v := range self.values {
		ng2.values[k] = v
	}
	return ng2
}

func (self *NominalGroup) SetValid(valid []Nominal) {
	self.values = make(map[Nominal]uint, len(valid))
	for _, n := range valid {
		if n != 0 {
			self.values[n] = 0
		}
	}
}

func (self *NominalGroup) Add(n Nominal, count uint) error {
	if _, ok := self.values[n]; !ok {
		return errors.Annotatef(ErrNominalInvalid, "Add(n=%s, c=%d)", Amount(n).Format100I(), count)
	}
	self.values[n] += count
	return nil
}

func (self *NominalGroup) AddFrom(source *NominalGroup) {
	if self.values == nil {
		self.values = make(map[Nominal]uint, len(source.values))
	}
	for k, v := range source.values {
		self.values[k] += v
	}
}

func (self *NominalGroup) Clear() {
	for n := range self.values {
		self.values[n] = 0
	}
}

func (self *NominalGroup) Get(n Nominal) (uint, error) {
	if stored, ok := self.values[n]; !ok {
		return 0, ErrNominalInvalid
	} else {
		return stored, nil
	}
}

func (self *NominalGroup) Iter(f func(nominal Nominal, count uint) error) error {
	for nominal, count := range self.values {
		if err := f(nominal, count); err != nil {
			return err
		}
	}
	return nil
}

func (self *NominalGroup) Total() Amount {
	sum := Amount(0)
	for nominal, count := range self.values {
		sum += Amount(nominal) * Amount(count)
	}
	return sum
}

func (self *NominalGroup) Diff(other *NominalGroup) Amount {
	result := Amount(0)
	for n, c := range self.values {
		result += Amount(n)*Amount(c) - Amount(n)*Amount(other.values[n])
	}
	return result
}
func (self *NominalGroup) Sub(other *NominalGroup) {
	for nominal := range self.values {
		self.values[nominal] -= other.values[nominal]
	}
}

func (self *NominalGroup) Withdraw(to *NominalGroup, a Amount, strategy ExpendStrategy) error {
	return self.expendLoop(to, a, strategy)
}

func (self *NominalGroup) String() string {
	parts := make([]string, 0, len(self.values)+1)
	sum := Amount(0)
	for nominal, count := range self.values {
		if count > 0 {
			parts = append(parts, fmt.Sprintf("%s:%d", Amount(nominal).Format100I(), count))
			sum += Amount(nominal) * Amount(count)
		}
	}
	sort.Strings(parts)
	parts = append(parts, fmt.Sprintf("total:%s", sum.Format100I()))
	return strings.Join(parts, ",")
}

func (self *NominalGroup) expendLoop(to *NominalGroup, amount Amount, strategy ExpendStrategy) error {
	strategy.Reset(self)
	for amount > 0 {
		nominal, err := strategy.ExpendOne(self, amount)
		if err != nil {
			return err
		}
		if nominal == 0 {
			panic("ExpendStrategy returned Nominal 0 without error")
		}
		amount -= Amount(nominal)
		if to != nil {
			to.values[nominal] += 1
		}
	}
	return nil
}

// common code from strategies
func expendOneOrdered(from *NominalGroup, order []Nominal, max Amount) (Nominal, error) {
	if len(order) < len(from.values) {
		panic("expendOneOrdered order must include all nominals")
	}
	if max == 0 {
		return 0, nil
	}
	for _, n := range order {
		if Amount(n) <= max && from.values[n] > 0 {
			from.values[n] -= 1
			return n, nil
		}
	}
	return 0, ErrNominalCount
}

type ngOrderSortElemFunc func(Nominal, uint) Nominal

func (self *NominalGroup) order(sortElemFunc ngOrderSortElemFunc) []Nominal {
	order := make([]Nominal, 0, len(self.values))
	for n := range self.values {
		order = append(order, n)
	}
	sort.Slice(order, func(i, j int) bool {
		ni, nj := order[i], order[j]
		return sortElemFunc(ni, self.values[ni]) > sortElemFunc(nj, self.values[nj])
	})
	return order
}
func ngOrderSortElemNominal(n Nominal, c uint) Nominal { return n }
func ngOrderSortElemCount(n Nominal, c uint) Nominal   { return Nominal(c) }

// NominalGroup.Withdraw = strategy.Reset + loop strategy.ExpendOne
type ExpendStrategy interface {
	Reset(from *NominalGroup)
	ExpendOne(from *NominalGroup, max Amount) (Nominal, error)
	Validate() bool
}

type ExpendGenericOrder struct {
	order        []Nominal
	SortElemFunc ngOrderSortElemFunc
}

func (self *ExpendGenericOrder) Reset(from *NominalGroup) {
	self.order = from.order(self.SortElemFunc)
}
func (self *ExpendGenericOrder) ExpendOne(from *NominalGroup, max Amount) (Nominal, error) {
	return expendOneOrdered(from, self.order, max)
}
func (self *ExpendGenericOrder) Validate() bool { return true }

func NewExpendLeastCount() ExpendStrategy {
	return &ExpendGenericOrder{SortElemFunc: ngOrderSortElemNominal}
}
func NewExpendMostAvailable() ExpendStrategy {
	return &ExpendGenericOrder{SortElemFunc: ngOrderSortElemCount}
}

type ExpendStatistical struct {
	order []Nominal
	Stat  *NominalGroup
}

func (self *ExpendStatistical) Reset(from *NominalGroup) {
	self.order = self.Stat.order(ngOrderSortElemCount)
}
func (self *ExpendStatistical) ExpendOne(from *NominalGroup, max Amount) (Nominal, error) {
	return expendOneOrdered(from, self.order, max)
}
func (self *ExpendStatistical) Validate() bool {
	return self.Stat.Total() > 0
}

type ExpendCombine struct {
	rnd   *rand.Rand
	S1    ExpendStrategy
	S2    ExpendStrategy
	Ratio float32
}

func (self *ExpendCombine) Reset(from *NominalGroup) {
	self.rnd = rand.New(rand.NewSource(int64(from.Total())))
	self.S1.Reset(from)
	self.S2.Reset(from)
}
func (self *ExpendCombine) ExpendOne(from *NominalGroup, max Amount) (Nominal, error) {
	if self.rnd.Float32() < self.Ratio {
		return self.S1.ExpendOne(from, max)
	}
	return self.S2.ExpendOne(from, max)
}
func (self *ExpendCombine) Validate() bool {
	return self.S1 != nil && self.S2 != nil && self.S1.Validate() && self.S2.Validate()
}
