package api

import (
	"context"
	"encoding/json"
	"math"
	"net/url"
	"strconv"
	"testing"

	"erecept/internal/db/dbtest"
	"erecept/internal/seed"
)

func TestDistanceKm(t *testing.T) {
	d := distanceKm(43.2389, 76.8897, 43.2567, 76.9286)
	if math.Abs(d-3.6) > 0.2 {
		t.Fatalf("distanceKm = %v, ожидали около 3.6", d)
	}
}

func TestSearchAPI(t *testing.T) {
	pool := dbtest.New(t)
	if err := seed.Run(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, pool: pool, h: (&API{DB: pool}).Routes()}

	offers := func(t *testing.T, q url.Values) OffersDTO {
		t.Helper()
		w := e.do("GET", "/offers?"+q.Encode(), "", nil) // без X-Device-Secret
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		var o OffersDTO
		if err := json.Unmarshal(w.Body.Bytes(), &o); err != nil {
			t.Fatal(err)
		}
		return o
	}

	t.Run("OffersSortPrice", func(t *testing.T) {
		o := offers(t, url.Values{"name": {"Парацетамол"}, "sort": {"price"}})
		if o.Medicine == nil || len(o.Offers) == 0 {
			t.Fatalf("нет предложений: %+v", o)
		}
		for i, of := range o.Offers {
			if !of.InStock || of.Pharmacy.Address == "" || of.Pharmacy.WorkHours == "" {
				t.Fatalf("плохое предложение: %+v", of)
			}
			if of.DistanceKm != nil {
				t.Fatal("distance_km без lat/lng должен быть null")
			}
			if i > 0 && of.Price < o.Offers[i-1].Price {
				t.Fatal("цены убывают")
			}
		}
	})

	t.Run("OffersSortDistance", func(t *testing.T) {
		o := offers(t, url.Values{"name": {"Парацетамол"}, "sort": {"distance"}, "lat": {"43.2389"}, "lng": {"76.8897"}})
		if len(o.Offers) == 0 {
			t.Fatal("пусто")
		}
		for i, of := range o.Offers {
			if of.DistanceKm == nil {
				t.Fatal("distance_km null")
			}
			if i > 0 && *of.DistanceKm < *o.Offers[i-1].DistanceKm {
				t.Fatal("расстояния убывают")
			}
		}
	})

	t.Run("DistanceWithoutCoordsFallsBackToPrice", func(t *testing.T) {
		o := offers(t, url.Values{"name": {"Парацетамол"}, "sort": {"distance"}, "lat": {"abc"}})
		for i, of := range o.Offers {
			if of.DistanceKm != nil || (i > 0 && of.Price < o.Offers[i-1].Price) {
				t.Fatal("ожидали сортировку по цене без расстояния")
			}
		}
	})

	t.Run("OffersAnalogs", func(t *testing.T) {
		o := offers(t, url.Values{"name": {"Нурофен"}})
		if o.Medicine == nil || len(o.Analogs) == 0 {
			t.Fatalf("нет аналогов: %+v", o)
		}
		for i, a := range o.Analogs {
			if a.Medicine.ID == o.Medicine.ID {
				t.Fatal("сам препарат в аналогах")
			}
			if a.PharmaciesCount < 1 || (i > 0 && a.MinPrice < o.Analogs[i-1].MinPrice) {
				t.Fatalf("плохой аналог: %+v", a)
			}
		}
		if o.AnalogNote != "Согласуйте замену с врачом или фармацевтом" {
			t.Fatal(o.AnalogNote)
		}
	})

	t.Run("OffersByID", func(t *testing.T) {
		o := offers(t, url.Values{"name": {"Парацетамол"}})
		o2 := offers(t, url.Values{"medicine_id": {strconv.FormatInt(o.Medicine.ID, 10)}})
		if o2.Medicine == nil || o2.Medicine.ID != o.Medicine.ID {
			t.Fatal("поиск по id")
		}
	})

	t.Run("OffersUnknown", func(t *testing.T) {
		o := offers(t, url.Values{"name": {"Несуществующее"}})
		if o.Medicine != nil || len(o.Offers) != 0 || len(o.Analogs) != 0 {
			t.Fatalf("%+v", o)
		}
	})

	t.Run("MedicineSearch", func(t *testing.T) {
		w := e.do("GET", "/medicines/search?q="+url.QueryEscape("пара"), "", nil)
		var ms []MedicineDTO
		json.Unmarshal(w.Body.Bytes(), &ms)
		found := false
		for _, m := range ms {
			if m.Name == "Парацетамол" {
				found = true
			}
		}
		if w.Code != 200 || !found {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		for _, q := range []string{"", "%", "_"} {
			w = e.do("GET", "/medicines/search?q="+url.QueryEscape(q), "", nil)
			if w.Code != 200 || w.Body.String() != "[]\n" {
				t.Fatalf("q=%q: %d %s", q, w.Code, w.Body.String())
			}
		}
	})
}
