package db_test

import (
	"context"
	"testing"

	"erecept/internal/db"
	"erecept/internal/db/dbtest"
	"erecept/internal/seed"
)

// Миграция 010 дополняет уже заполненную базу (демо на Render): в каждой
// демо-аптеке появляются ходовые лекарства, свои цены аптеки не меняются.
func TestMigrationFillsDemoPharmacies(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.New(t)
	if err := seed.Run(ctx, pool); err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	// Как было на живой базе: у аптек мало позиций, у apteka1 своя цена на парацетамол.
	exec(`DELETE FROM pharmacy_items WHERE medicine_id NOT IN (SELECT id FROM medicines WHERE name IN ('Парацетамол', 'Сумамед'))`)
	exec(`UPDATE pharmacy_items SET price = 777, in_stock = false WHERE pharmacy_id = (SELECT id FROM pharmacies WHERE login = 'apteka1')
		AND medicine_id = (SELECT id FROM medicines WHERE name = 'Парацетамол')`)

	var name string
	if err := pool.QueryRow(ctx, `SELECT name FROM schema_migrations WHERE name LIKE '010_%'`).Scan(&name); err != nil {
		t.Fatalf("нет миграции 010: %v", err)
	}
	for range 2 { // повторный прогон ничего не дублирует и не падает
		exec(`DELETE FROM schema_migrations WHERE name = $1`, name)
		if err := db.Migrate(ctx, pool); err != nil {
			t.Fatal(err)
		}
	}

	var bad int
	err := pool.QueryRow(ctx, `SELECT count(*) FROM pharmacies p WHERE p.login LIKE 'apteka%' AND
		(SELECT count(*) FROM pharmacy_items i WHERE i.pharmacy_id = p.id) < 20`).Scan(&bad)
	if err != nil || bad != 0 {
		t.Fatalf("аптек с менее чем 20 позициями: %d (%v)", bad, err)
	}
	for _, med := range []string{"Ибупрофен", "Амлодипин", "Метформин", "Омепразол", "Цетиризин"} {
		var n, inStock int
		err := pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE i.in_stock) FROM pharmacy_items i
			JOIN medicines m ON m.id = i.medicine_id WHERE m.name = $1`, med).Scan(&n, &inStock)
		if err != nil || n != 8 || inStock < 6 {
			t.Errorf("%s: в %d аптеках, в наличии в %d (%v)", med, n, inStock, err)
		}
	}
	var price int
	var stock bool
	err = pool.QueryRow(ctx, `SELECT i.price, i.in_stock FROM pharmacy_items i JOIN pharmacies p ON p.id = i.pharmacy_id
		JOIN medicines m ON m.id = i.medicine_id WHERE p.login = 'apteka1' AND m.name = 'Парацетамол'`).Scan(&price, &stock)
	if err != nil || price != 777 || stock {
		t.Errorf("своя цена аптеки изменилась: %d %v (%v)", price, stock, err)
	}
}

// Миграция и сид дают одинаковые цены и наличие ходовых лекарств.
func TestMigrationMatchesSeed(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.New(t)
	if err := seed.Run(ctx, pool); err != nil {
		t.Fatal(err)
	}
	snapshot := func() map[string]string {
		rows, err := pool.Query(ctx, `SELECT p.login || '/' || m.name, i.price::text || '/' || i.in_stock::text
			FROM pharmacy_items i JOIN pharmacies p ON p.id = i.pharmacy_id JOIN medicines m ON m.id = i.medicine_id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		got := map[string]string{}
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err != nil {
				t.Fatal(err)
			}
			got[k] = v
		}
		return got
	}
	fromSeed := snapshot()
	if _, err := pool.Exec(ctx, `DELETE FROM pharmacy_items; DELETE FROM schema_migrations WHERE name LIKE '010_%'`); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	fromMigration := snapshot()
	if len(fromMigration) != 160 {
		t.Errorf("миграция добавила %d позиций, нужно 8 × 20", len(fromMigration))
	}
	for k, v := range fromMigration {
		if fromSeed[k] != v {
			t.Errorf("%s: сид %q, миграция %q", k, fromSeed[k], v)
		}
	}
}
