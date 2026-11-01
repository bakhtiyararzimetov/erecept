import * as Notifications from "expo-notifications";
import { router } from "expo-router";
import { Platform } from "react-native";
import { loadCourses } from "./storage";
import { notificationContent, responseKey, snoozeTime, upcomingNotifications } from "./schedule";
import { flushMarks, markTaken } from "./sync";

const CHANNEL = "reminders";
const SNOOZE_PREFIX = "snooze-";

let ready = false;
let listenerAdded = false;
const handled = new Set<string>();
const HANDLED_MAX = 50;
let channelReady: Promise<unknown> = Promise.resolve();

// Вызывается на верхнем уровне _layout.tsx (при загрузке модуля), чтобы кнопки
// без открытия приложения обрабатывались в фоне на Android.
export function setupNotifications(): void {
  if (ready) return;
  ready = true;
  Notifications.setNotificationHandler({
    handleNotification: async () => ({
      shouldPlaySound: true,
      shouldSetBadge: false,
      shouldShowBanner: true,
      shouldShowList: true,
    }),
  });
  if (Platform.OS === "android") {
    channelReady = Notifications.setNotificationChannelAsync(CHANNEL, {
      name: "Напоминания",
      importance: Notifications.AndroidImportance.HIGH,
    }).catch((e) => console.warn("notifications: канал", e));
  }
  Notifications.setNotificationCategoryAsync("dose", [
    { identifier: "TAKEN", buttonTitle: "Выпил", options: { opensAppToForeground: false } },
    { identifier: "SNOOZE", buttonTitle: "Через 15 мин", options: { opensAppToForeground: false } },
  ]).catch((e) => console.warn("notifications: категория", e));

  if (!listenerAdded) {
    listenerAdded = true;
    Notifications.addNotificationResponseReceivedListener((r) => {
      handleResponse(r).catch((e) => console.warn("notifications: ответ", e));
    });
    Notifications.getLastNotificationResponseAsync()
      .then((r) => (r ? handleResponse(r) : undefined))
      .catch((e) => console.warn("notifications: холодный старт", e));
  }
}

export async function ensureNotificationPermission(): Promise<boolean> {
  try {
    const cur = await Notifications.getPermissionsAsync();
    if (cur.granted) return true;
    if (!cur.canAskAgain) return false;
    return (await Notifications.requestPermissionsAsync()).granted;
  } catch (e) {
    console.warn("notifications: разрешение", e);
    return false;
  }
}

let busy: Promise<void> = Promise.resolve();

// Пересоздаёт все будущие напоминания (кроме отложенных «Через 15 мин»). Вызовы идут по очереди.
export function rescheduleAll(): Promise<void> {
  busy = busy.then(rescheduleNow, rescheduleNow);
  return busy;
}

async function rescheduleNow(): Promise<void> {
  const now = new Date();
  await channelReady;
  const courses = await loadCourses();
  const takenSnoozes = new Set(
    courses.flatMap((c) => c.doses.filter((d) => d.taken_at).map((d) => SNOOZE_PREFIX + d.id)),
  );
  const existing = await Notifications.getAllScheduledNotificationsAsync();
  await Promise.all(
    existing
      .filter((n) => !n.identifier.startsWith(SNOOZE_PREFIX) || takenSnoozes.has(n.identifier))
      .map((n) => Notifications.cancelScheduledNotificationAsync(n.identifier)),
  );
  const planned = upcomingNotifications(courses, now, 60);
  for (const p of planned) {
    try {
      await Notifications.scheduleNotificationAsync({
        identifier: p.key,
        content: notificationContent(p),
        trigger: { type: Notifications.SchedulableTriggerInputTypes.DATE, date: p.at, channelId: CHANNEL },
      });
    } catch (e) {
      console.warn("notifications: не удалось запланировать", p.key, e);
    }
  }
}

export async function handleResponse(resp: Notifications.NotificationResponse): Promise<void> {
  const req = resp.notification.request;
  const id = responseKey(req.identifier, resp.actionIdentifier, resp.notification.date);
  if (handled.has(id)) return; // тот же ответ мог прийти и из слушателя, и из getLastNotificationResponseAsync
  handled.add(id);
  if (handled.size > HANDLED_MAX) handled.delete(handled.values().next().value as string);

  const doseId = (req.content.data as { doseId?: number } | null)?.doseId;
  if (resp.actionIdentifier === "TAKEN" && doseId !== undefined) {
    await Notifications.cancelScheduledNotificationAsync(SNOOZE_PREFIX + doseId).catch(() => undefined);
    await markTaken(doseId, new Date());
    await flushMarks();
    await rescheduleAll();
  } else if (resp.actionIdentifier === "SNOOZE" && doseId !== undefined) {
    await channelReady;
    await Notifications.scheduleNotificationAsync({
      identifier: SNOOZE_PREFIX + doseId,
      content: {
        title: req.content.title ?? "",
        body: req.content.body ?? "",
        categoryIdentifier: "dose",
        data: { doseId },
      },
      trigger: { type: Notifications.SchedulableTriggerInputTypes.DATE, date: snoozeTime(new Date()), channelId: CHANNEL },
    });
  } else if (resp.actionIdentifier === Notifications.DEFAULT_ACTION_IDENTIFIER) {
    try {
      router.navigate("/");
    } catch (e) {
      console.warn("notifications: переход на «Сегодня»", e);
    }
  }
}
