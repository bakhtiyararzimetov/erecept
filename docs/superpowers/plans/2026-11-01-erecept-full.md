# Электронные рецепты — полная реализация (шаги 2–8)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Довести каркас (шаг 1, задеплоен на Render) до работающего демо: веб врача и аптеки, REST API, Expo-приложение пациента/родственника, push при пропуске, тестовые данные, README.

**Architecture:** Один Go-бинарник: chi-роутер, `/api/*` — JSON для приложения, остальные пути — HTML-страницы (html/template + htmx, шаблоны и CSS встроены через embed). Фоновая горутина `notifier` раз в минуту ищет пропуски и шлёт Expo Push. Приложение Expo (expo-router) хранит всё локально и синхронизирует отметки.

**Tech Stack:** Go 1.25, chi v5, pgx v5, bcrypt (golang.org/x/crypto), skip2/go-qrcode, fergusstrange/embedded-postgres (только тесты); Expo (последний SDK из create-expo-app), TypeScript, expo-router, expo-camera, expo-notifications, @react-native-async-storage/async-storage, @react-native-community/netinfo, expo-location, expo-image-picker, jest-expo.

**Spec:** `docs/superpowers/specs/2026-11-01-erecept-design.md`

## Global Constraints

- Весь пользовательский текст на русском. Без эмодзи, градиентов, теней, анимаций.
- Цвета: фон `#F6F8FA`, карточка `#FFFFFF`, рамка `1px #E5E7EB`, радиус карточки `12px`, текст `#1F2937`, второстепенный `#5B6472`, акцент `#0F766E` (только главные кнопки и активные элементы). Ошибки — `#B42318` (решение плана).
- Шрифт Inter; текст 15px, заголовки 24px. Поля и кнопки высотой ≥44px, радиус 8px. Главная кнопка — заливка акцентом, белый текст; второстепенная — белая с рамкой `#E5E7EB`.
- Веб: белая шапка 64px (логотип «еРецепт» + меню), адаптив до 360px ширины, без горизонтального скролла.
- Приложение: кнопки ≥56px высотой, текст ≥17px (пожилые пациенты).
- Текст напоминания: `«<название>»` в заголовке, тело — `<дозировка>, <до еды|после еды|во время еды|независимо от еды> · день N из M`.
- Часовой пояс Asia/Almaty везде (`time.Local` уже выставлен в main.go). Время приёма — строки `"HH:MM"`.
- Простой код: без интерфейсов-ради-тестов, без DI-фреймворков, ошибки — понятные русские сообщения пользователю и `log.Printf` с деталями.
- Коммиты: от имени автора, **без** строки `Co-Authored-By: Claude`. Сообщения на русском.
- Не трогать Render-сервисы `georomb-api` и `proof-of-build`.

## Review Focus

1. Рецепт сканируют дважды или другим телефоном — приёмы не дублируются (UNIQUE `item_id, scheduled_at`), рецепт перепривязывается, отметки сохраняются. Тест в Task A4.
2. Сервер проспал ночь — notifier не шлёт push за приёмы старше 3 часов и за приёмы до привязки рецепта. Тест в Task A7.
3. Офлайн: «Выпил» без сети — отметка не теряется, уходит при появлении сети, повторная отправка идемпотентна. Тесты в A4 (повторный sync) и B1 (очередь).
4. Кривой ввод врача: пустое название, «раз в день» не совпадает с числом времён, время не `HH:MM`, дата рождения в будущем — форма возвращается с ошибкой и сохранёнными полями, а не 500. Тест в A3.
5. Кривой CSV аптеки: разделитель `;` или `,`, BOM, лишние пробелы, цена «1 250» или с «₸», битая строка — валидные строки загружаются, по битым — список ошибок с номерами строк. Тест в A5.

---

## Структура файлов

```
cmd/app/main.go              — роутинг: /api → api, остальное → web; запуск notifier и автосида
cmd/app/keepalive.go         — (есть) самопинг Render
cmd/seed/main.go             — ручной запуск сида
internal/db/migrations/002_claim_photos.sql
internal/db/dbtest/dbtest.go — dbtest.New(t) на embedded-postgres (только из _test.go)
internal/models/schedule.go  — GenerateDoses, ValidTime
internal/seed/seed.go        — Run(ctx, pool) — 2 врача, 8 аптек, 30 лекарств, цены
internal/web/                — web.go (Handler, роуты), auth.go, doctor.go, pharmacy.go, public.go,
                               csv.go, templates/*.html, static/style.css
internal/api/                — api.go (есть), patient.go, relative.go, search.go, photo.go
internal/notifier/notifier.go
mobile/                      — Expo-приложение (Track B)
README.md
```

## Общие контракты (обе дорожки опираются на них)

### Миграция `002_claim_photos.sql`

