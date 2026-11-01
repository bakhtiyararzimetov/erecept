import { useRouter } from "expo-router";
import { useState } from "react";
import { KeyboardAvoidingView, Platform, ScrollView, StyleSheet, Text, TextInput } from "react-native";
import { Button } from "../components/Button";
import { Card } from "../components/Card";
import { ApiError, api } from "../lib/api";
import { registerPush } from "../lib/push";
import { setRole, waitForRole } from "../lib/storage";
import { colors, radius } from "../lib/theme";

export default function Join() {
  const router = useRouter();
  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit() {
    const c = code.trim();
    const n = name.trim();
    if (!/^\d{6}$/.test(c)) return setError("Введите код из 6 цифр");
    if (!n) return setError("Введите ваше имя");
    setBusy(true);
    setError(null);
    try {
      await api.joinRelative(c, n);
      await setRole("relative");
      registerPush().catch(() => undefined);
      await waitForRole("relative");
      router.replace("/relative");
    } catch (e) {
      console.warn("join: не удалось подключиться", e);
      setError(e instanceof ApiError ? e.message : "Не удалось подключиться");
    } finally {
      setBusy(false);
    }
  }

  return (
    <KeyboardAvoidingView style={{ flex: 1 }} behavior={Platform.OS === "ios" ? "padding" : undefined}>
      <ScrollView style={styles.screen} contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled">
        <Text style={styles.title}>Я родственник</Text>
        <Card style={{ gap: 12 }}>
          <Text style={styles.label}>Код из приложения пациента</Text>
          <TextInput
            value={code}
            onChangeText={(t) => setCode(t.replace(/\D/g, "").slice(0, 6))}
            keyboardType="number-pad"
            maxLength={6}
            style={[styles.input, { letterSpacing: 6, fontSize: 24 }]}
          />
          <Text style={styles.label}>Ваше имя</Text>
          <TextInput value={name} onChangeText={setName} style={styles.input} />
          {error && <Text style={styles.error}>{error}</Text>}
          <Button title={busy ? "Подключение..." : "Подключиться"} onPress={submit} disabled={busy} />
          <Button title="Назад" variant="secondary" onPress={() => router.back()} />
        </Card>
      </ScrollView>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg },
  content: { padding: 16, paddingTop: 56, gap: 16 },
  title: { fontSize: 24, fontFamily: "Inter_700Bold", color: colors.text },
  label: { fontSize: 17, fontFamily: "Inter_600SemiBold", color: colors.text },
  input: {
    minHeight: 56, borderWidth: 1, borderColor: colors.border, borderRadius: radius.control,
    paddingHorizontal: 16, fontSize: 17, fontFamily: "Inter_400Regular", color: colors.text,
  },
  error: { fontSize: 17, fontFamily: "Inter_600SemiBold", color: colors.error },
});
