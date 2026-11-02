# ИИ-памятка, «Мне плохо», повтор напоминания — план

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** Добавить в еРецепт ИИ-памятку о лекарстве по официальной инструкции НЦЭЛС, кнопку «Мне плохо» и повтор напоминания через 30 минут.

**Architecture:** Go: пакет `internal/leaflet` (клиент реестра НЦЭЛС + извлечение текста из DOCX), пакет `internal/explain` (Claude API → JSON-памятка, правило высокого риска, кэш в таблице `medicine_explanations`, фоновая догенерация), поле `explanation` в `ItemDTO`, блок на странице врача, `POST /api/sos`. Expo: экран памятки, кнопка «Мне плохо», повтор напоминания.

**Tech Stack:** Go 1.26, `github.com/anthropics/anthropic-sdk-go` (официальный SDK; API брать из `/tmp/claude-1000/bundled-skills/2.1.314/b824be7726e25167d0d2256230f0e390/claude-api/go/claude-api/README.md`, не угадывать), archive/zip + encoding/xml для DOCX; Expo SDK 58.

**Spec:** `docs/superpowers/specs/2026-11-02-ai-explain-sos-design.md`

## Global Constraints

- Весь пользовательский текст на русском, коротко, простыми словами; без эмодзи, градиентов, теней, анимаций.
- Веб: акцент `#0F766E` только для главных кнопок/активного; текст `#1F2937`/`#5B6472`; карточки белые, рамка `#E5E7EB`, радиус 12. Приложение: текст ≥17px, кнопки ≥56px.
- Модель `claude-opus-5-5`; ключ только из env `ANTHROPIC_API_KEY`; без ключа — функция выключена без ошибок. Никогда не логировать ключ и полный текст инструкции.
- Тесты не ходят в сеть: НЦЭЛС и Claude подменяются `httptest.Server` (SDK: `option.WithBaseURL`).
- Коммиты от имени автора, без `Co-Authored-By`, на русском. Не пушить.

## Контракт памятки (JSON, хранится и отдаётся как есть)

```json
{
  "purpose": "Снижает температуру и уменьшает боль.",
  "how_to_take": "По 1 таблетке после еды, запивая водой. Не чаще чем через 4 часа, не больше 4 таблеток в сутки.",
  "do_not_take_if": ["Аллергия на парацетамол", "Тяжёлые болезни печени"],
  "missed_dose": "Выпейте, как вспомните. Если скоро следующий приём — пропустите. Двойную дозу не пейте.",
  "if_stopped_early": "Для этого лекарства можно прекратить, когда симптомы прошли.",
  "urgent_signs": ["Сыпь, отёк лица или затруднённое дыхание — вызовите 103"],
  "high_risk": false,
  "source": {"title": "Инструкция по медицинскому применению (НЦЭЛС)", "reg_number": "РК-ЛС-5№…", "url": "https://register.ndda.kz/…"},
  "model": "claude-opus-5-5",
  "generated_at": "RFC3339"
}
```

`ItemDTO` получает поле `explanation` (объект выше или `null`). Мобильный тип `Item.explanation: Explanation | null`.

`POST /api/sos` (X-Device-Secret) → `{"notified": N}`; push: title «<ФИО> нажал(а) «Мне плохо»», body «Позвоните и узнайте, всё ли в порядке», data `{"type":"sos"}`. Повтор с того же устройства раньше 2 минут → `{"notified": 0, "throttled": true}`.

## Review Focus

1. Инструкция не найдена / реестр недоступен / DOCX битый → памятки нет, сервер работает, повторная попытка при следующем старте, без спама в логах.
2. Модель вернула отказ/обрезанный ответ/невалидный JSON → не сохраняем, логируем кратко.
3. Лекарство высокого риска (варфарин, инсулин…) → `missed_dose` всегда «позвоните врачу», независимо от ответа модели.
4. Старая версия приложения без поля `explanation` и новая без памятки у лекарства — UI не падает, кнопка скрыта.
5. «Мне плохо» без интернета — звонок всё равно открывается; двойное нажатие не шлёт два push.

---

### Task G1: Клиент реестра НЦЭЛС (`internal/leaflet`)

**Files:** Create `internal/leaflet/leaflet.go`, `internal/leaflet/leaflet_test.go`, `internal/leaflet/testdata/sample.docx` (минимальный DOCX, созданный в тесте через archive/zip — можно не хранить файл).

