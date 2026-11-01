// Операции над сохранёнными курсами для экранов: привязка рецепта и обновление с сервера.
import { api } from "./api";
import { mergeClaimed, mergeCourses } from "./courses";
import { rescheduleAll } from "./notifications";
import { getPendingMarks, loadCourses, saveCourses } from "./storage";
import { flushMarks } from "./sync";
import type { Course } from "./types";

export async function claimAndStore(code: string): Promise<Course> {
  const claimed = await api.claim(code);
  const merged = mergeClaimed(await loadCourses(), claimed, await getPendingMarks());
  await saveCourses(merged);
  try {
    await rescheduleAll();
  } catch (e) {
    console.warn("courses: не удалось обновить напоминания", e);
  }
  return merged.find((c) => c.prescription.id === claimed.prescription.id)!;
}

// Отправляет очередь, затем подтягивает курсы. Без сети тихо оставляет локальные.
export async function refreshCourses(): Promise<Course[]> {
  await flushMarks();
  try {
    const remote = await api.myPrescriptions();
    const merged = mergeCourses(await loadCourses(), remote, await getPendingMarks());
    await saveCourses(merged);
    await rescheduleAll();
    return merged;
  } catch (e) {
    console.warn("courses: не удалось обновить, показываем сохранённые", e);
    return loadCourses();
  }
}
