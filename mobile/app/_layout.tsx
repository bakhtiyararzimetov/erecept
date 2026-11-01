import { Inter_400Regular, Inter_600SemiBold, Inter_700Bold, useFonts } from "@expo-google-fonts/inter";
import { SplashScreen, Stack, useRouter, useSegments } from "expo-router";
import { StatusBar } from "expo-status-bar";
import { useEffect } from "react";
import { AppState } from "react-native";
import { flushMarks } from "../lib/sync";
import { rescheduleAll, setupNotifications } from "../lib/notifications";
import { getRole } from "../lib/storage";
import { colors } from "../lib/theme";

SplashScreen.preventAutoHideAsync();
setupNotifications(); // на верхнем уровне: фоновые кнопки на Android требуют слушателя при загрузке модуля

export default function RootLayout() {
  const [loaded, error] = useFonts({ Inter_400Regular, Inter_600SemiBold, Inter_700Bold });
  const ready = loaded || !!error; // без шрифта лучше системный, чем вечная заставка

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

  // Родственник видит только relative.tsx (и join), пациент — вкладки.
  const router = useRouter();
  const segments = useSegments() as string[];
  const first = segments[0];
  useEffect(() => {
    if (!ready) return;
    getRole()
      .then((role) => {
        if (role === "relative" && first !== "relative" && first !== "join") router.replace("/relative");
        else if (role !== "relative" && first === "relative") router.replace("/");
      })
      .catch((e) => console.warn("layout: роль", e));
  }, [ready, first, router]);

  if (!ready) return null;
  return (
    <>
      <StatusBar style="dark" />
      <Stack screenOptions={{ headerShown: false, contentStyle: { backgroundColor: colors.bg } }}>
        <Stack.Screen name="(tabs)" />
        <Stack.Screen name="relative" />
        <Stack.Screen name="join" />
        <Stack.Screen name="scan" options={{ presentation: "modal" }} />
      </Stack>
    </>
  );
}
