import { CameraView, useCameraPermissions } from "expo-camera";
import { useLocalSearchParams, useRouter } from "expo-router";
import { useEffect, useRef, useState } from "react";
import { Linking, ScrollView, StyleSheet, Text, TextInput, View } from "react-native";
import { Button } from "../components/Button";
import { Card } from "../components/Card";
import { ApiError } from "../lib/api";
import { claimAndStore } from "../lib/courseStore";
import { registerPush } from "../lib/push";
import { ensureNotificationPermission } from "../lib/notifications";
import { extractCode } from "../lib/schedule";
import { colors, radius } from "../lib/theme";
import { t, tf } from "../lib/i18n";

export default function Scan() {
  const router = useRouter();
  const [permission, requestPermission] = useCameraPermissions();
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [manual, setManual] = useState("");
  const lock = useRef(false); // камера выдаёт один и тот же QR много раз подряд
  // Ссылка со страницы рецепта (/app/scan?code=…) — рецепт загружается сразу, без камеры.
  const { code: linkCode } = useLocalSearchParams<{ code?: string }>();
  useEffect(() => {
    if (linkCode) submit(String(linkCode));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [linkCode]);

  async function submit(raw: string) {
    if (lock.current) return;
    const code = extractCode(raw);
    if (!code) {
      setError(t("Это не QR-код рецепта. Проверьте код и попробуйте снова"));
      return;
    }
    lock.current = true;
    setBusy(true);
    setError(null);
    try {
      await claimAndStore(code);
      await ensureNotificationPermission(); // отказ не блокирует
      registerPush().catch((e) => console.warn("push: регистрация после скана", e));
      if (router.canGoBack()) router.back();
      else router.replace("/");
    } catch (e) {
      console.warn("scan: не удалось загрузить рецепт", e);
      setError(e instanceof ApiError ? e.message : t("Не удалось загрузить рецепт"));
      lock.current = false;
    } finally {
      setBusy(false);
    }
  }

  function retry() {
    setError(null);
    lock.current = false;
  }

  const granted = permission?.granted;
  const showCamera = granted && !error && !busy;

  return (
    <ScrollView style={styles.screen} contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled">
      <Text style={styles.title}>{t("Сканирование рецепта")}</Text>

      {permission && !granted && (
        <Card style={{ gap: 16 }}>
          <Text style={styles.body}>
            {t("Камера нужна, чтобы отсканировать QR-код с рецепта врача")}
            {permission.canAskAgain ? "." : t(" Разрешите доступ в настройках телефона или введите код вручную ниже.")}
          </Text>
          <Button
            title={permission.canAskAgain ? t("Разрешить камеру") : t("Открыть настройки")}
            onPress={() => (permission.canAskAgain ? requestPermission() : Linking.openSettings())}
          />
        </Card>
      )}

      {showCamera && (
        <>
          <View style={styles.cameraBox}>
            <CameraView
              style={StyleSheet.absoluteFill}
              facing="back"
              barcodeScannerSettings={{ barcodeTypes: ["qr"] }}
              onBarcodeScanned={({ data }) => submit(data)}
            />
            <View style={styles.frame} pointerEvents="none" />
          </View>
          <Text style={styles.body}>{t("Наведите камеру на QR-код рецепта")}</Text>
        </>
      )}

      {busy && <Text style={styles.body}>{t("Загружаем рецепт…")}</Text>}

      {error && (
        <Card style={{ gap: 16 }}>
          <Text style={[styles.body, { color: colors.error }]}>{error}</Text>
          <Button title={t("Попробовать снова")} onPress={retry} />
        </Card>
      )}

      <Card style={{ gap: 12 }}>
        <Text style={styles.body}>{t("Не работает камера? Введите код рецепта")}</Text>
        <TextInput
          value={manual}
          onChangeText={setManual}
          placeholder={t("Код рецепта")}
          placeholderTextColor={colors.muted}
          autoCapitalize="none"
          autoCorrect={false}
          style={styles.input}
        />
        <Button title={t("Загрузить")} onPress={() => submit(manual)} disabled={busy || manual.trim() === ""} />
      </Card>

      <Button title={t("Закрыть")} variant="secondary" onPress={() => router.back()} />
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg },
  content: { padding: 16, paddingTop: 32, gap: 16 },
  title: { fontSize: 24, fontFamily: "Inter_700Bold", color: colors.text },
  body: { fontSize: 17, fontFamily: "Inter_400Regular", color: colors.text },
  cameraBox: { height: 340, borderRadius: radius.card, overflow: "hidden", backgroundColor: "#000" },
  frame: {
    position: "absolute", top: "15%", left: "15%", right: "15%", bottom: "15%",
    borderWidth: 3, borderColor: "#FFFFFF", borderRadius: radius.card,
  },
  input: {
    minHeight: 56, borderWidth: 1, borderColor: colors.border, borderRadius: radius.control,
    paddingHorizontal: 12, fontSize: 17, fontFamily: "Inter_400Regular", color: colors.text,
    backgroundColor: colors.card,
  },
});
