import {
  courseProgress,
  extractCode,
  reminderText,
  todayList,
  upcomingNotifications,
  waterTimes,
} from "../schedule";
import type { Course, Dose, Item, Prescription } from "../types";

const item = (over: Partial<Item> = {}): Item => ({
  id: 1,
  medicine_name: "Парацетамол",
  medicine_id: null,
  dosage: "1 таблетка 500 мг",
  times_per_day: 1,
  days: 7,
  meal: "after",
  meal_label: "после еды",
  times: ["08:00"],
  photo_url: "",
  ...over,
});

const presc = (over: Partial<Prescription> = {}): Prescription => ({
  id: 1,
  code: "abc123",
  patient_name: "Иванов Иван",
  patient_birth: "1950-01-01",
  doctor_name: "Ахметова Д.С.",
  comment: "",
  water_every_hours: 0,
  water_from: "09:00",
  water_to: "21:00",
  start_date: "2026-11-01",
  items: [item()],
  ...over,
});

const dose = (id: number, at: string, over: Partial<Dose> = {}): Dose => ({
  id,
  item_id: 1,
  scheduled_at: at,
  day_number: 1,
  total_days: 7,
  taken_at: null,
  ...over,
});

const course = (doses: Dose[], p: Prescription = presc()): Course => ({
  prescription: p,
  doses,
});

describe("extractCode", () => {
  it("берёт код из ссылки", () => {
    expect(extractCode("https://erecept.onrender.com/r/abc123xyz")).toBe("abc123xyz");
  });
  it("ссылка со слешем в конце", () => {
    expect(extractCode("https://erecept.onrender.com/r/abc123xyz/")).toBe("abc123xyz");
  });
  it("голый код", () => {
    expect(extractCode("  abc123xyz ")).toBe("abc123xyz");
  });
  it("мусор даёт null", () => {
    expect(extractCode("привет мир")).toBeNull();
    expect(extractCode("")).toBeNull();
    expect(extractCode("abc")).toBeNull();
    expect(extractCode("https://example.com/other")).toBeNull();
  });
});

describe("reminderText", () => {
  it("формирует заголовок и тело", () => {
    const d = dose(1, "2026-11-03T08:00:00+05:00", { day_number: 3, total_days: 7 });
    expect(reminderText(item(), d)).toEqual({
      title: "Парацетамол",
      body: "1 таблетка 500 мг, после еды · день 3 из 7",
    });
  });
});

describe("waterTimes", () => {
  const day = new Date("2026-11-01T12:00:00+05:00");
  it("каждые 2 часа с 09:00 до 21:00 — 7 времён", () => {
    const t = waterTimes(presc({ water_every_hours: 2 }), day);
    expect(t).toHaveLength(7);
    expect(t[0].toISOString()).toBe(new Date("2026-11-01T09:00:00+05:00").toISOString());
    expect(t[6].toISOString()).toBe(new Date("2026-11-01T21:00:00+05:00").toISOString());
  });
  it("0 — пусто", () => {
    expect(waterTimes(presc({ water_every_hours: 0 }), day)).toEqual([]);
  });
  it("вне дней курса — пусто", () => {
    const p = presc({ water_every_hours: 2 });
    expect(waterTimes(p, new Date("2026-11-08T12:00:00+05:00"))).toEqual([]);
    expect(waterTimes(p, new Date("2026-10-31T12:00:00+05:00"))).toEqual([]);
    expect(waterTimes(p, new Date("2026-11-07T12:00:00+05:00"))).toHaveLength(7);
  });
});

describe("upcomingNotifications", () => {
  const now = new Date("2026-11-01T10:00:00+05:00");
  it("пропускает прошедшие и отмеченные, сортирует", () => {
    const c = course([
      dose(3, "2026-11-02T08:00:00+05:00"),
      dose(1, "2026-11-01T08:00:00+05:00"),
      dose(2, "2026-11-01T20:00:00+05:00"),
      dose(4, "2026-11-01T21:00:00+05:00", { taken_at: "2026-11-01T09:00:00+05:00" }),
    ]);
    const r = upcomingNotifications([c], now);
    expect(r.filter((n) => !n.key.endsWith("-repeat")).map((n) => n.doseId)).toEqual([2, 3]);
    expect(r[0]).toMatchObject({ kind: "dose", title: "Парацетамол" });
    expect(r[0].body).toBe("1 таблетка 500 мг, после еды · день 1 из 7");
  });
  it("добавляет воду", () => {
    const c = course([], presc({ water_every_hours: 6 })); // 09:00, 15:00, 21:00
    const r = upcomingNotifications([c], now);
    expect(r.filter((n) => n.kind === "water").length).toBeGreaterThanOrEqual(2);
    expect(r[0]).toMatchObject({ title: "Время выпить воды", body: "Стакан воды", kind: "water" });
    expect(r.every((n) => n.at > now)).toBe(true);
  });
  it("лимит 60", () => {
    const doses = Array.from({ length: 100 }, (_, i) =>
      dose(i + 1, new Date(now.getTime() + (i + 1) * 3600_000).toISOString()),
    );
    const r = upcomingNotifications([course(doses)], now);
    expect(r).toHaveLength(60);
    expect(r[0].doseId).toBe(1);
    expect(r[1].key).toBe("dose-1-repeat");
    expect(upcomingNotifications([course(doses)], now, 5)).toHaveLength(5);
  });
});

