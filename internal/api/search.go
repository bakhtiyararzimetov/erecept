package api

import (
	"errors"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"erecept/internal/i18n"

	"github.com/jackc/pgx/v5"
)

type MedicineDTO struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	ActiveSubstance string `json:"active_substance"`
	Form            string `json:"form"`
	Image           string `json:"image"` // картинка упаковки (иллюстрация), /static/meds/…
}

type PharmacyDTO struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	Address   string  `json:"address"`
	Phone     string  `json:"phone"`
	WorkHours string  `json:"work_hours"`
	Lat       float64 `json:"lat"`
	Lng       float64 `json:"lng"`
}

type OfferDTO struct {
	Pharmacy   PharmacyDTO `json:"pharmacy"`
	Price      int         `json:"price"`
	InStock    bool        `json:"in_stock"`
	DistanceKm *float64    `json:"distance_km"`
	UpdatedAt  string      `json:"updated_at"`
}

type AnalogDTO struct {
	Medicine        MedicineDTO `json:"medicine"`
	MinPrice        int         `json:"min_price"`
	PharmaciesCount int         `json:"pharmacies_count"`
}

type OffersDTO struct {
	Medicine   *MedicineDTO `json:"medicine"`
	Offers     []OfferDTO   `json:"offers"`
	Analogs    []AnalogDTO  `json:"analogs"`
	AnalogNote string       `json:"analog_note"`
}

const analogNote = "Согласуйте замену с врачом или фармацевтом"

// distanceKm — расстояние по формуле гаверсинуса, радиус Земли 6371 км.
func distanceKm(lat1, lng1, lat2, lng2 float64) float64 {
	const rad = math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLng := (lng2 - lng1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 6371 * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func (a *API) searchMedicines(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	out := []MedicineDTO{}
	if q == "" {
		writeJSON(w, http.StatusOK, out)
		return
	}
	l := i18n.FromContext(r.Context())
	rows, err := a.DB.Query(r.Context(), `SELECT `+medCols+` FROM medicines
		WHERE name ILIKE $1 OR name_en ILIKE $1 ORDER BY name LIMIT 20`, "%"+likeEscaper.Replace(q)+"%")
	if err != nil {
		serverError(w, "поиск лекарств", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var m medRow
		if err := rows.Scan(m.dest()...); err != nil {
			serverError(w, "поиск лекарств", err)
			return
		}
		out = append(out, m.finish(l))
	}
	if err := rows.Err(); err != nil {
		serverError(w, "поиск лекарств", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) offers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	qs := r.URL.Query()
	l := i18n.FromContext(ctx)
	res := OffersDTO{Offers: []OfferDTO{}, Analogs: []AnalogDTO{}, AnalogNote: analogNote}
	if l == i18n.EN {
		res.AnalogNote = analogNoteEN
	}

	// Лекарство: по id, иначе точное имя (русское или английское), иначе первое «имя%».
	const cols = `SELECT ` + medCols + ` FROM medicines `
	var row medRow
	var found bool
	var err error
	scan := func(sql string, args ...any) bool {
		e := a.DB.QueryRow(ctx, sql, args...).Scan(row.dest()...)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			err = e
		}
		return e == nil
	}
	if id, perr := strconv.ParseInt(qs.Get("medicine_id"), 10, 64); perr == nil {
		found = scan(cols+`WHERE id = $1`, id)
	}
	if name := strings.TrimSpace(qs.Get("name")); !found && err == nil && name != "" {
		found = scan(cols+`WHERE lower(name) = lower($1) OR lower(name_en) = lower($1) ORDER BY lower(name) = lower($1) DESC LIMIT 1`, name)
		if !found && err == nil {
			found = scan(cols+`WHERE name ILIKE $1 OR name_en ILIKE $1 ORDER BY name LIMIT 1`, likeEscaper.Replace(name)+"%")
		}
	}
	if err != nil {
		serverError(w, "поиск лекарства", err)
		return
	}
	if !found {
		writeJSON(w, http.StatusOK, res)
		return
	}
	m := row.finish(l)
	m.ActiveSubstance = row.ActiveSubstance // для поиска аналогов — русское МНН справочника
	res.Medicine = &m

	lat, errLat := strconv.ParseFloat(qs.Get("lat"), 64)
	lng, errLng := strconv.ParseFloat(qs.Get("lng"), 64)
	hasPos := errLat == nil && errLng == nil && !math.IsNaN(lat) && !math.IsNaN(lng)

	rows, err := a.DB.Query(ctx, `SELECT p.id, p.name, p.address, p.phone, p.work_hours, p.lat, p.lng, pi.price, pi.updated_at
		FROM pharmacy_items pi JOIN pharmacies p ON p.id = pi.pharmacy_id
		WHERE pi.medicine_id = $1 AND pi.in_stock`, m.ID)
	if err != nil {
		serverError(w, "предложения аптек", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var o OfferDTO
		var upd time.Time
		p := &o.Pharmacy
		if err := rows.Scan(&p.ID, &p.Name, &p.Address, &p.Phone, &p.WorkHours, &p.Lat, &p.Lng, &o.Price, &upd); err != nil {
			serverError(w, "предложения аптек", err)
			return
		}
		o.InStock = true
		if l == i18n.EN {
			pharmacyEN(p)
		}
		o.UpdatedAt = upd.In(time.Local).Format(time.RFC3339)
		if hasPos {
			d := math.Round(distanceKm(lat, lng, p.Lat, p.Lng)*10) / 10
			o.DistanceKm = &d
		}
		res.Offers = append(res.Offers, o)
	}
	if err := rows.Err(); err != nil {
		serverError(w, "предложения аптек", err)
		return
	}
	byDistance := hasPos && qs.Get("sort") == "distance"
	sort.SliceStable(res.Offers, func(i, j int) bool {
		x, y := res.Offers[i], res.Offers[j]
		if byDistance && *x.DistanceKm != *y.DistanceKm {
			return *x.DistanceKm < *y.DistanceKm
		}
		if x.Price != y.Price {
			return x.Price < y.Price
		}
		return x.Pharmacy.Name < y.Pharmacy.Name
	})

	arows, err := a.DB.Query(ctx, `SELECT m.id, m.name, m.active_substance, m.form, m.name_en, m.substance_en, MIN(pi.price), COUNT(DISTINCT pi.pharmacy_id)
		FROM medicines m JOIN pharmacy_items pi ON pi.medicine_id = m.id AND pi.in_stock
		WHERE m.id <> $1 AND $2 <> '' AND lower(m.active_substance) = lower($2)
		GROUP BY m.id ORDER BY MIN(pi.price), m.name`, m.ID, m.ActiveSubstance)
	if err != nil {
		serverError(w, "аналоги", err)
		return
	}
	defer arows.Close()
	for arows.Next() {
		var an AnalogDTO
		var mr medRow
		if err := arows.Scan(append(mr.dest(), &an.MinPrice, &an.PharmaciesCount)...); err != nil {
			serverError(w, "аналоги", err)
			return
		}
		an.Medicine = mr.finish(l)
		res.Analogs = append(res.Analogs, an)
	}
	if err := arows.Err(); err != nil {
		serverError(w, "аналоги", err)
		return
	}
	if l == i18n.EN {
		res.Medicine.ActiveSubstance = row.finish(l).ActiveSubstance
	}
	writeJSON(w, http.StatusOK, res)
}
