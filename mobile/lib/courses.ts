// Чистые функции слияния курсов (без expo-модулей, тестируются в jest).
import type { Course, Mark } from "./types";

function withPending(course: Course, pending: Mark[]): Course {
  const byDose = new Map(pending.map((m) => [m.dose_id, m.taken_at]));
  return {
    prescription: course.prescription,
    doses: course.doses.map((d) =>
      !d.taken_at && byDose.has(d.id) ? { ...d, taken_at: byDose.get(d.id)! } : d,
    ),
  };
}

// Список с сервера — источник правды; отметки из очереди отправки (сервер мог их ещё не получить)
// сохраняются. local нужен, только если сервер вернул пустой ответ из-за сбоя: тогда не затираем кэш.
export function mergeCourses(local: Course[], remote: Course[], pending: Mark[]): Course[] {
  if (remote.length === 0 && local.length > 0) return local.map((c) => withPending(c, pending));
  return remote.map((c) => withPending(c, pending));
}

// Рецепт, который только что привязали: заменяет курс с тем же id, остальные не трогаем.
export function mergeClaimed(local: Course[], claimed: Course, pending: Mark[]): Course[] {
  const fresh = withPending(claimed, pending);
  const found = local.some((c) => c.prescription.id === fresh.prescription.id);
  if (!found) return [fresh, ...local];
  return local.map((c) => (c.prescription.id === fresh.prescription.id ? fresh : c));
}
