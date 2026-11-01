package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

const maxPhoto = 5 << 20

func (a *API) uploadPhoto(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	itemID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "Лекарство не найдено")
		return
	}
	pid, err := a.patientID(ctx, secretFrom(ctx))
	if err != nil {
		serverError(w, "пациент", err)
		return
	}
	var owned bool
	err = a.DB.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM prescription_items i
		JOIN prescriptions p ON p.id = i.prescription_id WHERE i.id = $1 AND p.patient_id = $2)`, itemID, pid).Scan(&owned)
	if err != nil {
		serverError(w, "владелец", err)
		return
	}
	if !owned {
		writeError(w, http.StatusNotFound, "Лекарство не найдено")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPhoto+(1<<20))
	file, _, err := r.FormFile("photo")
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusBadRequest, "Фото больше 5 МБ")
			return
		}
		writeError(w, http.StatusBadRequest, "неверный запрос")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxPhoto+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "неверный запрос")
		return
	}
	if len(data) > maxPhoto {
		writeError(w, http.StatusBadRequest, "Фото больше 5 МБ")
		return
	}
	mime := http.DetectContentType(data)
	if mime != "image/jpeg" && mime != "image/png" {
		writeError(w, http.StatusBadRequest, "Нужно фото в формате JPEG или PNG")
		return
	}

	tx, err := a.DB.Begin(ctx)
	if err != nil {
		serverError(w, "транзакция", err)
		return
	}
	defer tx.Rollback(ctx)
	url := "/api/photos/" + strconv.FormatInt(itemID, 10)
	if _, err := tx.Exec(ctx, `INSERT INTO item_photos (item_id, mime, data) VALUES ($1, $2, $3)
		ON CONFLICT (item_id) DO UPDATE SET mime = EXCLUDED.mime, data = EXCLUDED.data, created_at = now()`,
		itemID, mime, data); err != nil {
		serverError(w, "фото", err)
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE prescription_items SET photo_url = $1 WHERE id = $2`, url, itemID); err != nil {
		serverError(w, "фото", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		serverError(w, "коммит", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"photo_url": url})
}

func (a *API) getPhoto(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "Фото не найдено")
		return
	}
	var mime string
	var data []byte
	err = a.DB.QueryRow(r.Context(), `SELECT mime, data FROM item_photos WHERE item_id = $1`, itemID).Scan(&mime, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Фото не найдено")
		return
	}
	if err != nil {
		serverError(w, "фото", err)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Write(data)
}
