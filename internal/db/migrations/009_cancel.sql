-- Врач может отменить рецепт: пациент видит пометку, новый скан не проходит.
ALTER TABLE prescriptions ADD COLUMN cancelled_at TIMESTAMPTZ;