```sql
ALTER TABLE prescriptions ADD COLUMN claimed_at TIMESTAMPTZ;
CREATE TABLE item_photos (
    item_id    BIGINT PRIMARY KEY REFERENCES prescription_items(id) ON DELETE CASCADE,
    mime       TEXT NOT NULL,
    data       BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### REST API (все ответы JSON, ошибки `{"error":"<русский текст>"}`)

Аутентификация приложения: заголовок `X-Device-Secret` (16–128 символов `[A-Za-z0-9_-]`). Без него — 401 `{"error":"нет X-Device-Secret"}`. Пациент создаётся автоматически (`INSERT ... ON CONFLICT (device_secret) DO NOTHING`).

| Метод, путь | Тело запроса | Ответ |
|---|---|---|
| `GET /api/health` | — | (есть) |
| `GET /api/prescriptions/{code}` | — | `PrescriptionDTO` (без секрета; 404 «Рецепт не найден») |
| `POST /api/prescriptions/{code}/claim` | — | `{"prescription": PrescriptionDTO, "doses": [DoseDTO]}` |
| `GET /api/me/prescriptions` | — | `[{"prescription":…, "doses":[…]}]` — все рецепты пациента, новые первыми |
| `POST /api/doses/sync` | `{"marks":[{"dose_id":1,"taken_at":"RFC3339"}]}` | `{"updated": N}`; чужие/неизвестные id молча пропускаются; уже отмеченные не перезаписываются |
| `POST /api/push-token` | `{"token":"ExponentPushToken[…]"}` | `{"ok":true}`; обновляет `push_token` у пациента и/или родственника с этим секретом |
| `POST /api/invites` | — | `{"code":"482913","expires_at":"RFC3339"}` — 6 цифр, живёт 24 ч |
| `POST /api/relatives/join` | `{"code":"482913","name":"Айгуль"}` | `{"patient_name":"Иванов Иван Иванович"}`; 400 «Код неверный или истёк» |
| `GET /api/relative/overview` | — | `{"patients":[{"patient_name","taken":N,"missed":N,"upcoming":N,"recent_missed":[{"medicine_name","scheduled_at"}]}]}` |
| `GET /api/medicines/search?q=пара` | — | `[{"id","name","active_substance","form"}]` до 20, ILIKE по name |
| `GET /api/offers?name=…&medicine_id=…&sort=price\|distance&lat=…&lng=…` | — | `OffersDTO` |
| `POST /api/items/{id}/photo` | multipart `photo` (jpeg/png, ≤5 МБ) | `{"photo_url":"/api/photos/{id}"}`; только владелец рецепта |
| `GET /api/photos/{id}` | — | картинка |

```
PrescriptionDTO = {id, code, patient_name, patient_birth:"YYYY-MM-DD", doctor_name, comment,
  water_every_hours, water_from:"HH:MM", water_to:"HH:MM", start_date:"YYYY-MM-DD"|null,
  items:[{id, medicine_name, medicine_id|null, dosage, times_per_day, days, meal, meal_label, times:["08:00"], photo_url}]}
DoseDTO = {id, item_id, scheduled_at:"RFC3339 с +05:00", day_number, total_days, taken_at:"RFC3339"|null}
OffersDTO = {medicine:{id,name,active_substance,form}|null,
  offers:[{pharmacy:{id,name,address,phone,work_hours,lat,lng}, price, in_stock:true, distance_km|null, updated_at}],
  analogs:[{medicine:{…}, min_price, pharmacies_count}],
  analog_note:"Согласуйте замену с врачом или фармацевтом"}
```

Поиск лекарства в `/api/offers`: по `medicine_id`, иначе точное совпадение `lower(name)`, иначе первое `ILIKE name%`. Без совпадения — `medicine:null`, пустые списки (200). Сортировка `distance` без `lat/lng` → как `price`. Расстояние — haversine в Go, округление до 0.1 км.

### Публичные веб-пути

- `GET /r/{code}` — читаемая страница рецепта (без входа). Это содержимое QR.
- `GET /qr/{code}.png` — PNG 512px, ссылка `<PUBLIC_URL>/r/{code}`. `PUBLIC_URL` = env `PUBLIC_URL`, иначе `RENDER_EXTERNAL_URL`, иначе `http://<r.Host>`.

### Учётки сида (их показывает README и подсказка на странице входа)

Врачи: `doctor1` / `doctor123` (Ахметова Динара Сериковна, Поликлиника №5), `doctor2` / `doctor123`. Аптеки: `apteka1`…`apteka8` / `apteka123`.

---

# Track A — Go (основное рабочее дерево, последовательно)

Тесты с базой: `dbtest.New(t)` (пакет `erecept/internal/db/dbtest`) поднимает embedded-postgres на свободном порту в `t.TempDir()`, применяет `db.Migrate`, возвращает `*pgxpool.Pool`, останавливает в `t.Cleanup`. Один сервер на пакет (`sync.Once` + `TestMain` не нужен — допускается сервер на тест, если быстро; если >3 с — общий через `sync.Once` и `TRUNCATE ... RESTART IDENTITY CASCADE` в `New`). Бинарники уже в `~/.embedded-postgres-go` (v18.3.0) — использовать `embeddedpostgres.V18`.

### Task A1: Миграция 002, генерация приёмов, тестовая база

**Files:**
- Create: `internal/db/migrations/002_claim_photos.sql`, `internal/db/dbtest/dbtest.go`, `internal/models/schedule.go`, `internal/models/schedule_test.go`, `internal/db/db_test.go`
- Modify: `internal/models/models.go` (поле `ClaimedAt *time.Time` в Prescription, `json:"-"`)

**Interfaces:**
- Produces: `func ValidTime(s string) bool` (`HH:MM`, 00–23, 00–59); `func GenerateDoses(item PrescriptionItem, start time.Time) []Dose` — `start` = полночь первого дня в `time.Local`; для каждого дня `d=0..Days-1` и каждого `t` из `Times` (отсортированы) — `Dose{ItemID: item.ID, PrescriptionID: item.PrescriptionID, ScheduledAt: start+d дней в t, DayNumber: d+1}`; `func dbtest.New(t testing.TB) *pgxpool.Pool` — чистая база с применёнными миграциями.

- [ ] **Step 1: Тесты**

