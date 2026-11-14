// Форматирование цен и расстояний для экранов. Без зависимостей от expo.
const NNBSP = " "; // узкий неразрывный пробел

export function formatPrice(value: number): string {
  const n = Math.round(value);
  const sign = n < 0 ? "-" : "";
  const digits = String(Math.abs(n)).replace(/\B(?=(\d{3})+(?!\d))/g, NNBSP);
  return `${sign}${digits} ₸`;
}

export function formatKm(km: number): string {
  return `${(Math.round(km * 10) / 10).toFixed(1).replace(".", ",")} км`;
}

// openStatus: открыта ли аптека сейчас по строке часов работы («08:00–22:00», «Круглосуточно»).
// Смена через полночь («20:00–02:00») учитывается; непонятный формат — null (статус не показываем).
export function openStatus(hours: string, now: Date): "open" | "closed" | null {
  const h = hours.trim().toLowerCase();
  if (h === "круглосуточно" || h === "24 hours" || h === "24/7") return "open";
  const m = h.match(/^(\d{1,2}):(\d{2})\s*[–—-]\s*(\d{1,2}):(\d{2})$/);
  if (!m) return null;
  const from = Number(m[1]) * 60 + Number(m[2]);
  const to = Number(m[3]) * 60 + Number(m[4]);
  const cur = now.getHours() * 60 + now.getMinutes();
  const open = from <= to ? cur >= from && cur < to : cur >= from || cur < to;
  return open ? "open" : "closed";
}
