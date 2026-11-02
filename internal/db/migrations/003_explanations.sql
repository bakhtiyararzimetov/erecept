-- ИИ-памятка о лекарстве по официальной инструкции НЦЭЛС (JSON отдаётся приложению как есть).
CREATE TABLE medicine_explanations (
    medicine_id BIGINT PRIMARY KEY REFERENCES medicines(id) ON DELETE CASCADE,
    data        JSONB NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Последняя неудачная попытка. retry_at — когда пробовать снова:
-- через 24 ч (нет в реестре, чужое МНН, плохой ответ модели), через 1 ч (сбой сети или сервиса).
CREATE TABLE medicine_explanation_failures (
    medicine_id BIGINT PRIMARY KEY REFERENCES medicines(id) ON DELETE CASCADE,
    reason      TEXT NOT NULL,
    failed_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    retry_at    TIMESTAMPTZ NOT NULL DEFAULT now() + interval '24 hours'
);