```go
func TestGenerateDoses(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Almaty")
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, loc)
	item := PrescriptionItem{ID: 7, PrescriptionID: 3, Days: 3, TimesPerDay: 2, Times: []string{"20:00", "08:00"}}
	ds := GenerateDoses(item, start)
	// 6 приёмов, по порядку, день 1..3
	if len(ds) != 6 { t.Fatalf("len=%d", len(ds)) }
	if !ds[0].ScheduledAt.Equal(time.Date(2026, 11, 1, 8, 0, 0, 0, loc)) || ds[0].DayNumber != 1 { t.Fatal(ds[0]) }
	if !ds[5].ScheduledAt.Equal(time.Date(2026, 11, 3, 20, 0, 0, 0, loc)) || ds[5].DayNumber != 3 { t.Fatal(ds[5]) }
}
func TestValidTime(t *testing.T) { /* "08:00","23:59" true; "8:00","24:00","12:60","ab:cd","" false */ }
func TestMigrateTwice(t *testing.T) { pool := dbtest.New(t); if err := db.Migrate(ctx, pool); err != nil { t.Fatal(err) } /* claimed_at и item_photos существуют */ }
```

- [ ] **Step 2:** `go test ./internal/models/ ./internal/db/` — FAIL (нет функций).
- [ ] **Step 3:** Реализовать. `go get github.com/fergusstrange/embedded-postgres@v1.34.0`.
- [ ] **Step 4:** `go test ./internal/...` — PASS; `go vet ./...` чисто.
- [ ] **Step 5:** Commit `Миграция 002, генерация приёмов, тестовая база`.

### Task A2: Сид, вход, общий макет и стили

**Files:**
- Create: `internal/seed/seed.go`, `internal/seed/seed_test.go`, `cmd/seed/main.go`, `internal/web/web.go`, `internal/web/auth.go`, `internal/web/auth_test.go`, `internal/web/templates/layout.html`, `internal/web/templates/login.html`, `internal/web/static/style.css`
- Modify: `cmd/app/main.go` (автосид если `SELECT count(*) FROM doctors` = 0 и `SEED_DEMO != "0"`; `r.Mount("/", web.New(pool).Routes())` после `/api`)

**Interfaces:**
- Produces: `seed.Run(ctx, pool) error` (идемпотентно, `ON CONFLICT DO NOTHING`); `web.New(pool *pgxpool.Pool) *Web`; `(*Web).Routes() http.Handler`; `(*Web).render(w, r, page string, data map[string]any)` — рендерит `layout.html` + `<page>.html`, в data всегда кладёт `User` (текущий `*SessionUser` или nil); `type SessionUser struct{ Role string; ID int64; Name string }`; middleware `requireRole(role string)`; `currentUser(r) *SessionUser` (из context).
- Маршруты: `GET /` → редирект по роли (`/doctor`, `/pharmacy`) или `/login`; `GET/POST /login`; `POST /logout`; `GET /static/*`.

Сид: 30 лекарств с реальными МНН и формами (парацетамол, ибупрофен, амоксициллин, азитромицин, омепразол, метформин, лизиноприл, амлодипин, аторвастатин, цетиризин, лоратадин, амброксол, ацетилцистеин, метронидазол, ципрофлоксацин, дротаверин, панкреатин, лоперамид, нимесулид, диклофенак, валсартан, бисопролол, кларитромицин, флуконазол, ранитидин→фамотидин, монтелукаст, левоцетиризин, эналаприл, ацикловир, мелоксикам; минимум 5 пар «оригинал/дженерик» с одинаковым веществом, напр. «Нурофен» и «Ибупрофен»). 8 аптек в Алматы с вымышленными названиями, реальными адресами-улицами и координатами в пределах 43.20–43.28 / 76.85–76.97, разные часы (одна «Круглосуточно»). Каждая аптека — 18–26 лекарств, цена = базовая × коэффициент аптеки (0.85–1.20) с детерминированным разбросом, 1–2 позиции `in_stock=false`. Пароли bcrypt cost 10.

Сессия: токен 32 байта hex, cookie `erecept_session`, HttpOnly, SameSite=Lax, Secure если `r.TLS != nil || X-Forwarded-Proto == "https"`, 7 дней. Логин ищет сначала `doctors`, потом `pharmacies`. Неверно — «Неверный логин или пароль», поле логина сохраняется. На странице входа подсказка с демо-учётками.

Макет: шапка 64px — слева «еРецепт» (акцентный квадрат 28px + текст), справа меню по роли (врач: «Новый рецепт», «Мои рецепты»; аптека: «Мои лекарства») и кнопка «Выйти». Inter из Google Fonts, htmx `https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js`. CSS-классы для всех задач: `.card`, `.btn`, `.btn-primary`, `.btn-secondary`, `.field`, `.grid-2` (форма слева, карточка справа; ≤900px — одна колонка), `.table` (на ≤640px — превращается в карточки построчно через `data-label`), `.muted`, `.error`, `.badge`, `.stat`.

- [ ] **Step 1: Тесты** — `TestSeedCounts` (2 врача, 8 аптек, 30 лекарств; повторный `Run` не меняет числа; у каждой аптеки ≥18 позиций; есть ≥5 веществ с ≥2 лекарствами); `TestLoginDoctor` (POST /login doctor1/doctor123 → 303 на `/doctor`, cookie выставлена; GET `/doctor` с cookie → не 303 на /login); `TestLoginWrong` (200, тело содержит «Неверный логин или пароль»); `TestPharmacyCannotOpenDoctor` (apteka1 → GET /doctor → 403 или редирект на /pharmacy). Использовать `httptest` + `dbtest.New`.
- [ ] **Step 2:** FAIL. **Step 3:** реализовать (`go get golang.org/x/crypto/bcrypt`). Временные страницы `/doctor`, `/pharmacy` — заглушки-карточки, их заменят A3/A5.
- [ ] **Step 4:** `go test ./...` PASS; `go run ./cmd/app` с локальной БД не нужен.
- [ ] **Step 5:** Commit `Вход врача и аптеки, макет, тестовые данные`.

