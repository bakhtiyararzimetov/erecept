// Язык приложения: русский по умолчанию, английский — по выбору в «Настройках»
// или если язык телефона английский. Ключ перевода — сама русская строка.
export type Lang = "ru" | "en";

// Хранилище подключается лениво: модуль перевода используют и чистые функции (lib/schedule) в тестах.
function storage() {
  return require("@react-native-async-storage/async-storage").default as typeof import("@react-native-async-storage/async-storage").default;
}

const KEY = "erecept.lang";
let lang: Lang = "ru";
const listeners = new Set<(l: Lang) => void>();

export function getLang(): Lang {
  return lang;
}

export async function initLang(): Promise<Lang> {
  try {
    const saved = await storage().getItem(KEY);
    if (saved === "ru" || saved === "en") {
      lang = saved;
      return lang;
    }
  } catch {
    // нет хранилища — язык по телефону
  }
  const device = Intl.DateTimeFormat().resolvedOptions().locale ?? "";
  lang = device.toLowerCase().startsWith("en") ? "en" : "ru";
  return lang;
}

export async function setLang(l: Lang): Promise<void> {
  lang = l;
  await storage().setItem(KEY, l).catch(() => undefined);
  listeners.forEach((f) => f(l));
}

export function onLangChange(f: (l: Lang) => void): () => void {
  listeners.add(f);
  return () => listeners.delete(f);
}

export function t(ru: string): string {
  return lang === "en" ? (en[ru] ?? ru) : ru;
}

// tf: «день {0} из {1}» — подстановка по номерам после перевода.
export function tf(ru: string, ...args: (string | number)[]): string {
  return t(ru).replace(/\{(\d+)\}/g, (_, i) => String(args[Number(i)] ?? ""));
}

export function locale(): string {
  return lang === "en" ? "en-GB" : "ru-RU";
}