**Interfaces — Produces:**
- `type Client struct{ BaseURL string; HTTP *http.Client }`; `func New() *Client` (BaseURL `https://register.ndda.kz/register/api/v1/PharmaRegister`, timeout 30 s).
- `type Entry struct{ RegisterID int64; TradeName, RegNumber, MNN string }`
- `func (c *Client) Find(ctx, tradeName string) (Entry, error)` — `POST /list` `{"tradeName": name}`; выбор: точное совпадение без «®», пробелов по краям и регистра; иначе первое; пусто → `ErrNotFound`.
- `func (c *Client) Instruction(ctx, registerID int64) (text string, err error)` — `GET /getRegisterInstructionFile?registerId=&lang=ru`, DOCX → текст абзацами (`</w:p>` → перевод строки), обрезка пустых строк; не DOCX/пусто → ошибка.
- `func SourceURL(e Entry) string` — ссылка для людей на запись реестра (`https://register.ndda.kz/` + путь записи; если путь неизвестен — `https://register.ndda.kz/`).

- [ ] Тесты (httptest): `TestFindExactMatch` (ответ с «Нурофен® Форте для детей» и «Нурофен®» → для «Нурофен» выбирается «Нурофен®»), `TestFindNotFound` (пустой массив → ErrNotFound), `TestInstructionDocx` (сервер отдаёт DOCX, собранный в тесте, → текст содержит «Способ применения»), `TestInstructionNotDocx` (HTML → ошибка). FAIL → реализовать → PASS. Commit «Клиент реестра НЦЭЛС: поиск и текст инструкции».

### Task G2: ИИ-памятка (`internal/explain`) + хранение + API + страница врача

**Files:** Create `internal/db/migrations/003_explanations.sql`, `internal/explain/explain.go`, `internal/explain/risk.go`, `internal/explain/explain_test.go`; Modify `internal/api/dto.go` (+`Explanation json.RawMessage` в ItemDTO, `json:"explanation"`, null если нет), загрузка рецепта (LEFT JOIN по `medicine_id`), `cmd/app/main.go` (горутина `explain.New(pool, leaflet.New(), key).BackfillLoop(ctx)` если `ANTHROPIC_API_KEY` не пуст), `internal/web/doctor.go` + `templates/doctor_rx.html` (блок «Коротко о лекарстве» под каждым лекарством, если есть), `.env.example` (`ANTHROPIC_API_KEY=`), `go.mod`.