### Task A3: Рецепт врача, QR, печать, публичная страница

**Files:**
- Create: `internal/web/doctor.go`, `internal/web/doctor_test.go`, `internal/web/public.go`, `internal/web/templates/doctor_new.html`, `doctor_list.html`, `doctor_rx.html`, `rx_print.html`, `rx_public.html`, `item_row.html`
- Modify: `internal/web/web.go` (роуты)

**Interfaces:**
- Consumes: `render`, `requireRole`, `models.ValidTime`, `models.MealLabel`.
- Produces: `func (wb *Web) loadPrescription(ctx, id int64) (models.Prescription, error)` и `loadPrescriptionByCode(ctx, code string)` (с Items, по `position`) — используются в A7; `func publicURL(r *http.Request) string`; `func newCode() string` (10 символов base32 без `=`, нижний регистр).
- Маршруты (роль doctor): `GET /doctor` (форма нового рецепта слева + справа карточка-подсказка «QR появится после сохранения»), `GET /doctor/item-row?i=N` (htmx, строка таблицы), `POST /doctor/prescriptions`, `GET /doctor/prescriptions` (список: ФИО, дата, статус «Ожидает сканирования»/«Курс идёт», % выполнения), `GET /doctor/prescriptions/{id}` (слева — рецепт только для чтения + статистика (A7), справа — QR-карточка: картинка, код, кнопки «Распечатать рецепт» (главная, открывает `/doctor/prescriptions/{id}/print`) и «Новый рецепт»), `GET /doctor/prescriptions/{id}/print` (A4-лист, без шапки, `window.print()` по кнопке, QR + таблица + подпись врача). Чужой рецепт → 404. Публичные: `GET /r/{code}`, `GET /qr/{code}.png`.

Форма: поля `patient_name`, `patient_birth` (date), `comment`, `water_every_hours` (select: «Не напоминать», 1, 2, 3, 4), `water_from`, `water_to` (time); строки лекарств с индексами `items[i].name|dosage|times_per_day|days|meal|times` — `times` одной строкой «08:00, 20:00». При смене «раз в день» htmx не нужен: строка `times` предзаполняется по умолчанию (1→08:00; 2→08:00, 20:00; 3→08:00, 14:00, 20:00; 4→08:00, 12:00, 16:00, 20:00) через маленький inline-скрипт. `name` — `<input list="medicines">` с `<datalist>` из справочника. Кнопка «Удалить» у строки (если строк >1). Сохранение в одной транзакции; `medicine_id` подставляется, если `lower(name)` совпал со справочником. Ошибки валидации — над формой списком + подсветка, форма с введёнными данными, статус 422.

- [ ] **Step 1: Тесты** — `TestCreatePrescription` (POST от doctor1 с 2 лекарствами → 303 на `/doctor/prescriptions/{id}`, в БД 2 items, `times` = `{08:00,20:00}`, `medicine_id` заполнен для «Парацетамол»); `TestCreatePrescriptionInvalid` (таблица: пустое имя пациента; дата рождения в будущем; «раз в день»=2 и одно время; время «25:00»; нет ни одного лекарства → 422, тело содержит соответствующий русский текст ошибки и введённое ФИО); `TestQRPNG` (GET /qr/{code}.png → 200, `image/png`); `TestPublicPage` (GET /r/{code} → 200, содержит название лекарства и «после еды»); `TestOtherDoctorGets404`.
- [ ] **Step 2:** FAIL. **Step 3:** реализовать (`go get github.com/skip2/go-qrcode`).
- [ ] **Step 4:** `go test ./...` PASS.
- [ ] **Step 5:** Commit `Страница врача: рецепт, QR-код, печать`.

### Task A4: REST API пациента и родственника, фото

**Files:**
- Create: `internal/api/patient.go`, `internal/api/relative.go`, `internal/api/photo.go`, `internal/api/dto.go`, `internal/api/api_test.go`
- Modify: `internal/api/api.go` (роуты, middleware `deviceSecret` кладёт секрет в context, CORS `*` для GET/POST с заголовком `X-Device-Secret` — нужно для Expo web)

**Interfaces:**
- Consumes: `models.GenerateDoses`, миграция 002.
- Produces: всё из таблицы «REST API», кроме `/api/medicines/search` и `/api/offers` (A6). `func (a *API) patientID(ctx, secret string) (int64, error)` (upsert).

Claim в транзакции: найти рецепт по code (404); `UPDATE prescriptions SET patient_id=$p, start_date=COALESCE(start_date, current_date), claimed_at=COALESCE(claimed_at, now())`; для каждого item — `GenerateDoses(item, start_date в 00:00 local)` и `INSERT ... ON CONFLICT (item_id, scheduled_at) DO NOTHING`; вернуть все doses рецепта по `scheduled_at`. `total_days` в DoseDTO = `days` его item. `doctor_name` — из doctors.

Relative overview: для каждого пациента, к которому привязан родственник: `patient_name` из его последнего рецепта; `taken` = отмеченные, `missed` = неотмеченные с `scheduled_at <= now()-60 min`, `upcoming` = остальные; `recent_missed` — до 5 последних пропущенных.

