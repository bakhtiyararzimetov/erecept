// Чистая логика расписания. Без expo-модулей, чтобы тестироваться в jest.
// Функции, которым нужен «календарный день», используют ЛОКАЛЬНОЕ время устройства
// (на телефонах пациентов это Asia/Almaty).
import type { Course, Dose, Item, Prescription } from "./types";
import { t, tf } from "./i18n";

export type PlannedNotification = {
  key: string;
  at: Date;
  title: string;
  body: string;
  kind: "dose" | "water";
  doseId?: number;
};

export type DoseStatus = "taken" | "missed" | "due" | "upcoming";
export type TodayEntry = { dose: Dose; item: Item; status: DoseStatus };

const MIN = 60_000;
const MISSED_AFTER_MIN = 60;
const DUE_BEFORE_MIN = 30;
const REPEAT_MIN = 30;

export function extractCode(scanned: string): string | null {
  const s = scanned.trim();
  const url = s.match(/\/r\/([a-z0-9]{6,32})\/?(?:[?#].*)?$/i);
  if (url) return url[1].toLowerCase();
  if (/^[a-z0-9]{6,32}$/i.test(s)) return s.toLowerCase();
  return null;
}

export function reminderText(item: Item, dose: Dose): { title: string; body: string } {
  return {
    title: item.medicine_name,
    body: `${item.dosage}, ${item.meal_label} · ${tf("день {0} из {1}", dose.day_number, dose.total_days)}`,
  };
}

function startOfDay(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate());
}

function parseDateLocal(s: string): Date {
  const [y, m, d] = s.split("-").map(Number);
  return new Date(y, m - 1, d);
}

function atTime(day: Date, hhmm: string): Date {
  const [h, m] = hhmm.split(":").map(Number);
  return new Date(day.getFullYear(), day.getMonth(), day.getDate(), h, m);
}

function sameDay(a: Date, b: Date): boolean {
  return startOfDay(a).getTime() === startOfDay(b).getTime();
}

// Первый и последний день курса (локальные полночи) или null, если дата начала не задана.
function courseDays(p: Prescription): { first: Date; last: Date } | null {
  if (!p.start_date) return null;
  const first = parseDateLocal(p.start_date);
  const maxDays = Math.max(0, ...p.items.map((i) => i.days));
  if (maxDays === 0) return null;
  const last = new Date(first.getFullYear(), first.getMonth(), first.getDate() + maxDays - 1);
  return { first, last };
}

export function waterTimes(p: Prescription, day: Date): Date[] {
  if (!p.water_every_hours || p.water_every_hours <= 0) return [];
  const range = courseDays(p);
  if (!range) return [];
  const d = startOfDay(day);
  if (d < range.first || d > range.last) return [];
  const from = atTime(d, p.water_from);
  const to = atTime(d, p.water_to);
  const out: Date[] = [];
  for (let t = from.getTime(); t <= to.getTime(); t += p.water_every_hours * 60 * MIN) {
    out.push(new Date(t));
  }
  return out;
}

function itemOf(course: Course, dose: Dose): Item | undefined {
  return course.prescription.items.find((i) => i.id === dose.item_id);
}

export function upcomingNotifications(courses: Course[], now: Date, limit = 60): PlannedNotification[] {
  const out: PlannedNotification[] = [];
  for (const c of courses) {
    for (const dose of c.doses) {
      if (dose.taken_at) continue;
      const at = new Date(dose.scheduled_at);
      const repeatAt = new Date(at.getTime() + REPEAT_MIN * MIN);
      if (repeatAt <= now) continue;
      const item = itemOf(c, dose);
      if (!item) continue;
      const text = reminderText(item, dose);
      if (at > now) out.push({ key: `dose-${dose.id}`, at, ...text, kind: "dose", doseId: dose.id });
      out.push({
        key: `dose-${dose.id}-repeat`,
        at: repeatAt,
        title: text.title,
        body: t("Напоминание: ") + text.body,
        kind: "dose",
        doseId: dose.id,
      });
    }
    const range = courseDays(c.prescription);
    if (range && c.prescription.water_every_hours > 0) {
      const from = startOfDay(now) > range.first ? startOfDay(now) : range.first;
      for (let d = from; d <= range.last; d = new Date(d.getFullYear(), d.getMonth(), d.getDate() + 1)) {
        for (const at of waterTimes(c.prescription, d)) {
          if (at <= now) continue;
          out.push({
            key: `water-${c.prescription.id}-${at.getTime()}`,
            at,
            title: t("Время выпить воды"),
            body: t("Стакан воды"),
            kind: "water",
          });
        }
      }
    }
  }
  out.sort((a, b) => a.at.getTime() - b.at.getTime());
  return out.slice(0, limit);
}

// Отметка раньше окна «Пора принять» — переспрашиваем пациента (защита от случайного тапа).
export function isEarly(dose: Dose, now: Date): boolean {
  return doseStatus(dose, now) === "upcoming";
}

export function doseStatus(dose: Dose, now: Date): DoseStatus {
  if (dose.taken_at) return "taken";
  const diffMin = (now.getTime() - new Date(dose.scheduled_at).getTime()) / MIN;
  if (diffMin >= MISSED_AFTER_MIN) return "missed";
  if (diffMin >= -DUE_BEFORE_MIN) return "due";
  return "upcoming";
}

export function todayList(courses: Course[], now: Date): TodayEntry[] {
  const out: TodayEntry[] = [];
  for (const c of courses) {
    for (const dose of c.doses) {
      if (!sameDay(new Date(dose.scheduled_at), now)) continue;
      const item = itemOf(c, dose);
      if (!item) continue;
      out.push({ dose, item, status: doseStatus(dose, now) });
    }
  }
  out.sort((a, b) => new Date(a.dose.scheduled_at).getTime() - new Date(b.dose.scheduled_at).getTime());
  return out;
}

// percent = floor(принято*100 / (принято + пропущено)), 100 если приёмов ещё не было — как на сервере.
export function courseProgress(
  course: Course,
  now: Date,
): { taken: number; missed: number; total: number; percent: number } {
  let taken = 0;
  let missed = 0;
  for (const d of course.doses) {
    if (d.taken_at) taken++;
    else if (doseStatus(d, now) === "missed") missed++;
  }
  const due = taken + missed;
  return {
    taken,
    missed,
    total: course.doses.length,
    percent: due === 0 ? 100 : Math.floor((taken * 100) / due),
  };
}

export const SNOOZE_MIN = 15;

export function snoozeTime(now: Date): Date {
  return new Date(now.getTime() + SNOOZE_MIN * MIN);
}

// Содержимое локального уведомления: у доз есть категория с кнопками и doseId, у воды — нет.
export function notificationContent(p: PlannedNotification): {
  title: string;
  body: string;
  categoryIdentifier?: string;
  data: { doseId?: number };
} {
  if (p.kind === "dose" && p.doseId !== undefined) {
    return { title: p.title, body: p.body, categoryIdentifier: "dose", data: { doseId: p.doseId } };
  }
  return { title: p.title, body: p.body, data: {} };
}

// Ключ для отсева дублей ответа на уведомление: слушатель и getLastNotificationResponseAsync
// отдают один и тот же ответ, а повторная отложка того же уведомления (другая дата доставки) — новый.
export function responseKey(requestId: string, actionId: string, deliveredAt: number): string {
  return `${requestId}|${actionId}|${deliveredAt}`;
}
