import { mergeClaimed, mergeCourses } from "../courses";
import type { Course, Prescription } from "../types";

const rx = (id: number) => ({ id, items: [] }) as unknown as Prescription;
const course = (id: number, taken: Record<number, string | null>): Course => ({
  prescription: rx(id),
  doses: Object.entries(taken).map(([k, v]) => ({
    id: Number(k), item_id: 1, scheduled_at: "2026-11-01T08:00:00+05:00",
    day_number: 1, total_days: 3, taken_at: v,
  })),
});

describe("mergeCourses", () => {
  it("берёт данные сервера и сохраняет отметки из очереди", () => {
    const local = [course(1, { 10: "2026-11-01T03:00:00Z", 11: null })];
    const remote = [course(1, { 10: null, 11: null }), course(2, { 20: null })];
    const out = mergeCourses(local, remote, [{ dose_id: 10, taken_at: "2026-11-01T03:00:00Z" }]);
    expect(out).toHaveLength(2);
    expect(out[0].doses[0].taken_at).toBe("2026-11-01T03:00:00Z");
    expect(out[0].doses[1].taken_at).toBeNull();
  });
  it("не затирает отметку сервера", () => {
    const out = mergeCourses([], [course(1, { 10: "S" })], [{ dose_id: 10, taken_at: "L" }]);
    expect(out[0].doses[0].taken_at).toBe("S");
  });
  it("пустой ответ сервера не стирает локальный кэш", () => {
    const local = [course(1, { 10: null })];
    expect(mergeCourses(local, [], [])).toEqual(local);
  });
});

describe("mergeClaimed", () => {
  it("заменяет курс с тем же id, остальные оставляет", () => {
    const local = [course(1, { 10: null }), course(2, { 20: null })];
    const out = mergeClaimed(local, course(2, { 20: null, 21: null }), []);
    expect(out.map((c) => c.prescription.id)).toEqual([1, 2]);
    expect(out[1].doses).toHaveLength(2);
  });
  it("новый курс ставит первым и применяет очередь", () => {
    const out = mergeClaimed([course(1, { 10: null })], course(3, { 30: null }), [
      { dose_id: 30, taken_at: "X" },
    ]);
    expect(out.map((c) => c.prescription.id)).toEqual([3, 1]);
    expect(out[0].doses[0].taken_at).toBe("X");
  });
});