- [ ] **Step 1: Тесты** — `TestClaimCreatesDoses` (рецепт с 1 лекарством 2×3 дня → 6 doses, `start_date` = сегодня, `day_number` 1..3, `total_days`=3); `TestClaimTwiceNoDuplicates` (второй claim тем же секретом → те же 6 id; claim другим секретом → те же 6 id, `patient_id` новый, отметки, сделанные первым, сохранились); `TestSyncIdempotent` (sync двух отметок → updated 2; повтор → updated 0, `taken_at` не изменился; чужой dose_id → не обновлён); `TestNoSecret401`; `TestInviteJoinOverview` (пациент создаёт invite → родственник join с другим секретом → overview содержит patient_name; повторный join тем же кодом → 400 «Код неверный или истёк»); `TestPushTokenStored`; `TestPhotoUploadOwnerOnly` (владелец → 200 и GET photo отдаёт те же байты; чужой секрет → 404).
- [ ] **Step 2:** FAIL. **Step 3:** реализовать. **Step 4:** `go test ./...` PASS.
- [ ] **Step 5:** Commit `REST API: сканирование рецепта, отметки, родственники, фото`.

### Task A5: Кабинет аптеки (вручную и CSV)

**Files:**
- Create: `internal/web/pharmacy.go`, `internal/web/csv.go`, `internal/web/csv_test.go`, `internal/web/pharmacy_test.go`, `internal/web/templates/pharmacy.html`, `pharmacy_row.html`
- Modify: `internal/web/web.go`

**Interfaces:**
- Produces: `type CSVRow struct{ Name, Substance, Form string; Price int; InStock bool }`; `func ParseCSV(r io.Reader) (rows []CSVRow, errs []string)` — заголовок обязателен: `название;действующее вещество;форма;цена;наличие` (порядок колонок по заголовку, регистр и пробелы не важны; `форма`, `действующее вещество`, `наличие` необязательны). Разделитель `;` или `,` (определяется по заголовку), BOM срезается, цена: убрать пробелы/неразрывные пробелы/«₸»/«тг», целое ≥0; наличие: пусто/да/есть/1/true → true, нет/0/false → false. Ошибка — `"строка 5: цена «abc» — не число"`.
- Маршруты (роль pharmacy): `GET /pharmacy` (сверху карточка аптеки: название, адрес, часы; форма «Добавить лекарство»: название (datalist), действующее вещество, форма, цена, «В наличии»; блок «Загрузить CSV» с примером формата и ссылкой `GET /pharmacy/sample.csv`; таблица позиций с поиском по названию (htmx `hx-get /pharmacy?q=` `hx-select`)); `POST /pharmacy/items` (upsert по `pharmacy_id, medicine_id`; новое лекарство создаётся в справочнике); `POST /pharmacy/items/{id}` (htmx: изменить цену/наличие, возвращает строку); `POST /pharmacy/items/{id}/delete`; `POST /pharmacy/csv` (multipart `file`, итог: «Загружено N, ошибок M» + список ошибок).

- [ ] **Step 1: Тесты** — `TestParseCSV` (таблица: `;` + BOM; `,`; цена «1 250 ₸» → 1250; «нет» → false; битая строка даёт ошибку с номером, остальные строки возвращаются; без колонки «цена» → одна ошибка про заголовок); `TestPharmacyAddAndUpdate` (apteka1 добавляет новое лекарство «Тестин» → появилось в medicines и pharmacy_items; повторное добавление обновляет цену, а не дублирует; POST /pharmacy/items/{id} меняет in_stock); `TestPharmacyCSVUpload` (3 строки, 1 битая → 2 позиции, ответ содержит «Загружено 2»); чужая позиция → 404.
- [ ] **Step 2:** FAIL. **Step 3:** реализовать. **Step 4:** `go test ./...` PASS.
- [ ] **Step 5:** Commit `Кабинет аптеки: лекарства, цены, загрузка CSV`.

### Task A6: API поиска по аптекам

**Files:**
- Create: `internal/api/search.go`, `internal/api/search_test.go`
- Modify: `internal/api/api.go`

**Interfaces:**
- Produces: `GET /api/medicines/search`, `GET /api/offers` по контракту; `func distanceKm(lat1, lng1, lat2, lng2 float64) float64`.

- [ ] **Step 1: Тесты** — `TestDistanceKm` (Алматы 43.2389,76.8897 → 43.2567,76.9286 ≈ 3.6 км ±0.2); `TestOffersSortPrice` (по сиду, «Парацетамол»: только `in_stock`, цены не убывают, у каждой аптеки есть адрес и часы); `TestOffersSortDistance` (с lat/lng: `distance_km` не убывает, не null); `TestOffersAnalogs` (лекарство с парой по веществу → `analogs` не пуст, сам препарат в analogs отсутствует, `analog_note` = «Согласуйте замену с врачом или фармацевтом»); `TestOffersUnknown` (`name=Несуществующее` → 200, `medicine` null); `TestMedicineSearch` (`q=пара` → содержит «Парацетамол»).
- [ ] **Step 2:** FAIL. **Step 3:** реализовать. **Step 4:** PASS.
- [ ] **Step 5:** Commit `API поиска лекарств по аптекам`.

### Task A7: Push родственнику и статистика врача