```sql
CREATE TABLE medicine_explanations (
    medicine_id BIGINT PRIMARY KEY REFERENCES medicines(id) ON DELETE CASCADE,
    data        JSONB NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE medicine_explanation_failures (
    medicine_id BIGINT PRIMARY KEY REFERENCES medicines(id) ON DELETE CASCADE,
    reason      TEXT NOT NULL,
    failed_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**Interfaces — Consumes:** G1 `leaflet.Client`. **Produces:**
- `type Explanation struct{ Purpose, HowToTake string; DoNotTakeIf []string; MissedDose, IfStoppedEarly string; UrgentSigns []string; HighRisk bool; Source Source; Model string; GeneratedAt time.Time }` с json-тегами из контракта.
- `func IsHighRisk(mnn, atc string) bool` — по списку из спеки (МНН: варфарин, аценокумарол, инсулин*, карбамазепин, вальпроевая кислота, ламотриджин, леветирацетам, фенитоин, метотрексат, такролимус, циклоспорин, леводопа, этинилэстрадиол/левоноргестрел/дезогестрел…; АТХ-префиксы B01AA, A10A, N03A, L01, L04, N04B, G03A).
- `type Service struct{ DB *pgxpool.Pool; Leaflet *leaflet.Client; Claude anthropic.Client; Model string }`; `func New(pool, lc, apiKey string, opts ...option.RequestOption) *Service`.
- `func (s *Service) Generate(ctx, medicineName, instructionText string, e leaflet.Entry) (Explanation, error)` — один запрос Messages API, `claude-opus-5-5`, structured output по JSON-схеме контракта (без source/model/generated_at — их ставит код), effort `medium`, `max_tokens` 4000; system-промпт: пересказать ТОЛЬКО по данной инструкции, коротко (каждое поле ≤ 2 предложений, списки ≤ 4 пунктов), простыми словами для пожилого человека, без латыни, чего нет в тексте — «В инструкции не указано, спросите врача или фармацевта», не придумывать дозировки; инструкция передаётся в user-сообщении внутри `<instruction>…</instruction>`. Проверить `stop_reason` (`refusal`/`max_tokens` → ошибка). Если `IsHighRisk` — перезаписать `missed_dose` и `high_risk=true`.
- `func (s *Service) Ensure(ctx, medicineID int64) error` — уже есть → nil; иначе Find → Instruction → Generate → INSERT; ошибка → запись в failures (повтор не раньше чем через 24 ч).
- `func (s *Service) BackfillLoop(ctx)` — при старте и раз в час: все `medicines` без памятки и без свежей неудачи, по одному, пауза 2 с между; лог «памятка готова: <название>» / «памятка не создана: <название>: <причина>».

- [ ] Тесты: `TestIsHighRisk` (варфарин, инсулин гларгин, B01AA03 → true; парацетамол → false); `TestGenerateParsesAndOverridesHighRisk` (фейковый Claude через httptest + `option.WithBaseURL` возвращает JSON → поля заполнены; для варфарина missed_dose заменён); `TestGenerateRefusal` (stop_reason refusal → ошибка); `TestEnsureStoresAndSkips` (dbtest + фейковые НЦЭЛС и Claude: первый вызов пишет строку, второй не ходит в сеть); `TestEnsureNotFoundRecordsFailure`; `TestPrescriptionDTOIncludesExplanation` (api: claim возвращает `explanation` у лекарства с памяткой и `null` у другого); веб: страница рецепта показывает «Коротко о лекарстве» и «Сгенерировано ИИ». FAIL → реализовать → PASS. Commit «ИИ-памятка о лекарстве по инструкции НЦЭЛС».

### Task G3: `POST /api/sos`

**Files:** Create `internal/api/sos.go`, тест в `internal/api/sos_test.go`; Modify `internal/api/api.go` (роут под deviceSecret), миграция `004_sos.sql` (`ALTER TABLE patients ADD COLUMN last_sos_at TIMESTAMPTZ`). Push через тот же код отправки, что в `internal/notifier` (вынести `notifier.SendExpo(ctx, client, url, msgs)` если ещё нет — без дублирования логики).

- [ ] Тесты: `TestSOSNotifiesRelatives` (2 родственника с токенами → 2 сообщения в фейковый Expo, текст с ФИО), `TestSOSThrottle` (второй вызов через 10 с → throttled, 0 push), `TestSOSNoRelatives` (→ notified 0, 200). Commit «Кнопка «Мне плохо»: push родственникам».

### Task M1: Expo — памятка, «Мне плохо», повтор напоминания (отдельный worktree, параллельно G1–G3)

**Files:** Modify `mobile/lib/types.ts` (`Explanation`, `Item.explanation?: Explanation | null`), `mobile/lib/api.ts` (`api.sos()` → `{notified, throttled?}`), `mobile/lib/schedule.ts` (повтор), `mobile/lib/__tests__/schedule.test.ts`; Create `mobile/app/explain.tsx` (модальный экран по `itemId`), `mobile/components/SosButton.tsx`; Modify `mobile/app/(tabs)/index.tsx` (кнопка «Коротко о лекарстве» у карточки, если есть explanation; `SosButton` вверху экрана), `mobile/app/(tabs)/course.tsx` (та же ссылка у лекарства).

- Экран памятки: заголовок — название; блоки с подзаголовками «Зачем», «Как принимать», «Кому нельзя» (список), «Если пропустили приём» (при `high_risk` — плашка с рамкой и кнопкой «Позвонить врачу» не нужна, только текст), «Если бросить раньше», «Срочно к врачу» (список); внизу мелко серым «Сгенерировано ИИ по инструкции НЦЭЛС, рег. № … Не заменяет совет врача». Текст ≥17px.
- SosButton: вторичная кнопка во всю ширину с красной рамкой `#B42318` и текстом «Мне плохо» (без заливки); по нажатию `Alert`: «Позвонить в скорую 103?» → [«Отмена», «Позвонить»]; «Позвонить» → `Linking.openURL("tel:103")` и параллельно `api.sos()` (ошибку сети игнорировать); под кнопкой мелко «112 — единая служба · 9103 — скорая в Алматы».
- Повтор: `upcomingNotifications` для каждой неотмеченной дозы добавляет второе уведомление в `at + 30 мин` с ключом `dose-<id>-repeat`, `kind: "dose"`, `doseId`, title тот же, body «Напоминание: » + исходный body; только если `at + 30 мин > now`; общий лимит 60 сохраняется (сортировка по времени, потом срез).

- [ ] Тесты jest: повтор создаётся на +30 мин и пропадает для отмеченной дозы; прошедшая основная, но будущая повторная — остаётся только повтор; лимит 60. `npm test`, `npx tsc --noEmit`, `npx expo export --platform android` (удалить вывод). Commit «Expo: памятка о лекарстве, кнопка «Мне плохо», повтор напоминания».

### Финал

- [ ] Слить ветки в main, `go test ./...`, `go vet`, `npm test`, `tsc`.
- [ ] Финальный обзор (opus), одна волна правок.
- [ ] Render: переменная `ANTHROPIC_API_KEY` (через API, не печатая), push, дождаться live, проверить: в логах «памятка готова», `claim` отдаёт `explanation`, страница врача показывает блок, `POST /api/sos` → 200.
- [ ] README: раздел «ИИ-памятка» и «Мне плохо».
