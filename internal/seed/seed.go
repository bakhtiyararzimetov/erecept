// Package seed: тестовые данные для демо (врачи, аптеки, лекарства, цены).
package seed

import (
	"context"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type medicine struct {
	name, substance, form string
	price                 int    // базовая цена, тенге
	nameEN, substanceEN   string // английская версия интерфейса
}

// 30 лекарств; у восьми веществ есть пара «оригинал/дженерик».
var medicines = []medicine{
	{"Парацетамол", "парацетамол", "таблетки 500 мг №10", 300, "Paracetamol", "paracetamol"},
	{"Панадол", "парацетамол", "таблетки 500 мг №12", 1300, "Panadol", "paracetamol"},
	{"Ибупрофен", "ибупрофен", "таблетки 400 мг №20", 550, "Ibuprofen", "ibuprofen"},
	{"Нурофен", "ибупрофен", "таблетки 200 мг №24", 1900, "Nurofen", "ibuprofen"},
	{"Амоксициллин", "амоксициллин", "капсулы 500 мг №16", 1100, "Amoxicillin", "amoxicillin"},
	{"Флемоксин Солютаб", "амоксициллин", "таблетки 500 мг №20", 3200, "Flemoxin Solutab", "amoxicillin"},
	{"Азитромицин", "азитромицин", "таблетки 500 мг №3", 1500, "Azithromycin", "azithromycin"},
	{"Сумамед", "азитромицин", "таблетки 500 мг №3", 4300, "Sumamed", "azithromycin"},
	{"Омепразол", "омепразол", "капсулы 20 мг №30", 900, "Omeprazole", "omeprazole"},
	{"Омез", "омепразол", "капсулы 20 мг №30", 2300, "Omez", "omeprazole"},
	{"Метформин", "метформин", "таблетки 850 мг №60", 1200, "Metformin", "metformin"},
	{"Глюкофаж", "метформин", "таблетки 850 мг №60", 2800, "Glucophage", "metformin"},
	{"Лизиноприл", "лизиноприл", "таблетки 10 мг №30", 800, "Lisinopril", "lisinopril"},
	{"Амлодипин", "амлодипин", "таблетки 5 мг №30", 700, "Amlodipine", "amlodipine"},
	{"Аторвастатин", "аторвастатин", "таблетки 20 мг №30", 1700, "Atorvastatin", "atorvastatin"},
	{"Цетиризин", "цетиризин", "таблетки 10 мг №20", 600, "Cetirizine", "cetirizine"},
	{"Зодак", "цетиризин", "таблетки 10 мг №30", 2100, "Zodac", "cetirizine"},
	{"Лоратадин", "лоратадин", "таблетки 10 мг №10", 450, "Loratadine", "loratadine"},
	{"Амброксол", "амброксол", "сироп 30 мг/5 мл 100 мл", 900, "Ambroxol", "ambroxol"},
	{"Ацетилцистеин", "ацетилцистеин", "шипучие таблетки 600 мг №10", 1800, "Acetylcysteine", "acetylcysteine"},
	{"Метронидазол", "метронидазол", "таблетки 250 мг №20", 500, "Metronidazole", "metronidazole"},
	{"Ципрофлоксацин", "ципрофлоксацин", "таблетки 500 мг №10", 850, "Ciprofloxacin", "ciprofloxacin"},
	{"Дротаверин", "дротаверин", "таблетки 40 мг №20", 400, "Drotaverine", "drotaverine"},
	{"Но-шпа", "дротаверин", "таблетки 40 мг №24", 1600, "No-Spa", "drotaverine"},
	{"Панкреатин", "панкреатин", "таблетки 25 ЕД №50", 700, "Pancreatin", "pancreatin"},
	{"Лоперамид", "лоперамид", "капсулы 2 мг №10", 500, "Loperamide", "loperamide"},
	{"Нимесулид", "нимесулид", "таблетки 100 мг №20", 950, "Nimesulide", "nimesulide"},
	{"Диклофенак", "диклофенак", "таблетки 50 мг №20", 650, "Diclofenac", "diclofenac"},
	{"Валсартан", "валсартан", "таблетки 80 мг №28", 2400, "Valsartan", "valsartan"},
	{"Бисопролол", "бисопролол", "таблетки 5 мг №30", 1300, "Bisoprolol", "bisoprolol"},
}

type pharmacy struct {
	name, address, phone, hours string
	lat, lng                    float64
	coef                        float64 // множитель цены
}

var pharmacies = []pharmacy{
	{"Аптека «Здоровье плюс»", "пр. Тауке хана, 52", "+7 7252 50-11-01", "08:00–22:00", 42.3155, 69.5869, 1.00},
	{"Аптека «Айболит»", "ул. Байтурсынова, 91", "+7 7252 50-11-02", "09:00–21:00", 42.3297, 69.601, 1.10},
	{"Аптека «Тау Фарм»", "пр. Республики, 42", "+7 7252 50-11-03", "08:00–23:00", 42.3172, 69.5955, 0.90},
	{"Аптека «Нур Мед»", "ул. Казыбек би, 77", "+7 7252 50-11-04", "Круглосуточно", 42.323, 69.579, 1.20},
	{"Аптека «Береке»", "пр. Кунаева, 58", "+7 7252 50-11-05", "09:00–20:00", 42.3402, 69.6105, 0.85},
	{"Аптека «Дәрі-Дәрмек»", "ул. Толе би, 30", "+7 7252 50-11-06", "08:00–21:00", 42.3125, 69.6043, 0.95},
	{"Аптека «Шымкент Фарм»", "мкр. Нурсат, 212", "+7 7252 50-11-07", "08:30–22:00", 42.365, 69.628, 1.05},
	{"Аптека «Жеті Қазына»", "мкр. Самал-2, 15", "+7 7252 50-11-08", "09:00–22:00", 42.301, 69.564, 1.15},
}

// Run заполняет базу демо-данными. Можно запускать повторно: готовое не меняется.
func Run(ctx context.Context, pool *pgxpool.Pool) error {
	doctorHash, err := hash("doctor123")
	if err != nil {
		return err
	}
	pharmHash, err := hash("apteka123")
	if err != nil {
		return err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // после Commit ничего не делает

	doctors := []struct{ login, name, clinic string }{
		{"doctor1", "Ахметова Динара Сериковна", "Поликлиника №5"},
		{"doctor2", "Ким Александр Петрович", "Городская поликлиника №12"},
	}
	for _, d := range doctors {
		_, err := tx.Exec(ctx, `INSERT INTO doctors (login, password_hash, full_name, clinic)
			VALUES ($1, $2, $3, $4) ON CONFLICT (login) DO NOTHING`, d.login, doctorHash, d.name, d.clinic)
		if err != nil {
			return fmt.Errorf("сид врачей: %w", err)
		}
	}

	for _, m := range medicines {
		_, err := tx.Exec(ctx, `INSERT INTO medicines (name, active_substance, form, name_en, substance_en)
			VALUES ($1, $2, $3, $4, $5) ON CONFLICT (name) DO NOTHING`, m.name, m.substance, m.form, m.nameEN, m.substanceEN)
		if err != nil {
			return fmt.Errorf("сид лекарств: %w", err)
		}
	}
	ids := map[string]int64{}
	for _, m := range medicines {
		var id int64
		if err := tx.QueryRow(ctx, `SELECT id FROM medicines WHERE name = $1`, m.name).Scan(&id); err != nil {
			return err
		}
		ids[m.name] = id
	}

	for i, p := range pharmacies {
		login := fmt.Sprintf("apteka%d", i+1)
		var pharmacyID int64
		err := tx.QueryRow(ctx, `INSERT INTO pharmacies (login, password_hash, name, address, phone, work_hours, lat, lng)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (login) DO UPDATE SET login = EXCLUDED.login RETURNING id`,
			login, pharmHash, p.name, "Шымкент, "+p.address, p.phone, p.hours, p.lat, p.lng).Scan(&pharmacyID)
		if err != nil {
			return fmt.Errorf("сид аптек: %w", err)
		}

		// В аптеке 18, 21 или 24 лекарства, набор сдвигается от аптеки к аптеке.
		n := 18 + (i*3)%9
		for j := 0; j < n; j++ {
			m := medicines[(i*3+j)%len(medicines)]
			spread := float64((i*7+j*13)%11-5) / 100 // от -5% до +5%
			price := int(math.Round(float64(m.price)*p.coef*(1+spread)/10)) * 10
			inStock := !(j == 3 || (i%2 == 0 && j == 11))
			_, err := tx.Exec(ctx, `INSERT INTO pharmacy_items (pharmacy_id, medicine_id, price, in_stock)
				VALUES ($1, $2, $3, $4) ON CONFLICT (pharmacy_id, medicine_id) DO NOTHING`,
				pharmacyID, ids[m.name], price, inStock)
			if err != nil {
				return fmt.Errorf("сид цен: %w", err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("сид: фиксация: %w", err)
	}
	return nil
}

func hash(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	return string(h), err
}