**Files:**
- Modify: `internal/notifier/notifier.go`, `cmd/app/main.go` (`go notifier.New(pool).Run(ctx)`), `internal/web/doctor.go`, `internal/web/templates/doctor_rx.html`, `doctor_list.html`
- Create: `internal/notifier/notifier_test.go`, `internal/web/stats.go`, `internal/web/stats_test.go`

**Interfaces:**
- Produces: `type Notifier struct{ DB *pgxpool.Pool; PushURL string; Client *http.Client; Now func() time.Time }`; `func New(pool) *Notifier` (PushURL `https://exp.host/--/api/v2/push/send`, Now `time.Now`); `func (n *Notifier) Run(ctx)` — тикер 1 мин, первый проход сразу; `func (n *Notifier) CheckOnce(ctx) (sent int, err error)`.
- `type RxStats struct{ Total, Taken, Missed, Upcoming int; Percent int; Items []ItemStats }`, `type ItemStats struct{ MedicineName string; Taken, Missed, Total int }`; `func (wb *Web) prescriptionStats(ctx, rxID int64, now time.Time) (RxStats, error)`. `Percent = Taken*100 / (Taken+Missed)`, при знаменателе 0 — 100.

CheckOnce: выбрать doses `taken_at IS NULL AND relative_notified_at IS NULL AND scheduled_at <= now-60min AND scheduled_at > now-3h AND scheduled_at >= prescriptions.claimed_at`; для каждой — все родственники пациента с непустым `push_token`; одно сообщение Expo на родственника: `{"to": token, "title": "Пропущен приём лекарства", "body": "<ФИО> не отметил «<лекарство>» в <HH:MM>", "sound": "default", "data": {"type":"missed","dose_id":id}}` (массив сообщений одним POST). `relative_notified_at = now` ставится даже без родственников (чтобы не перебирать заново). Ошибка HTTP — `log.Printf`, отметку не ставить, повторить в следующую минуту.

Страница рецепта врача: блок «Соблюдение лечения» — 4 `.stat` (Выполнено %, Отмечено, Пропущено, Впереди) и таблица по лекарствам; до сканирования — «Пациент ещё не отсканировал рецепт». Список рецептов — колонка «%».

- [ ] **Step 1: Тесты** — `TestCheckOnceSends` (httptest-сервер как PushURL; dose 70 мин назад, родственник с токеном → 1 запрос, тело содержит ФИО и название, `relative_notified_at` выставлен; второй `CheckOnce` → 0); `TestCheckOnceSkipsOld` (dose 4 ч назад → 0 запросов); `TestCheckOnceSkipsBeforeClaim` (dose раньше `claimed_at` → 0); `TestCheckOnceSkipsTaken`; `TestCheckOncePushError` (сервер 500 → `relative_notified_at` остаётся NULL); `TestPrescriptionStats` (3 отмечено, 1 пропущено, 2 впереди → Percent 75).
- [ ] **Step 2:** FAIL. **Step 3:** реализовать. **Step 4:** `go test ./...` PASS.
- [ ] **Step 5:** Commit `Push родственнику при пропуске, статистика для врача`.

---

# Track B — Expo-приложение (отдельный git worktree, параллельно с Track A)

Работает только в `mobile/`. API берётся из `process.env.EXPO_PUBLIC_API_URL ?? "https://erecept.onrender.com"`. Пока Track A не задеплоен, проверка — `jest` + `tsc --noEmit` + `npx expo export --platform android` (сборка бандла без ошибок).

### Task B1: Каркас, хранилище, расписание (с тестами)

**Files:**
- Create: `mobile/` через `npx create-expo-app@latest mobile --template blank-typescript --no-install`, затем `npx expo install expo-router expo-camera expo-notifications @react-native-async-storage/async-storage @react-native-community/netinfo expo-location expo-image-picker expo-linking expo-constants expo-status-bar react-native-safe-area-context react-native-screens expo-font @expo-google-fonts/inter` и `npm i -D jest-expo jest @types/jest`. `mobile/lib/types.ts`, `lib/api.ts`, `lib/storage.ts`, `lib/schedule.ts`, `lib/sync.ts`, `lib/theme.ts`, `lib/__tests__/schedule.test.ts`, `lib/__tests__/sync.test.ts`.
- `app.json`: `name` «еРецепт», `slug` `erecept`, `scheme` `erecept`, плагины expo-router, expo-camera (`cameraPermission`: «Камера нужна, чтобы отсканировать QR-код рецепта»), expo-notifications, expo-location (текст про поиск ближайших аптек), expo-image-picker; `main` = `expo-router/entry`.