describe("повтор напоминания через 30 минут", () => {
  const now = new Date("2026-11-01T10:00:00+05:00");
  it("создаётся на +30 мин для неотмеченной дозы", () => {
    const r = upcomingNotifications([course([dose(7, "2026-11-01T20:00:00+05:00")])], now);
    expect(r.map((n) => n.key)).toEqual(["dose-7", "dose-7-repeat"]);
    expect(r[1].at.getTime()).toBe(new Date("2026-11-01T20:30:00+05:00").getTime());
    expect(r[1]).toMatchObject({ kind: "dose", doseId: 7, title: "Парацетамол" });
    expect(r[1].body).toBe("Напоминание: " + r[0].body);
  });
  it("нет повтора у отмеченной дозы", () => {
    const r = upcomingNotifications(
      [course([dose(7, "2026-11-01T20:00:00+05:00", { taken_at: "2026-11-01T09:00:00+05:00" })])],
      now,
    );
    expect(r).toEqual([]);
  });
  it("основное прошло, повтор в будущем: остаётся только повтор", () => {
    const r = upcomingNotifications([course([dose(7, "2026-11-01T09:45:00+05:00")])], now);
    expect(r.map((n) => n.key)).toEqual(["dose-7-repeat"]);
  });
  it("оба прошли: пусто", () => {
    expect(upcomingNotifications([course([dose(7, "2026-11-01T09:00:00+05:00")])], now)).toEqual([]);
  });
});

describe("todayList", () => {
  const now = new Date("2026-11-01T12:00:00+05:00");
  it("статусы", () => {
    const doses = [
      dose(1, "2026-11-01T08:00:00+05:00", { taken_at: "2026-11-01T08:05:00+05:00" }),
      dose(2, "2026-11-01T10:00:00+05:00"), // прошло 120 мин
      dose(3, "2026-11-01T11:30:00+05:00"), // +30 мин: due
      dose(4, "2026-11-01T12:20:00+05:00"), // через 20 мин: due
      dose(5, "2026-11-01T20:00:00+05:00"), // upcoming
      dose(6, "2026-11-02T08:00:00+05:00"), // не сегодня
      dose(7, "2026-11-01T11:00:00+05:00"), // ровно 60 мин: missed
    ];
    const r = todayList([course(doses)], now);
    expect(r.map((e) => [e.dose.id, e.status])).toEqual([
      [1, "taken"],
      [2, "missed"],
      [7, "missed"],
      [3, "due"],
      [4, "due"],
      [5, "upcoming"],
    ]);
    expect(r[0].item.medicine_name).toBe("Парацетамол");
  });
});

describe("courseProgress", () => {
  it("3 принято, 1 пропущено — 75%", () => {
    const now = new Date("2026-11-01T12:00:00+05:00");
    const doses = [
      dose(1, "2026-10-30T08:00:00+05:00", { taken_at: "2026-10-30T08:01:00+05:00" }),
      dose(2, "2026-10-30T20:00:00+05:00", { taken_at: "2026-10-30T20:01:00+05:00" }),
      dose(3, "2026-10-31T08:00:00+05:00", { taken_at: "2026-10-31T08:01:00+05:00" }),
      dose(4, "2026-10-31T20:00:00+05:00"),
      dose(5, "2026-11-02T08:00:00+05:00"),
    ];
    expect(courseProgress(course(doses), now)).toEqual({ taken: 3, missed: 1, total: 5, percent: 75 });
  });
  it("нет прошедших доз — 100", () => {
    const now = new Date("2026-10-01T12:00:00+05:00");
    expect(courseProgress(course([dose(1, "2026-11-02T08:00:00+05:00")]), now).percent).toBe(100);
  });
  it("2 принято, 1 пропущено — 66", () => {
    const now = new Date("2026-11-01T12:00:00+05:00");
    const doses = [
      dose(1, "2026-10-30T08:00:00+05:00", { taken_at: "2026-10-30T08:01:00+05:00" }),
      dose(2, "2026-10-30T20:00:00+05:00", { taken_at: "2026-10-30T20:01:00+05:00" }),
      dose(3, "2026-10-31T08:00:00+05:00"),
    ];
    expect(courseProgress(course(doses), now).percent).toBe(66);
  });
});

import { notificationContent, snoozeTime } from "../schedule";

describe("snoozeTime / notificationContent", () => {
  test("snoozeTime = now + 15 минут", () => {
    const now = new Date("2026-11-01T10:00:00+05:00");
    expect(snoozeTime(now).getTime() - now.getTime()).toBe(15 * 60_000);
  });
  test("доза получает категорию и doseId, вода — нет", () => {
    const at = new Date();
    const dose = notificationContent({ key: "dose-7", at, title: "Т", body: "Б", kind: "dose", doseId: 7 });
    expect(dose).toEqual({ title: "Т", body: "Б", categoryIdentifier: "dose", data: { doseId: 7 } });
    const water = notificationContent({ key: "w", at, title: "Время выпить воды", body: "Стакан воды", kind: "water" });
    expect(water.categoryIdentifier).toBeUndefined();
    expect(water.data).toEqual({});
  });
});

import { responseKey } from "../schedule";

test("responseKey различает повторную доставку того же уведомления", () => {
  expect(responseKey("snooze-1", "SNOOZE", 100)).toBe(responseKey("snooze-1", "SNOOZE", 100));
  expect(responseKey("snooze-1", "SNOOZE", 100)).not.toBe(responseKey("snooze-1", "SNOOZE", 200));
});
