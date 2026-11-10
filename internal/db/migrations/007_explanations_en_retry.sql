-- Неудачный перевод памятки повторяем не раньше этого времени (не каждый час за деньги).
ALTER TABLE medicine_explanations ADD COLUMN en_retry_at TIMESTAMPTZ;
