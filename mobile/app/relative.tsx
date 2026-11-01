import { useRouter } from "expo-router";
import { useCallback, useEffect, useState } from "react";
import { ScrollView, StyleSheet, Text, View } from "react-native";
import { Button } from "../components/Button";
import { Card } from "../components/Card";
import { ApiError, api } from "../lib/api";
import { setRole } from "../lib/storage";
import { colors } from "../lib/theme";
import type { RelativeOverview } from "../lib/types";

function when(iso: string): string {
  const d = new Date(iso);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(d.getDate())}.${p(d.getMonth() + 1)} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

export default function Relative() {
  const router = useRouter();
  const [data, setData] = useState<RelativeOverview | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setData(await api.relativeOverview());
    } catch (e) {
      console.warn("relative: обзор", e);
      setError(e instanceof ApiError ? e.message : "Не удалось загрузить данные");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  async function leave() {
    await setRole("patient");
    router.replace("/");
  }

  return (
    <ScrollView style={styles.screen} contentContainerStyle={styles.content}>
      <Text style={styles.title}>Близкие</Text>
      {loading && !data && !error && <Text style={styles.body}>Загрузка…</Text>}
      {error && (
        <Card style={{ gap: 12 }}>
          <Text style={styles.error}>{error}</Text>
          <Button title="Повторить" onPress={load} />
        </Card>
      )}
      {data && data.patients.length === 0 && (
        <Card>
          <Text style={styles.body}>Пока нет подключённых пациентов</Text>
        </Card>
      )}
      {data?.patients.map((p, i) => (
        <Card key={`${p.patient_name}-${i}`} style={{ gap: 8 }}>
          <Text style={styles.name}>{p.patient_name}</Text>
          <Text style={styles.text}>
            Сегодня: выпито {p.taken}, пропущено {p.missed}
          </Text>
          {p.recent_missed.length > 0 && (
            <View style={{ gap: 4 }}>
              <Text style={styles.body}>Последние пропуски</Text>
              {p.recent_missed.map((m, j) => (
                <Text key={j} style={styles.text}>
                  {m.medicine_name}, {when(m.scheduled_at)}
                </Text>
              ))}
            </View>
          )}
        </Card>
      ))}
      <Button title={loading ? "Обновление..." : "Обновить"} onPress={load} disabled={loading} />
      <Button title="Выйти из режима родственника" variant="secondary" onPress={leave} />
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg },
  content: { padding: 16, paddingTop: 56, gap: 16 },
  title: { fontSize: 24, fontFamily: "Inter_700Bold", color: colors.text },
  name: { fontSize: 20, fontFamily: "Inter_600SemiBold", color: colors.text },
  body: { fontSize: 17, fontFamily: "Inter_400Regular", color: colors.muted },
  text: { fontSize: 17, fontFamily: "Inter_400Regular", color: colors.text },
  error: { fontSize: 17, fontFamily: "Inter_600SemiBold", color: colors.error },
});
