package web

import (
	"context"
	"time"
)

// ItemStats — соблюдение по одному лекарству.
type ItemStats struct {
	MedicineName         string
	Taken, Missed, Total int
}

// RxStats — соблюдение лечения по рецепту.
type RxStats struct {
	Total, Taken, Missed, Upcoming int
	Percent                        int
	Items                          []ItemStats
}

// prescriptionStats считает приёмы: пропуск = не отмечен и прошло больше 60 минут.
func (wb *Web) prescriptionStats(ctx context.Context, rxID int64, now time.Time) (RxStats, error) {
	var st RxStats
	rows, err := wb.DB.Query(ctx, `
		SELECT i.medicine_name,
		       count(d.id) FILTER (WHERE d.taken_at IS NOT NULL),
		       count(d.id) FILTER (WHERE d.taken_at IS NULL AND d.scheduled_at <= $2),
		       count(d.id)
		FROM prescription_items i LEFT JOIN doses d ON d.item_id = i.id
		WHERE i.prescription_id = $1
		GROUP BY i.id ORDER BY i.position, i.id`, rxID, now.Add(-60*time.Minute))
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var it ItemStats
		if err := rows.Scan(&it.MedicineName, &it.Taken, &it.Missed, &it.Total); err != nil {
			return st, err
		}
		st.Items = append(st.Items, it)
		st.Taken += it.Taken
		st.Missed += it.Missed
		st.Total += it.Total
	}
	if err := rows.Err(); err != nil {
		return st, err
	}
	st.Upcoming = st.Total - st.Taken - st.Missed
	st.Percent = 100
	if d := st.Taken + st.Missed; d > 0 {
		st.Percent = st.Taken * 100 / d
	}
	return st, nil
}
