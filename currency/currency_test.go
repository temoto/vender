package currency

import (
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestNominalGroup(t testing.TB) *NominalGroup {
	ng := &NominalGroup{}
	ng.SetValid([]Nominal{10, 5, 2, 1})
	require.Error(t, ng.Add(101, 1), "expected invalid nominal")
	require.NoError(t, ng.Add(10, 2))
	require.NoError(t, ng.Add(5, 8))
	require.NoError(t, ng.Add(2, 1))
	require.NoError(t, ng.Add(1, 3))
	return ng
}

func testCheckExpend(t *testing.T, strategy ExpendStrategy) {
	ng := newTestNominalGroup(t)

	if es, ok := strategy.(*ExpendStatistical); ok {
		es.Stat = ng.Copy()
	}

	require.True(t, strategy.Validate())
	total1 := ng.Total()
	require.NoError(t, ng.Copy().Withdraw(nil, 17, strategy))
	total2 := ng.Total()
	require.NoError(t, ng.Withdraw(nil, 17, strategy))
	total3 := ng.Total()
	require.Error(t, ng.Copy().Withdraw(nil, 100, strategy), "expected withdraw error")
	total4 := ng.Total()
	require.Error(t, ng.Withdraw(nil, 100, strategy), "expected withdraw error")
	total5 := ng.Total()
	const exptotal1 = Amount(65)
	const exptotal2 = Amount(48)
	const exptotal3 = Amount(0)
	require.Equal(t, exptotal1, total1)
	require.Equal(t, exptotal1, total2)
	require.Equal(t, exptotal2, total3)
	require.Equal(t, exptotal2, total4)
	require.Equal(t, exptotal3, total5)
}

func TestNominalGroup(t *testing.T) {
	t.Parallel()
	t.Run("ExpendLeastCount", func(t *testing.T) { testCheckExpend(t, NewExpendLeastCount()) })
	t.Run("ExpendMostAvailable", func(t *testing.T) { testCheckExpend(t, NewExpendMostAvailable()) })
	t.Run("ExpendStatistical", func(t *testing.T) { testCheckExpend(t, &ExpendStatistical{}) })
	t.Run("ExpendCombine", func(t *testing.T) {
		testCheckExpend(t, &ExpendCombine{S1: NewExpendLeastCount(), S2: NewExpendMostAvailable(), Ratio: 0.5})
	})
	t.Run("Get", func(t *testing.T) {
		ng := newTestNominalGroup(t)
		for n, expectCount := range ng.values {
			count, err := ng.Get(n)
			assert.NoError(t, err)
			assert.Equal(t, expectCount, count)
		}
		checkErr := func(n Nominal) {
			count, err := ng.Get(n)
			assert.Equal(t, ErrNominalInvalid, err)
			assert.Equal(t, uint(0), count)
		}
		checkErr(0)
		checkErr(3)
		checkErr(11)
	})
	t.Run("Iter", func(t *testing.T) {
		ng := newTestNominalGroup(t)
		ss := make([]string, 0, 8)
		fun := func(n Nominal, count uint) error {
			ss = append(ss, fmt.Sprintf("%d:%d", n, count))
			return nil
		}
		require.NoError(t, ng.Iter(fun))
		sort.Strings(ss)
		expect := []string{"10:2", "1:3", "2:1", "5:8"}
		assert.Equal(t, expect, ss)
	})
	t.Run("Iter/error", func(t *testing.T) {
		ng := newTestNominalGroup(t)
		expectErr := fmt.Errorf("expected-error")
		visited := 0
		fun := func(n Nominal, count uint) error {
			visited++
			if visited == 2 {
				return expectErr
			}
			return nil
		}
		assert.Equal(t, expectErr, ng.Iter(fun))
		assert.Equal(t, 2, visited)
	})
	t.Run("String", func(t *testing.T) {
		assert.Equal(t, "0.01:3,0.02:1,0.05:8,0.1:2,total:0.65", newTestNominalGroup(t).String())
	})
}
