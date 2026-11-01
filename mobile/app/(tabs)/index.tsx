import NetInfo from "@react-native-community/netinfo";
import { useFocusEffect, useRouter } from "expo-router";
import { useCallback, useEffect, useState } from "react";
import { ScrollView, StyleSheet, Text, View } from "react-native";
import { Button } from "../../components/Button";
import { Card } from "../../components/Card";
import { refreshCourses } from "../../lib/courseStore";
import { rescheduleAll } from "../../lib/notifications";
import { todayList, type DoseStatus } from "../../lib/schedule";
import { loadCourses } from "../../lib/storage";
import { flushMarks, markTaken } from "../../lib/sync";
import { colors, radius } from "../../lib/theme";
import type { Course } from "../../lib/types";

const STATUS: Record<DoseStatus, { label: string; color: string } | null> = {
  taken: { label: "Выпито", color: colors.accent },
  missed: { label: "Пропущено", color: colors.error },
  due: { label: "Пора принять", color: colors.text },
  upcoming: null,
};

function dateTitle(d: Date): string {
  return new Intl.DateTimeFormat("ru-RU", { weekday: "long", day: "numeric", month: "long" }).format(d);
}

function timeOf(iso: string): string {
  const d = new Date(iso);
  return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

export default function Today() {
  const router = useRouter();
  const [courses, setCourses] = useState<Course[] | null>(null);
  const [online, setOnline] = useState(true);
  const [now, setNow] = useState(new Date());

  const refresh = useCallback(async () => {
    setCourses(await loadCourses()); // сразу показываем сохранённое
    setCourses(await refreshCourses());
    setNow(new Date());
  }, []);

  useFocusEffect(
    useCallback(() => {
      refresh();
      const t = setInterval(() => setNow(new Date()), 60_000);
      return () => clearInterval(t);
    }, [refresh]),
  );

  useEffect(() => {
    let wasOnline = true;
    return NetInfo.addEventListener((s) => {
      const isOnline = s.isConnected !== false && s.isInternetReachable !== false;
      setOnline(isOnline);
      if (isOnline && !wasOnline) flushMarks().catch(() => undefined);
      wasOnline = isOnline;
    });
  }, []);

  async function onTaken(doseId: number) {
    const at = new Date();
    await markTaken(doseId, at);
    setCourses(await loadCourses());
    setNow(at);
    rescheduleAll().catch(() => undefined);
    flushMarks().catch(() => undefined);
  }

  if (courses === null) return <View style={styles.screen} />;

  const entries = todayList(courses, now);
  

  return (
    <ScrollView style={styles.screen} contentContainerStyle={styles.content}>
      {!online && (
        <View style={styles.offline}>
          <Text style={styles.offlineText}>Нет интернета. Отметки сохранятся и отправятся позже</Text>
        </View>
      )}
      {courses.length === 0 ? (
        <Card style={{ gap: 16 }}>
          <Text style={styles.title}>Сегодня</Text>
          <Text style={styles.body}>Отсканируйте QR-код с рецепта врача</Text>
          <Button title="Сканировать рецепт" onPress={() => router.push("/scan")} />
        </Card>
      ) : (
        <>
          <Text style={styles.date}>{dateTitle(now)}</Text>
          <Button title="Сканировать ещё рецепт" variant="secondary" onPress={() => router.push("/scan")} />
          {entries.length === 0 && (
            <Card>
              <Text style={styles.body}>На сегодня приёмов нет</Text>
            </Card>
          )}
          {entries.map(({ dose, item, status }) => {
            const badge = STATUS[status];
            return (
              <Card key={dose.id} style={{ gap: 8 }}>
                <View style={styles.row}>
                  <Text style={styles.time}>{timeOf(dose.scheduled_at)}</Text>
                  {badge && <Text style={[styles.badge, { color: badge.color, borderColor: badge.color }]}>{badge.label}</Text>}
                </View>
                <Text style={styles.name}>{item.medicine_name}</Text>
                <Text style={styles.body}>
                  {item.dosage}, {item.meal_label}
                </Text>
                {status !== "taken" && <Button title="Выпил" onPress={() => onTaken(dose.id)} />}
                <Button
                  title="Где купить"
                  variant="secondary"
                  onPress={() => router.push({ pathname: "/shops", params: { name: item.medicine_name } })}
                />
              </Card>
            );
          })}
        </>
      )}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg },
  content: { padding: 16, paddingTop: 56, gap: 16 },
  offline: { backgroundColor: "#E5E7EB", borderRadius: radius.control, padding: 12 },
  offlineText: { fontSize: 17, fontFamily: "Inter_600SemiBold", color: colors.text },
  title: { fontSize: 24, fontFamily: "Inter_700Bold", color: colors.text },
  date: { fontSize: 24, fontFamily: "Inter_700Bold", color: colors.text, textTransform: "capitalize" },
  body: { fontSize: 17, fontFamily: "Inter_400Regular", color: colors.muted },
  row: { flexDirection: "row", justifyContent: "space-between", alignItems: "center" },
  time: { fontSize: 22, fontFamily: "Inter_700Bold", color: colors.text },
  badge: {
    fontSize: 17, fontFamily: "Inter_600SemiBold", borderWidth: 1, borderRadius: radius.control,
    paddingHorizontal: 10, paddingVertical: 4,
  },
  name: { fontSize: 20, fontFamily: "Inter_600SemiBold", color: colors.text },
});
