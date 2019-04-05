package currency

import "testing"

func createTestNominalGroup(t *testing.T) *NominalGroup {
	ng := &NominalGroup{}
	ng.SetValid([]Nominal{10, 5, 2, 1})
	if err := ng.Add(101, 1); err == nil {
		t.Fatal("expected invalid nominal")
	}
	if err := ng.Add(10, 2); err != nil {
		t.Fatal(err)
	}
	if err := ng.Add(5, 8); err != nil {
		t.Fatal(err)
	}
	if err := ng.Add(2, 1); err != nil {
		t.Fatal(err)
	}
	if err := ng.Add(1, 3); err != nil {
		t.Fatal(err)
	}
	return ng
}

func testCheckNominalGroup(t *testing.T, strategy ExpendStrategy) {
	ng := createTestNominalGroup(t)

	if es, ok := strategy.(*ExpendStatistical); ok {
		es.Stat = ng.Copy()
	}

	total1 := ng.Total()
	if err := ng.Copy().Withdraw(nil, 17, strategy); err != nil {
		t.Fatal(err)
	}
	total2 := ng.Total()
	if err := ng.Withdraw(nil, 17, strategy); err != nil {
		t.Fatal(err)
	}
	total3 := ng.Total()
	if err := ng.Copy().Withdraw(nil, 100, strategy); err == nil {
		t.Fatal("expected withdraw error")
	}
	total4 := ng.Total()
	if err := ng.Withdraw(nil, 100, strategy); err == nil {
		t.Fatal("expected withdraw error")
	}
	total5 := ng.Total()
	const exptotal1 = 65
	const exptotal2 = 48
	const exptotal3 = 0
	if total1 != exptotal1 || total2 != exptotal1 {
		t.Fatalf("expected total1 %d == total2 %d == %d", total1, total2, exptotal1)
	}
	if total3 != exptotal2 || total4 != exptotal2 {
		t.Fatalf("expected total3 %d == total4 %d == %d", total3, total4, exptotal2)
	}
	if total5 != exptotal3 {
		t.Fatalf("expected total5 %d == %d", total5, exptotal3)
	}
}

func testCheckContains(t *testing.T, a Amount, expected bool) {
	ng := createTestNominalGroup(t)
	if ng.Contains(a) != expected {
		t.Fatalf("")
	}
}

func TestNominalGroup(t *testing.T) {
	t.Parallel()
	t.Run("ExpendLeastCount", func(t *testing.T) { testCheckNominalGroup(t, NewExpendLeastCount()) })
	t.Run("ExpendMostAvailable", func(t *testing.T) { testCheckNominalGroup(t, NewExpendMostAvailable()) })
	t.Run("ExpendStatistical", func(t *testing.T) { testCheckNominalGroup(t, &ExpendStatistical{}) })
	t.Run("ExpendCombine", func(t *testing.T) {
		testCheckNominalGroup(t, &ExpendCombine{S1: NewExpendLeastCount(), S2: NewExpendMostAvailable(), Ratio: 0.5})
	})
	t.Run("Contains/0", func(t *testing.T) { testCheckContains(t, 0, true) })
	t.Run("Contains/17", func(t *testing.T) { testCheckContains(t, 17, true) })
	t.Run("Contains/39", func(t *testing.T) { testCheckContains(t, 39, true) })
	t.Run("Contains/200", func(t *testing.T) { testCheckContains(t, 200, false) })
}
