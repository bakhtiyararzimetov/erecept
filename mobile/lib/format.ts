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
