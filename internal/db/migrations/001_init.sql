-- Врачи и аптеки входят в веб по логину и паролю.
CREATE TABLE doctors (
    id            BIGSERIAL PRIMARY KEY,
    login         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    full_name     TEXT NOT NULL,
    clinic        TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE pharmacies (
    id            BIGSERIAL PRIMARY KEY,
    login         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    name          TEXT NOT NULL,
    address       TEXT NOT NULL DEFAULT '',
    phone         TEXT NOT NULL DEFAULT '',
    work_hours    TEXT NOT NULL DEFAULT '', -- например «08:00–22:00» или «Круглосуточно»
    lat           DOUBLE PRECISION NOT NULL DEFAULT 0,
    lng           DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Сессии веба (врач или аптека). Токен лежит в cookie.
CREATE TABLE web_sessions (
    token      TEXT PRIMARY KEY,
    role       TEXT NOT NULL CHECK (role IN ('doctor', 'pharmacy')),
    user_id    BIGINT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);

-- Справочник лекарств. Аналоги = одинаковое действующее вещество.
CREATE TABLE medicines (
    id               BIGSERIAL PRIMARY KEY,
    name             TEXT NOT NULL UNIQUE,
    active_substance TEXT NOT NULL DEFAULT '',
    form             TEXT NOT NULL DEFAULT '' -- «таблетки 500 мг №20»
);
CREATE INDEX medicines_substance_idx ON medicines (lower(active_substance));

-- Что есть в аптеке и почём. Цена в тенге, целым числом.
CREATE TABLE pharmacy_items (
    id          BIGSERIAL PRIMARY KEY,
    pharmacy_id BIGINT NOT NULL REFERENCES pharmacies(id) ON DELETE CASCADE,
    medicine_id BIGINT NOT NULL REFERENCES medicines(id) ON DELETE CASCADE,
    price       INTEGER NOT NULL CHECK (price >= 0),
    in_stock    BOOLEAN NOT NULL DEFAULT true,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (pharmacy_id, medicine_id)
);

-- Пациент = установленное приложение. device_secret генерирует приложение
-- и присылает в заголовке, это и есть его «логин».
CREATE TABLE patients (
    id            BIGSERIAL PRIMARY KEY,
    device_secret TEXT NOT NULL UNIQUE,
    push_token    TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE prescriptions (
    id                BIGSERIAL PRIMARY KEY,
    code              TEXT NOT NULL UNIQUE, -- случайная строка, зашита в QR
    doctor_id         BIGINT NOT NULL REFERENCES doctors(id),
    patient_id        BIGINT REFERENCES patients(id), -- заполняется при сканировании
    patient_name      TEXT NOT NULL,
    patient_birth     DATE NOT NULL,
    comment           TEXT NOT NULL DEFAULT '',
    -- Напоминание «Вода»: каждые N часов с water_from до water_to. 0 = выключено.
    water_every_hours INTEGER NOT NULL DEFAULT 0 CHECK (water_every_hours >= 0),
    water_from        TEXT NOT NULL DEFAULT '09:00',
    water_to          TEXT NOT NULL DEFAULT '21:00',
    start_date        DATE, -- первый день курса = день сканирования
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE prescription_items (
    id              BIGSERIAL PRIMARY KEY,
    prescription_id BIGINT NOT NULL REFERENCES prescriptions(id) ON DELETE CASCADE,
    position        INTEGER NOT NULL DEFAULT 0,
    medicine_name   TEXT NOT NULL,
    medicine_id     BIGINT REFERENCES medicines(id), -- если нашли в справочнике
    dosage          TEXT NOT NULL,                    -- «1 таблетка 500 мг»
    times_per_day   INTEGER NOT NULL CHECK (times_per_day BETWEEN 1 AND 12),
    days            INTEGER NOT NULL CHECK (days BETWEEN 1 AND 365),
    meal            TEXT NOT NULL DEFAULT 'any' CHECK (meal IN ('before', 'after', 'during', 'any')),
    times           TEXT[] NOT NULL,                  -- {"08:00","20:00"}, длина = times_per_day
    photo_url       TEXT NOT NULL DEFAULT ''
);

-- Конкретные приёмы на весь курс. Создаются при сканировании рецепта.
-- По ним считается статистика и ищутся пропуски для push родственнику.
CREATE TABLE doses (
    id              BIGSERIAL PRIMARY KEY,
    prescription_id BIGINT NOT NULL REFERENCES prescriptions(id) ON DELETE CASCADE,
    item_id         BIGINT NOT NULL REFERENCES prescription_items(id) ON DELETE CASCADE,
    scheduled_at    TIMESTAMPTZ NOT NULL,
    day_number      INTEGER NOT NULL, -- «день 3 из 7»
    taken_at        TIMESTAMPTZ,      -- когда нажали «Выпил»
    relative_notified_at TIMESTAMPTZ, -- когда отправили push родственнику
    UNIQUE (item_id, scheduled_at)
);
CREATE INDEX doses_missed_idx ON doses (scheduled_at) WHERE taken_at IS NULL AND relative_notified_at IS NULL;

-- Родственники пациента. Входят по коду-приглашению.
CREATE TABLE relatives (
    id         BIGSERIAL PRIMARY KEY,
    patient_id    BIGINT NOT NULL REFERENCES patients(id) ON DELETE CASCADE,
    device_secret TEXT NOT NULL UNIQUE, -- как у пациента: секрет приложения родственника
    name          TEXT NOT NULL DEFAULT '',
    push_token    TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE invites (
    code       TEXT PRIMARY KEY, -- короткий код, например «482913»
    patient_id BIGINT NOT NULL REFERENCES patients(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ
);