export const en: Record<string, string> = {
  "Язык": "Language",
  " Разрешите доступ в настройках телефона или введите код вручную ниже.": " Allow access in the phone settings or enter the code below.",
  ", рег. №": ", reg. no.",
  "112 — единая служба · 9103 — скорая в Алматы": "112 — emergency line · 9103 — ambulance in Almaty",
  "Push недоступен: нужна сборка с EAS (см. README)": "Push is not available: an EAS build is needed (see README)",
  "Аналоги": "Analogs",
  "Аптеки": "Pharmacies",
  "Близкие": "Family",
  "Ваше имя": "Your name",
  "Введите ваше имя": "Enter your name",
  "Введите код из 6 цифр": "Enter the 6-digit code",
  "Включить уведомления": "Turn on notifications",
  "Время выпить воды": "Time to drink water",
  "Всего: выпито {0}, пропущено {1}": "Total: taken {0}, missed {1}",
  "Выберите лекарство или введите название": "Choose a medicine or type its name",
  "Выйти из режима родственника": "Leave relative mode",
  "Выпил": "Taken",
  "Выпито": "Taken",
  "Где купить": "Where to buy",
  "Дата начала не задана": "Start date not set",
  "Действует 24 часа": "Valid for 24 hours",
  "Для сортировки по расстоянию приложению нужна ваша геопозиция. Она нигде не сохраняется": "To sort by distance, the app needs your location. It is not stored anywhere",
  "Для этого лекарства краткой памятки пока нет": "There is no short summary for this medicine yet",
  "Если бросить раньше": "If you stop early",
  "Если пропустили приём": "If you miss a dose",
  "Загружаем рецепт…": "Loading the prescription…",
  "Загрузить": "Load",
  "Загрузка...": "Loading...",
  "Загрузка…": "Loading…",
  "Закрыть": "Close",
  "Зачем": "What it's for",
  "Из галереи": "From gallery",
  "Как принимать": "How to take",
  "Камера нужна, чтобы отсканировать QR-код с рецепта врача": "The camera is needed to scan the QR code from your doctor's prescription",
  "Код": "Code",
  "Код для приложения еРецепт:": "Code for the eRecept app:",
  "Код из приложения пациента": "Code from the patient's app",
  "Код рецепта": "Prescription code",
  "Комментарий врача": "Doctor's comment",
  "Кому нельзя": "Do not take if",
  "Коротко о лекарстве": "About this medicine",
  "Курс": "Course",
  "Курсов пока нет. Отсканируйте рецепт на экране «Сегодня»": "No courses yet. Scan a prescription on the “Today” screen",
  "Мне плохо": "I feel unwell",
  "На карте": "On the map",
  "На сегодня приёмов нет": "No doses today",
  "Наберите 103 вручную": "Dial 103 manually",
  "Наведите камеру на QR-код рецепта": "Point the camera at the prescription QR code",
  "Назад": "Back",
  "Название лекарства": "Medicine name",
  "Найти": "Search",
  "Напоминание: ": "Reminder: ",
  "Напоминания": "Reminders",
  "Настройки": "Settings",
  "Не заменяет совет врача": "It does not replace your doctor's advice",
  "Не работает камера? Введите код рецепта": "Camera not working? Enter the prescription code",
  "Не удалось загрузить данные": "Could not load data",
  "Не удалось загрузить рецепт": "Could not load the prescription",
  "Не удалось загрузить фото. Попробуйте ещё раз": "Could not upload the photo. Please try again",
  "Не удалось подключиться": "Could not connect",
  "Не удалось позвонить": "Could not call",
  "Не удалось создать код": "Could not create a code",
  "Не удалось сохранить отметку. Попробуйте ещё раз": "Could not save. Please try again",
  "Нет доступа к камере. Разрешите его в настройках телефона": "No camera access. Allow it in the phone settings",
  "Нет интернета. Отметки сохранятся и отправятся позже": "No internet. Your marks are saved and will be sent later",
  "Нет связи с сервером": "No connection to the server",
  "Обновить": "Refresh",
  "Обновление...": "Refreshing...",
  "Осталось: {0}": "Left: {0}",
  "Открыть настройки": "Open settings",
  "Отмена": "Cancel",
  "Отмечено: {0}": "Taken: {0}",
  "Отсканируйте QR-код с рецепта врача": "Scan the QR code from your doctor's prescription",
  "Памятки нет": "No summary",
  "По расстоянию": "By distance",
  "По цене": "By price",
  "Повторить": "Retry",
  "Поделиться": "Share",
  "Подключение...": "Connecting...",
  "Подключиться": "Connect",
  "Позвонить": "Call",
  "Позвонить в скорую 103?": "Call an ambulance (103)?",
  "Пока нет подключённых пациентов": "No connected patients yet",
  "Попробовать снова": "Try again",
  "Пора принять": "Time to take",
  "Последние пропуски": "Recent missed doses",
  "Пригласить родственника": "Invite a relative",
  "Пропущено": "Missed",
  "Пропущено: {0}": "Missed: {0}",
  "Разрешите геолокацию, чтобы видеть ближайшие": "Allow location to see the nearest ones",
  "Разрешить камеру": "Allow camera",
  "Родственник": "Relative",
  "Родственник увидит, пьёте ли вы лекарства вовремя": "A relative will see if you take your medicine on time",
  "Сгенерировано ИИ по инструкции НЦЭЛС": "Generated by AI from the official NCELS leaflet",
  "Сегодня": "Today",
  "Сейчас нет в наличии ни в одной аптеке": "Not in stock at any pharmacy right now",
  "Сервер: {0}": "Server: {0}",
  "Сканирование рецепта": "Scan prescription",
  "Сканировать ещё рецепт": "Scan another prescription",
  "Сканировать рецепт": "Scan prescription",
  "Снять": "Take photo",
  "Состояние уведомлений неизвестно": "Notification status unknown",
  "Срочно к врачу": "See a doctor urgently",
  "Стакан воды": "A glass of water",
  "Такое лекарство не найдено. Проверьте название": "Medicine not found. Check the name",
  "Уведомления": "Notifications",
  "Уведомления включены": "Notifications are on",
  "Уведомления выключены": "Notifications are off",
  "Уведомления выключены. Включите их в настройках телефона": "Notifications are off. Turn them on in the phone settings",
  "Фото таблетки": "Pill photo",
  "Через 15 мин": "In 15 min",
  "Это не QR-код рецепта. Проверьте код и попробуйте снова": "This is not a prescription QR code. Check it and try again",
  "Я родственник": "I'm a relative",
  "Я родственник — ввести код": "I'm a relative — enter code",
  "день {0} из {1}": "day {0} of {1}",
  "от {0}, аптек: {1}": "from {0}, pharmacies: {1}",
};
