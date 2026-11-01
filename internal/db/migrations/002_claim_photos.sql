-- Когда пациент привязал рецепт к своему телефону.
ALTER TABLE prescriptions ADD COLUMN claimed_at TIMESTAMPTZ;

-- Фото упаковки лекарства (до 5 МБ, хранится прямо в базе).
CREATE TABLE item_photos (
    item_id    BIGINT PRIMARY KEY REFERENCES prescription_items(id) ON DELETE CASCADE,
    mime       TEXT NOT NULL,
    data       BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
