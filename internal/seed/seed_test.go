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

// В каждой демо-аптеке — ходовые лекарства, 20–26 позиций, цены разные, немного «нет в наличии».
func TestSeedAssortment(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	if err := Run(ctx, pool); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, `SELECT p.login, count(*), count(*) FILTER (WHERE NOT i.in_stock)
		FROM pharmacies p JOIN pharmacy_items i ON i.pharmacy_id = p.id GROUP BY p.login`)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for rows.Next() {
		var login string
		var total, out int
		if err := rows.Scan(&login, &total, &out); err != nil {
			t.Fatal(err)
		}
		n++
		if total < 20 || total > 26 {
			t.Errorf("%s: позиций %d, нужно 20–26", login, total)
		}
		if out < 1 || out > 4 {
			t.Errorf("%s: нет в наличии %d, нужно 1–4", login, out)
		}
	}
	rows.Close()
	if n != 8 {
		t.Fatalf("аптек с позициями %d, нужно 8", n)
	}
	for _, name := range []string{"Парацетамол", "Ибупрофен", "Амлодипин", "Метформин", "Омепразол", "Амоксициллин", "Цетиризин", "Аторвастатин"} {
		var have, prices int
		err := pool.QueryRow(ctx, `SELECT count(*), count(DISTINCT i.price) FROM pharmacy_items i
			JOIN medicines m ON m.id = i.medicine_id WHERE m.name = $1`, name).Scan(&have, &prices)
		if err != nil {
			t.Fatal(err)
		}
		if have != 8 {
			t.Errorf("%s есть в %d аптеках из 8", name, have)
		}
		if prices < 4 {
			t.Errorf("%s: всего %d разных цен", name, prices)
		}
	}
}
