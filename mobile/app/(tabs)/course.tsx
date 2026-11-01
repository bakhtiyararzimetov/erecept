import { useFocusEffect } from "expo-router";
import * as ImagePicker from "expo-image-picker";
import { useCallback, useState } from "react";
import { Image, ScrollView, StyleSheet, Text, View } from "react-native";
import { Button } from "../../components/Button";
import { Card } from "../../components/Card";
import { API_URL, ApiError, api } from "../../lib/api";
import { refreshCourses } from "../../lib/courseStore";
import { courseProgress } from "../../lib/schedule";
import { loadCourses, saveCourses } from "../../lib/storage";
import { colors, radius } from "../../lib/theme";
import type { Course, Item } from "../../lib/types";

function fmtDate(iso: string): string {
  const [y, m, d] = iso.split("-");
  return `${d}.${m}.${y}`;
}

function period(c: Course): string {
  const p = c.prescription;
  if (!p.start_date) return "Дата начала не задана";
  const [y, m, d] = p.start_date.split("-").map(Number);
  const days = Math.max(1, ...p.items.map((i) => i.days));
  const end = new Date(y, m - 1, d + days - 1);
  const endIso = `${end.getFullYear()}-${String(end.getMonth() + 1).padStart(2, "0")}-${String(end.getDate()).padStart(2, "0")}`;
  return `${fmtDate(p.start_date)} – ${fmtDate(endIso)}`;
}

export default function CourseScreen() {
  const [courses, setCourses] = useState<Course[] | null>(null);
  const [now, setNow] = useState(new Date());
  const [localPhotos, setLocalPhotos] = useState<Record<number, string>>({});
  const [choosing, setChoosing] = useState<number | null>(null);
  const [busyItem, setBusyItem] = useState<number | null>(null);
  const [photoError, setPhotoError] = useState<{ itemId: number; text: string } | null>(null);

  useFocusEffect(
    useCallback(() => {
      let alive = true;
      (async () => {
        setCourses(await loadCourses());
        const fresh = await refreshCourses();
        if (alive) {
          setCourses(fresh);
          setNow(new Date());
        }
      })().catch((e) => console.warn("course: загрузка", e));
      return () => {
        alive = false;
      };
    }, []),
  );

  async function pick(item: Item, source: "camera" | "gallery") {
    setChoosing(null);
    setPhotoError(null);
    try {
      let res: ImagePicker.ImagePickerResult;
      if (source === "camera") {
        const perm = await ImagePicker.requestCameraPermissionsAsync();
        if (!perm.granted) {
          setPhotoError({ itemId: item.id, text: "Нет доступа к камере. Разрешите его в настройках телефона" });
          return;
        }
        res = await ImagePicker.launchCameraAsync({ quality: 0.6 });
      } else {
        res = await ImagePicker.launchImageLibraryAsync({ mediaTypes: ["images"], quality: 0.6 });
      }
      if (res.canceled || !res.assets[0]) return;
      const uri = res.assets[0].uri;
      setBusyItem(item.id);
      const { photo_url } = await api.uploadPhoto(item.id, uri);
      setLocalPhotos((p) => ({ ...p, [item.id]: uri }));
      const saved = await loadCourses();
      for (const c of saved) for (const it of c.prescription.items) if (it.id === item.id) it.photo_url = photo_url;
      await saveCourses(saved);
      setCourses(saved);
    } catch (e) {
      console.warn("course: фото не загружено", e);
      setPhotoError({ itemId: item.id, text: e instanceof ApiError ? e.message : "Не удалось загрузить фото. Попробуйте ещё раз" });
    } finally {
      setBusyItem(null);
    }
  }

  if (courses === null) return <View style={styles.screen} />;

  return (
    <ScrollView style={styles.screen} contentContainerStyle={styles.content}>
      <Text style={styles.title}>Курс</Text>
      {courses.length === 0 && (
        <Card>
          <Text style={styles.body}>Курсов пока нет. Отсканируйте рецепт на экране «Сегодня»</Text>
        </Card>
      )}
      {courses.map((c) => {
        const pr = courseProgress(c, now);
        const left = pr.total - pr.taken - pr.missed;
        return (
          <Card key={c.prescription.id} style={{ gap: 12 }}>
            <Text style={styles.name}>{c.prescription.doctor_name}</Text>
            <Text style={styles.body}>{period(c)}</Text>
            <Text style={styles.percent}>{pr.percent}%</Text>
            <View style={styles.track}>
              <View style={[styles.fill, { width: `${pr.percent}%` }]} />
            </View>
            <View style={styles.stats}>
              <Text style={styles.body}>Отмечено: {pr.taken}</Text>
              <Text style={styles.body}>Пропущено: {pr.missed}</Text>
              <Text style={styles.body}>Осталось: {left}</Text>
            </View>
            {!!c.prescription.comment && (
              <View style={styles.comment}>
                <Text style={styles.label}>Комментарий врача</Text>
                <Text style={styles.text}>{c.prescription.comment}</Text>
              </View>
            )}
            {c.prescription.items.map((item) => {
              const doses = c.doses.filter((d) => d.item_id === item.id);
              const sub = courseProgress({ prescription: c.prescription, doses }, now);
              const photo = localPhotos[item.id] ?? (item.photo_url ? `${API_URL}${item.photo_url}` : null);
              return (
                <View key={item.id} style={styles.item}>
                  <View style={styles.itemHead}>
                    <Text style={[styles.name, { flex: 1 }]}>{item.medicine_name}</Text>
                    <Text style={styles.name}>{sub.percent}%</Text>
                  </View>
                  <Text style={styles.body}>
                    {item.dosage}, {item.meal_label}
                  </Text>
                  <View style={styles.photoRow}>
                    {photo && <Image source={{ uri: photo }} style={styles.thumb} accessibilityLabel="Фото таблетки" />}
                    <View style={{ flex: 1 }}>
                      <Button
                        title={busyItem === item.id ? "Загрузка..." : "Фото таблетки"}
                        variant="secondary"
                        disabled={busyItem === item.id}
                        onPress={() => setChoosing(choosing === item.id ? null : item.id)}
                      />
                    </View>
                  </View>
                  {choosing === item.id && (
                    <View style={{ gap: 8 }}>
                      <Button title="Снять" variant="secondary" onPress={() => pick(item, "camera")} />
                      <Button title="Из галереи" variant="secondary" onPress={() => pick(item, "gallery")} />
                    </View>
                  )}
                  {photoError?.itemId === item.id && <Text style={styles.error}>{photoError.text}</Text>}
                </View>
              );
            })}
          </Card>
        );
      })}
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
  label: { fontSize: 17, fontFamily: "Inter_600SemiBold", color: colors.text },
  percent: { fontSize: 48, fontFamily: "Inter_700Bold", color: colors.text },
  track: { height: 12, borderRadius: 6, backgroundColor: colors.border, overflow: "hidden" },
  fill: { height: 12, backgroundColor: colors.accent },
  stats: { gap: 4 },
  comment: { backgroundColor: colors.bg, borderRadius: radius.control, padding: 12, gap: 4 },
  item: { borderTopWidth: 1, borderTopColor: colors.border, paddingTop: 12, gap: 8 },
  itemHead: { flexDirection: "row", gap: 8 },
  photoRow: { flexDirection: "row", gap: 12, alignItems: "center" },
  thumb: { width: 56, height: 56, borderRadius: radius.control, backgroundColor: colors.border },
  error: { fontSize: 17, fontFamily: "Inter_400Regular", color: colors.error },
});
