import { api } from "./api";
import { getPendingMarks, loadCourses, saveCourses, setPendingMarks } from "./storage";

// Простой мьютекс: чтение-изменение-запись хранилища выполняется строго по очереди,
// иначе параллельные вызовы теряют отметки.
let chain: Promise<unknown> = Promise.resolve();
function locked<T>(fn: () => Promise<T>): Promise<T> {
  const run = chain.then(fn, fn);
  chain = run.catch(() => undefined);
  return run;
}

// Отметка «Выпил»: сразу пишем в локальные курсы и ставим в очередь отправки.
export function markTaken(doseId: number, at: Date): Promise<void> {
  return locked(() => markTakenUnlocked(doseId, at));
}

async function markTakenUnlocked(doseId: number, at: Date): Promise<void> {
  const iso = at.toISOString();
  const courses = await loadCourses();
  for (const c of courses) {
    for (const d of c.doses) {
      if (d.id === doseId && !d.taken_at) d.taken_at = iso;
    }
  }
  await saveCourses(courses);
  const pending = await getPendingMarks();
  if (!pending.some((m) => m.dose_id === doseId)) {
    await setPendingMarks([...pending, { dose_id: doseId, taken_at: iso }]);
  }
}

// Отправляет очередь. Возвращает число отправленных отметок; при ошибке сети очередь остаётся.
export async function flushMarks(): Promise<number> {
  const sent = await locked(getPendingMarks);
  if (sent.length === 0) return 0;
  try {
    await api.sync(sent);
  } catch (e) {
    console.warn("sync: не удалось отправить отметки", e);
    return 0;
  }
  // за время отправки могли добавиться новые отметки — убираем только отправленные
  const sentIds = new Set(sent.map((m) => m.dose_id));
  await locked(async () => {
    const rest = (await getPendingMarks()).filter((m) => !sentIds.has(m.dose_id));
    await setPendingMarks(rest);
  });
  return sent.length;
}
