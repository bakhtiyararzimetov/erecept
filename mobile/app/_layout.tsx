import { Inter_400Regular, Inter_600SemiBold, Inter_700Bold, useFonts } from "@expo-google-fonts/inter";
import { Manrope_700Bold, Manrope_800ExtraBold } from "@expo-google-fonts/manrope";
import { SplashScreen, Stack } from "expo-router";
import { StatusBar } from "expo-status-bar";
import { useEffect, useState } from "react";
import NetInfo from "@react-native-community/netinfo";
import { AppState, Platform } from "react-native";
import { API_URL } from "../lib/api";
import { flushMarks } from "../lib/sync";
import { rescheduleAll, setupNotifications } from "../lib/notifications";
import { getRole, markRoleCommitted, onRoleChange } from "../lib/storage";
import type { Role } from "../lib/types";
import { colors } from "../lib/theme";
import { initLang, onLangChange, type Lang } from "../lib/i18n";

SplashScreen.preventAutoHideAsync();
// В браузере проверка связи по адресу Google не проходит (CORS) и приложение думает, что интернета нет.
// Проверяем доступность своего сервера.
if (Platform.OS === "web") {
  NetInfo.configure({ reachabilityUrl: API_URL + "/api/health", reachabilityMethod: "GET", reachabilityTest: async (r) => r.status === 200 });
}
setupNotifications(); // на верхнем уровне: фоновые кнопки на Android требуют слушателя при загрузке модуля

export default function RootLayout() {
  const [loaded, error] = useFonts({ Inter_400Regular, Inter_600SemiBold, Inter_700Bold, Manrope_700Bold, Manrope_800ExtraBold });
  // undefined — роль ещё читается; пока не прочитана, ничего не рисуем (заставка остаётся)
  const [role, setRoleState] = useState<Role | null | undefined>(undefined);
  // язык читается до первого кадра; при смене всё дерево перерисовывается заново (key)
  const [lang, setLangState] = useState<Lang | undefined>(undefined);
  useEffect(() => {
    initLang().then(setLangState, () => setLangState("ru"));
    return onLangChange(setLangState);
  }, []);
  useEffect(() => {
    getRole()
      .then(setRoleState)
      .catch((e) => {
        console.warn("layout: роль", e);
        setRoleState(null);
      });
    return onRoleChange(setRoleState);
  }, []);
  const ready = (loaded || !!error) && role !== undefined && lang !== undefined; // без шрифта лучше системный, чем вечная заставка
  const relative = role === "relative";
  useEffect(() => {
    if (role !== undefined) markRoleCommitted(role); // эффект идёт после коммита, когда guard уже обновлён
  }, [role]);

  useEffect(() => {
    if (ready) SplashScreen.hideAsync();
  }, [ready]);

  useEffect(() => {
    const refresh = () => {
      flushMarks().catch(() => undefined);
      rescheduleAll().catch((e) => console.warn("notifications: перепланирование", e));
    };
    refresh();
    const sub = AppState.addEventListener("change", (s) => s === "active" && refresh());
    return () => sub.remove();
  }, []);

  if (!ready) return null;
  return (
    <>
      <StatusBar style="dark" />
      <Stack key={lang} screenOptions={{ headerShown: false, contentStyle: { backgroundColor: colors.bg } }}>
        <Stack.Protected guard={!relative}>
          <Stack.Screen name="(tabs)" />
          <Stack.Screen name="scan" options={{ presentation: "modal" }} />
          <Stack.Screen name="explain" options={{ presentation: "modal" }} />
        </Stack.Protected>
        <Stack.Protected guard={relative}>
          <Stack.Screen name="relative" />
        </Stack.Protected>
        <Stack.Screen name="join" />
      </Stack>
    </>
  );
}
