jest.mock("../api", () => ({ api: { sync: jest.fn() } }));
jest.mock("../storage", () => {
  let courses: any[] = [];
  let pending: any[] = [];
  return {
    loadCourses: jest.fn(async () => courses),
    saveCourses: jest.fn(async (c: any[]) => {
      courses = c;
    }),
    getPendingMarks: jest.fn(async () => pending),
    setPendingMarks: jest.fn(async (m: any[]) => {
      pending = m;
    }),
  };
});

import { api } from "../api";
import * as storage from "../storage";
import { flushMarks, markTaken } from "../sync";

const syncMock = api.sync as jest.Mock;

beforeEach(async () => {
  syncMock.mockReset();
  await storage.setPendingMarks([]);
  await storage.saveCourses([
    {
      prescription: {} as any,
      doses: [
        { id: 1, taken_at: null },
        { id: 2, taken_at: null },
      ] as any,
    },
  ]);
});

describe("markTaken / flushMarks", () => {
  it("markTaken пишет локально и в очередь", async () => {
    await markTaken(1, new Date("2026-11-01T08:00:00+05:00"));
    const courses = await storage.loadCourses();
    expect(courses[0].doses[0].taken_at).toBe(new Date("2026-11-01T08:00:00+05:00").toISOString());
    expect(await storage.getPendingMarks()).toHaveLength(1);
  });

  it("повторный markTaken той же дозы не дублирует очередь", async () => {
    await markTaken(1, new Date("2026-11-01T08:00:00+05:00"));
    await markTaken(1, new Date("2026-11-01T08:05:00+05:00"));
    expect(await storage.getPendingMarks()).toHaveLength(1);
  });

  it("успех очищает очередь", async () => {
    syncMock.mockResolvedValue({ updated: 2 });
    await markTaken(1, new Date());
    await markTaken(2, new Date());
    expect(await flushMarks()).toBe(2);
    expect(syncMock).toHaveBeenCalledTimes(1);
    expect(await storage.getPendingMarks()).toEqual([]);
  });

  it("ошибка сети оставляет очередь", async () => {
    syncMock.mockRejectedValue(new Error("Нет связи с сервером"));
    await markTaken(1, new Date());
    expect(await flushMarks()).toBe(0);
    expect(await storage.getPendingMarks()).toHaveLength(1);
  });

  it("пустая очередь не ходит в сеть", async () => {
    expect(await flushMarks()).toBe(0);
    expect(syncMock).not.toHaveBeenCalled();
  });

  it("отметка, добавленная во время отправки, не теряется", async () => {
    await markTaken(1, new Date());
    syncMock.mockImplementation(async () => {
      await markTaken(2, new Date());
      return { updated: 1 };
    });
    expect(await flushMarks()).toBe(1);
    const left = await storage.getPendingMarks();
    expect(left.map((m: any) => m.dose_id)).toEqual([2]);
  });
});
