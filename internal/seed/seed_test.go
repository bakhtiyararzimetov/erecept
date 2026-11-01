package seed

import (
	"context"
	"testing"

	"erecept/internal/db/dbtest"
)

func TestSeedCounts(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()

	count := func(q string) int {
		var n int
		if err := pool.QueryRow(ctx, q).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	check := func() {
		if n := count(`SELECT count(*) FROM doctors`); n != 2 {
			t.Errorf("врачей %d, нужно 2", n)
		}
		if n := count(`SELECT count(*) FROM pharmacies`); n != 8 {
			t.Errorf("аптек %d, нужно 8", n)
		}
		if n := count(`SELECT count(*) FROM medicines`); n != 30 {
			t.Errorf("лекарств %d, нужно 30", n)
		}
	}

	if err := Run(ctx, pool); err != nil {
		t.Fatal(err)
	}
	check()
	items := count(`SELECT count(*) FROM pharmacy_items`)

	if err := Run(ctx, pool); err != nil {
		t.Fatal(err)
	}
	check()
	if n := count(`SELECT count(*) FROM pharmacy_items`); n != items {
		t.Errorf("повторный Run изменил число позиций: %d -> %d", items, n)
	}

	if n := count(`SELECT count(*) FROM pharmacies p WHERE
		(SELECT count(*) FROM pharmacy_items i WHERE i.pharmacy_id = p.id) < 18`); n != 0 {
		t.Errorf("аптек с менее чем 18 позициями: %d", n)
	}
	if n := count(`SELECT count(*) FROM (SELECT active_substance FROM medicines
		GROUP BY active_substance HAVING count(*) >= 2) s`); n < 5 {
		t.Errorf("веществ с аналогами %d, нужно минимум 5", n)
	}
	if n := count(`SELECT count(*) FROM pharmacy_items WHERE NOT in_stock`); n < 8 {
		t.Errorf("позиций «нет в наличии» %d, нужно хотя бы по одной на аптеку", n)
	}
}
