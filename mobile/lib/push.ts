import Constants from "expo-constants";
import * as Notifications from "expo-notifications";
import { api } from "./api";
import { ensureNotificationPermission } from "./notifications";

export function pushProjectId(): string | undefined {
  return Constants.expoConfig?.extra?.eas?.projectId ?? Constants.easConfig?.projectId ?? undefined;
}

// Регистрирует Expo push-токен на сервере. Нет projectId или ошибка — null, без падения.
export async function registerPush(): Promise<string | null> {
  const projectId = pushProjectId();
  if (!projectId) return null;
  try {
    if (!(await ensureNotificationPermission())) return null;
    const { data } = await Notifications.getExpoPushTokenAsync({ projectId });
    await api.pushToken(data);
    return data;
  } catch (e) {
    console.warn("push: не удалось зарегистрировать токен", e);
    return null;
  }
}