**Interfaces:**
- Produces (`lib/types.ts`): типы `Prescription`, `Item`, `Dose`, `Offer`, `OffersResponse`, `RelativeOverview` — поля как в DTO контракта (snake_case как в JSON). `Mark = {dose_id: number; taken_at: string}`. `Role = "patient" | "relative"`.
- `lib/api.ts`: `api.claim(code)`, `api.myPrescriptions()`, `api.sync(marks)`, `api.pushToken(token)`, `api.createInvite()`, `api.joinRelative(code, name)`, `api.relativeOverview()`, `api.offers(params)`, `api.uploadPhoto(itemId, uri)`; все шлют `X-Device-Secret`; ошибка → `throw new ApiError(message, status)` с текстом `error` из ответа или «Нет связи с сервером».
- `lib/storage.ts`: `getDeviceSecret()` (создаёт 32 hex и сохраняет), `getRole()/setRole()`, `loadCourses(): Promise<Course[]>` / `saveCourses()` где `Course = {prescription: Prescription; doses: Dose[]}`, `getPendingMarks()/setPendingMarks()`.
- `lib/schedule.ts` (чистые функции, без expo):
  - `extractCode(scanned: string): string | null` — из `https://…/r/<code>` или голого кода `[a-z0-9]{6,32}`.
  - `reminderText(item: Item, dose: Dose): {title: string; body: string}` → `{title: "Парацетамол", body: "1 таблетка 500 мг, после еды · день 3 из 7"}`.
  - `waterTimes(p: Prescription, day: Date): Date[]` — от `water_from` до `water_to` включительно с шагом `water_every_hours`; `[]` если 0; дни курса = от `start_date` до последнего дня самого длинного лекарства.
  - `upcomingNotifications(courses: Course[], now: Date, limit = 60): PlannedNotification[]` — `{key: string; at: Date; title; body; kind: "dose"|"water"; doseId?: number}`, только `at > now` и только неотмеченные дозы, отсортированы, первые `limit`. Вода: `title` «Время выпить воды», `body` «Стакан воды».
  - `todayList(courses, now): TodayEntry[]` — дозы сегодняшнего дня, `{dose, item, status: "taken"|"missed"|"due"|"upcoming"}` (`missed` — не отмечено и прошло ≥60 мин; `due` — от −30 до +60 мин).
  - `courseProgress(course, now): {taken: number; missed: number; total: number; percent: number}` — percent как у сервера.
- `lib/sync.ts`: `markTaken(doseId: number, at: Date)` — пишет в курсы локально и добавляет в очередь; `flushMarks(): Promise<number>` — шлёт очередь, при успехе очищает только отправленное; при ошибке сети — оставляет.

- [ ] **Step 1: Тесты** (`jest`): `extractCode` (ссылка, ссылка со слешем в конце, голый код, мусор → null); `reminderText` (точная строка выше); `waterTimes` (каждые 2 ч 09:00–21:00 → 7 времён; 0 → []); `upcomingNotifications` (не берёт прошедшие и отмеченные; лимит 60; сортировка); `todayList` статусы; `courseProgress` (3 taken, 1 missed → 75); `flushMarks` (мок `api.sync`: успех очищает очередь; ошибка оставляет; повторный `markTaken` той же дозы не дублирует запись в очереди).
- [ ] **Step 2:** `npx jest` FAIL. **Step 3:** реализовать. **Step 4:** `npx jest` PASS, `npx tsc --noEmit` чисто.
- [ ] **Step 5:** Commit `Expo: каркас, хранилище, расписание`.

### Task B2: Скан QR и экран «Сегодня»

**Files:**
- Create: `mobile/app/_layout.tsx` (шрифт Inter, Stack), `app/(tabs)/_layout.tsx` (4 вкладки: «Сегодня», «Курс», «Где купить», «Настройки»; активный цвет `#0F766E`, подписи 14px, высота таббара ≥64), `app/(tabs)/index.tsx`, `app/scan.tsx` (модальный), `components/Button.tsx` (`variant: "primary"|"secondary"`, высота 56, радиус 8, текст 17), `components/Card.tsx`.

Поведение: при старте — `flushMarks()` и обновление курсов `api.myPrescriptions()` (если сеть есть, иначе локальные). «Сегодня» без курсов — пустое состояние: текст «Отсканируйте QR-код с рецепта врача» и главная кнопка «Сканировать рецепт». С курсами — дата крупно, список приёмов (время 22px жирным, название, доза, «после еды», бейдж статуса: «Выпито» / «Пропущено» / «Пора принять»), у неотмеченных — главная кнопка «Выпил» на всю ширину; «Где купить» второстепенной ссылкой у лекарства (переход во вкладку с `?name=`). Сверху — вторичная кнопка «Сканировать ещё рецепт». Офлайн — серая плашка «Нет интернета. Отметки сохранятся и отправятся позже». Скан: `CameraView` с `barcodeScannerSettings={{barcodeTypes:["qr"]}}`, рамка-подсказка, текст «Наведите камеру на QR-код рецепта»; без разрешения — объяснение и кнопка «Разрешить камеру»; после успешного `claim` — сохранить курс, вызвать `rescheduleAll()` (B3; до B3 — no-op экспорт в `lib/notifications.ts`), вернуться на «Сегодня». Ошибки — понятный текст, кнопка «Попробовать снова». Также ручной ввод кода (поле + «Загрузить») — на случай, если камера не работает на демо.

- [ ] **Step 1:** `npx tsc --noEmit` и `npx jest` PASS; `npx expo export --platform android --output-dir /tmp/…` без ошибок.
- [ ] **Step 2:** Commit `Expo: сканирование рецепта и экран «Сегодня»`.

### Task B3: Локальные уведомления с кнопками

**Files:**
- Create/Modify: `mobile/lib/notifications.ts`, `app/_layout.tsx` (инициализация, слушатель ответов)

**Interfaces:**
- Produces: `setupNotifications()` — `setNotificationHandler` (показывать баннер и звук), Android-канал `reminders` (важность HIGH), категория `dose` с действиями `TAKEN` «Выпил» и `SNOOZE` «Через 15 мин» (`opensAppToForeground: false` для обоих); `rescheduleAll()` — `cancelAllScheduledNotificationsAsync()` → `upcomingNotifications(loadCourses(), new Date(), 60)` → `scheduleNotificationAsync` с триггером `{type: DATE, date}` и `categoryIdentifier: "dose"` только для доз, `data: {doseId}`; `handleResponse(resp)` — `TAKEN` → `markTaken` + `flushMarks` + `rescheduleAll`; `SNOOZE` → одно уведомление через 15 мин с тем же текстом и категорией; обычный тап → открыть «Сегодня». Слушатели в `_layout.tsx` + обработка `getLastNotificationResponseAsync()` при холодном старте. `rescheduleAll()` также при `AppState` → `active` и после каждого `markTaken` из UI. Запрос разрешения — при первом успешном скане (с объяснением на экране).

