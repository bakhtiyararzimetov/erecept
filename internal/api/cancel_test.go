package api

import (
	"context"
	"testing"

	"erecept/internal/db/dbtest"
)

// Отменённый врачом рецепт: новый скан не проходит, у того, кто уже сканировал, — пометка cancelled.
func TestCancelledPrescription(t *testing.T) {
	pool := dbtest.New(t)
	e := &env{t: t, pool: pool, h: (&API{DB: pool}).Routes()}
	ctx := context.Background()
	e.newPrescription("cancel1")
	got := e.claim("cancel1", "secret-cancel-0123456789")
	if got.Prescription.Cancelled {
		t.Fatal("ещё не отменён")
	}
	pool.Exec(ctx, `UPDATE prescriptions SET cancelled_at = now() WHERE code = 'cancel1'`)
	w := e.do("GET", "/me/prescriptions", "secret-cancel-0123456789", nil)
	list := decode[[]claimResp](t, w)
	if len(list) != 1 || !list[0].Prescription.Cancelled {
		t.Errorf("пометка отмены: %+v", list)
	}
	e.newPrescription("cancel2")
	pool.Exec(ctx, `UPDATE prescriptions SET cancelled_at = now() WHERE code = 'cancel2'`)
	if w := e.do("POST", "/prescriptions/cancel2/claim", "secret-cancel-0123456789", nil); w.Code != 410 {
		t.Errorf("скан отменённого: %d %s", w.Code, w.Body.String())
	}
}
