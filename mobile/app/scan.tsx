import { CameraView, useCameraPermissions } from "expo-camera";
import { useRouter } from "expo-router";
import { useRef, useState } from "react";
import { Linking, ScrollView, StyleSheet, Text, TextInput, View } from "react-native";
import { Button } from "../components/Button";
import { Card } from "../components/Card";
import { ApiError } from "../lib/api";
import { claimAndStore } from "../lib/courseStore";
import { extractCode } from "../lib/schedule";
import { colors, radius } from "../lib/theme";

export default function Scan() {
  const router = useRouter();
  const [permission, requestPermission] = useCameraPermissions();
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [manual, setManual] = useState("");
  const lock = useRef(false); // камера выдаёт один и тот же QR много раз подряд

  async function submit(raw: string) {
    if (lock.current) return;
    const code = extractCode(raw);
    if (!code) {
      setError("Это не QR-код рецепта. Проверьте код и попробуйте снова");
      return;
    }
    lock.current = true;
    setBusy(true);
    setError(null);
    try {
      await claimAndStore(code);
      router.back();
    } catch (e) {
      console.warn("scan: не удалось загрузить рецепт", e);
      setError(e instanceof ApiError ? e.message : "Не удалось загрузить рецепт");
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
      <Text style={styles.title}>Сканирование рецепта</Text>

      {permission && !granted && (
        <Card style={{ gap: 16 }}>
          <Text style={styles.body}>
            Камера нужна, чтобы отсканировать QR-код с рецепта врача
            {permission.canAskAgain ? "." : " Разрешите доступ в настройках телефона или введите код вручную ниже."}
          </Text>
          <Button
            title={permission.canAskAgain ? "Разрешить камеру" : "Открыть настройки"}
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
          <Text style={styles.body}>Наведите камеру на QR-код рецепта</Text>
        </>
      )}

      {busy && <Text style={styles.body}>Загружаем рецепт…</Text>}

      {error && (
        <Card style={{ gap: 16 }}>
          <Text style={[styles.body, { color: colors.error }]}>{error}</Text>
          <Button title="Попробовать снова" onPress={retry} />
        </Card>
      )}

      <Card style={{ gap: 12 }}>
        <Text style={styles.body}>Не работает камера? Введите код рецепта</Text>
        <TextInput
          value={manual}
          onChangeText={setManual}
          placeholder="Код рецепта"
          placeholderTextColor={colors.muted}
          autoCapitalize="none"
          autoCorrect={false}
          style={styles.input}
        />
        <Button title="Загрузить" onPress={() => submit(manual)} disabled={busy || manual.trim() === ""} />
      </Card>

      <Button title="Закрыть" variant="secondary" onPress={() => router.back()} />
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