- [ ] **Step 1:** тест `upcomingNotifications` уже есть; добавить тест `snoozeTime(now)` = now+15 мин в `schedule.ts`. `npx jest`, `npx tsc --noEmit`, `expo export` — PASS.
- [ ] **Step 2:** Commit `Expo: локальные напоминания о лекарствах и воде с кнопками`.

### Task B4: «Курс», «Где купить», «Настройки», режим родственника, фото

**Files:**
- Create: `mobile/app/(tabs)/course.tsx`, `app/(tabs)/pharmacies.tsx`, `app/(tabs)/settings.tsx`, `app/relative.tsx`, `app/join.tsx`, `lib/push.ts`

Поведение:
- **Курс:** по каждому рецепту — ФИО врача, даты, большой процент выполнения, полоса прогресса (акцент), «Отмечено / Пропущено / Осталось»; по лекарствам — строки с процентом и кнопкой «Фото таблетки» (`expo-image-picker` камера или галерея → `api.uploadPhoto`, миниатюра 56px). Комментарий врача.
- **Где купить:** сверху выбор лекарства (чипы из курсов + поле поиска); переключатель «По цене / По расстоянию» (сегменты 48px; активный — акцент); расстояние — `expo-location` (если отказ — сортировка по цене и плашка «Разрешите геолокацию, чтобы видеть ближайшие»). Карточка аптеки: название, цена крупно «1 250 ₸», адрес, часы, расстояние «2,4 км», кнопки «На карте» (`Linking.openURL("https://maps.google.com/?q=<lat>,<lng>")`) и «Позвонить» (`tel:`). Блок «Аналоги» с серой плашкой `analog_note` и минимальной ценой. Принимает параметр `name` из «Сегодня».
- **Настройки:** «Пригласить родственника» → `api.createInvite()` → крупный код 6 цифр (36px, разреженно), «Действует 24 часа», кнопка «Поделиться» (`Share.share` с текстом «Код для приложения еРецепт: 482913»); «Я родственник — ввести код» → `join.tsx`; статус уведомлений; версия API-адреса мелко.
- **join.tsx:** поля «Код из приложения пациента», «Ваше имя» → `api.joinRelative` → `setRole("relative")`, регистрация push (`lib/push.ts`) → `relative.tsx`.
- **relative.tsx** (если `role === "relative"`, `_layout` открывает его вместо вкладок): по каждому пациенту — ФИО, сегодня «Выпито N, пропущено M», список последних пропусков, кнопка «Обновить»; внизу второстепенная «Выйти из режима родственника».
- **lib/push.ts:** `registerPush()` — разрешение, `getExpoPushTokenAsync({projectId: Constants.expoConfig?.extra?.eas?.projectId})`; если `projectId` нет или ошибка — вернуть null и показать в Настройках «Push недоступен: нужна сборка с EAS (см. README)», без падения; при успехе `api.pushToken`. Вызывается и для пациента (после скана).

- [ ] **Step 1:** `npx jest`, `npx tsc --noEmit`, `expo export --platform android` — PASS.
- [ ] **Step 2:** Commit `Expo: курс, где купить, настройки, режим родственника`.

---

# Финал

### Task F1: Слияние, README, деплой

- [ ] Слить worktree Track B в `main` (конфликтов быть не должно: разные каталоги).
- [ ] `README.md`: что это, демо-ссылка, учётки, сценарий показа по шагам (врач → QR → телефон → «Выпил» → аптеки → статистика), запуск локально (`cp .env.example .env && docker compose up --build`; без Docker — `go run ./cmd/app` с внешним Postgres), запуск приложения (`cd mobile && npm i && npx expo start`, Expo Go, `EXPO_PUBLIC_API_URL`), формат CSV, push родственнику через EAS (`npx eas init`, `npx eas build --profile development`), тесты (`go test ./...`, `cd mobile && npx jest`), ограничения бесплатного Render.
- [ ] `.env.example`: добавить `PUBLIC_URL=`, `SEED_DEMO=1`.
- [ ] `go test ./...`, `go vet ./...`, `cd mobile && npx jest && npx tsc --noEmit` — всё PASS. Commit, `git push`.
- [ ] Дождаться деплоя Render (`live`), проверить вживую: `/api/health`, вход doctor1, создание рецепта, `/qr/<code>.png`, `POST /api/prescriptions/<code>/claim` с секретом, `/api/offers?name=Парацетамол`, вход apteka1.

### Task F2: Параллельные проверки (2 агента одновременно)

- **Агент «Ошибки»:** читает весь diff `main` от `2b93e84`, ищет баги корректности, гонки, SQL, обработку ошибок, безопасность (доступ к чужим рецептам/позициям, XSS в шаблонах, CSRF-риски форм). Список находок с файлом и строкой.
- **Агент «Юзабилити»:** сверяет веб (шаблоны/CSS) и экраны Expo с разделом «Дизайн» спеки и Global Constraints, проходит сценарии глазами пожилого пациента, врача и фармацевта; проверяет тексты (русский, коротко, без эмодзи), размеры, мобильную вёрстку, пустые состояния и ошибки. Список находок.
- Исправить подтверждённые находки, тесты PASS, commit, push, повторная живая проверка.
