import NetInfo from "@react-native-community/netinfo";
import { useFocusEffect, useRouter } from "expo-router";
import { useCallback, useEffect, useRef, useState } from "react";
import { Image, Modal, Pressable, ScrollView, StyleSheet, Text, View } from "react-native";
import { API_URL } from "../../lib/api";
import { Button } from "../../components/Button";
import { SosButton } from "../../components/SosButton";
import { Card } from "../../components/Card";
import { refreshCourses } from "../../lib/courseStore";
import { rescheduleAll } from "../../lib/notifications";
import { isEarly, lateTakenAt, todayList, type DoseStatus } from "../../lib/schedule";
import { confirm } from "../../lib/confirm";
import { loadCourses } from "../../lib/storage";
import { flushMarks, markTaken } from "../../lib/sync";
import Ionicons from "@expo/vector-icons/Ionicons";
import { colors, fonts, radius, shadow } from "../../lib/theme";
import type { Course, Dose } from "../../lib/types";
import { locale, t, tf } from "../../lib/i18n";

// Статус приёма: спокойные цвета — опоздание не красное, а янтарное (как у Round Health).
const STATUS: Record<DoseStatus, { label: string; color: string; bg: string } | null> = {
  taken: { label: "Выпито", color: colors.success, bg: colors.successSoft },
  missed: { label: "Пропущено", color: colors.warning, bg: colors.warningSoft },
  due: { label: "Пора принять", color: colors.accentDark, bg: colors.accentSoft },
  upcoming: null,
};

function greeting(d: Date): string {
  const h = d.getHours();
  if (h < 5) return t("Доброй ночи");
  if (h < 12) return t("Доброе утро");
  if (h < 18) return t("Добрый день");
  return t("Добрый вечер");
}

function dateTitle(d: Date): string {
  return new Intl.DateTimeFormat(locale(), { weekday: "long", day: "numeric", month: "long" }).format(d);
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
  const [actionError, setActionError] = useState<string | null>(null);
  const inFlight = useRef(new Set<number>());
  // «Выпил» сохраняется не сразу: 5 секунд можно нажать «Отменить» (ошибочная отметка уходит врачу).
  type Pending = { doseId: number; name: string; at: Date };
  const [undo, setUndo] = useState<Pending | null>(null);
  const undoTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const undoRef = useRef<Pending | null>(null);
  // Пропущенный приём: спрашиваем, когда выпили (вовремя или только что) — врачу уходит верное время.
  const [askLate, setAskLate] = useState<{ dose: Dose; name: string; time: string } | null>(null);

  function commitUndo() {
    if (undoTimer.current) clearTimeout(undoTimer.current);
    undoTimer.current = null;
    const u = undoRef.current;
    undoRef.current = null;
    setUndo(null);
    if (u) onTaken(u.doseId, u.at);
  }

  function hold(doseId: number, name: string, at: Date) {
    if (undoRef.current) commitUndo(); // предыдущую отметку сохраняем сразу
    undoRef.current = { doseId, name, at };
    setUndo(undoRef.current);
    undoTimer.current = setTimeout(commitUndo, 5000);
  }

  async function onCheck(dose: Dose, status: DoseStatus, name: string, time: string) {
    if (status === "missed") {
      setAskLate({ dose, name, time });
      return;
    }
    if (status === "upcoming") {
      const ok = await confirm(tf("Ещё не время: приём в {0}. Отметить сейчас?", time), t("Отметить"), t("Отмена"));
      if (!ok) return;
    }
    hold(dose.id, name, new Date());
  }

  function answerLate(when: "on_time" | "now") {
    const a = askLate;
    setAskLate(null);
    if (a) hold(a.dose.id, a.name, lateTakenAt(a.dose, when, new Date()));
  }

  function cancelUndo() {
    if (undoTimer.current) clearTimeout(undoTimer.current);
    undoTimer.current = null;
    undoRef.current = null;
    setUndo(null);
  }

  // уход с экрана — отметку не теряем
  useEffect(() => () => {
    if (undoRef.current) commitUndo();
  }, []);

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
      if (isOnline && !wasOnline) {
        flushMarks()
          .then(() => refreshCourses())
          .then((c) => setCourses(c))
          .catch(() => undefined);
      }
      wasOnline = isOnline;
    });
  }, []);

  async function onTaken(doseId: number, at: Date) {
    if (inFlight.current.has(doseId)) return;
    inFlight.current.add(doseId);
    setActionError(null);
    try {
      await markTaken(doseId, at);
      setCourses(await loadCourses());
      setNow(new Date());
      rescheduleAll().catch(() => undefined);
      flushMarks().catch(() => undefined);
    } catch (e) {
      console.warn("today: не удалось сохранить отметку", e);
      setActionError(t("Не удалось сохранить отметку. Попробуйте ещё раз"));
    } finally {
      inFlight.current.delete(doseId);
    }
  }

  if (courses === null) return <View style={styles.screen} />;

  const entries = todayList(courses, now);
  

  const taken = entries.filter((e) => e.status === "taken" || e.dose.id === undo?.doseId).length;
  const groups: { time: string; list: typeof entries }[] = [];
  for (const e of entries) {
    const tm = timeOf(e.dose.scheduled_at);
    const g = groups[groups.length - 1];
    if (g && g.time === tm) g.list.push(e);
    else groups.push({ time: tm, list: [e] });
  }

  return (
    <View style={styles.screen}>
    <ScrollView style={styles.screen} contentContainerStyle={styles.content}>
      {!online && (
        <View style={styles.offline}>
          <Text style={styles.offlineText}>{t("Нет интернета. Отметки сохранятся и отправятся позже")}</Text>
        </View>
      )}
      {actionError && (
        <View style={styles.offline}>
          <Text style={[styles.offlineText, { color: colors.error }]}>{actionError}</Text>
        </View>
      )}

      {courses.filter((c) => c.prescription.cancelled).map((c) => (
        <View key={"x" + c.prescription.id} style={styles.cancelled} accessibilityRole="alert">
          <Ionicons name="close-circle" size={22} color={colors.error} />
          <View style={{ flex: 1, gap: 2 }}>
            <Text style={styles.cancelledTitle}>{t("Врач отменил рецепт")}</Text>
            <Text style={styles.cancelledText}>
              {c.prescription.items.map((i) => i.medicine_name).join(", ")}. {t("Не принимайте эти лекарства по этому рецепту.")}
            </Text>
          </View>
        </View>
      ))}
      {courses.length === 0 ? (
        <View style={styles.empty}>
          <View style={styles.emptyIcon}>
            <Ionicons name="qr-code-outline" size={44} color={colors.accent} />
          </View>
          <Text style={styles.hello}>{greeting(now)}</Text>
          <Text style={styles.emptyText}>{t("Отсканируйте QR-код с рецепта врача")}</Text>
          <Button title={t("Сканировать рецепт")} onPress={() => router.push("/scan")} />
        </View>
      ) : (
        <>
          <View style={styles.hero}>
            <View style={{ flex: 1 }}>
              <Text style={styles.heroHello}>{greeting(now)}</Text>
              <Text style={styles.heroDate}>{dateTitle(now)}</Text>
            </View>
            {entries.length > 0 && (
              <View style={styles.heroCount}>
                <Text style={styles.heroNum}>{taken}/{entries.length}</Text>
                <Text style={styles.heroLabel}>{t("принято")}</Text>
              </View>
            )}
          </View>
          {entries.length > 0 && (
            <View style={styles.track}>
              <View style={[styles.fill, { width: `${Math.round((taken * 100) / entries.length)}%` }]} />
            </View>
          )}

          {entries.length === 0 && (
            <Card>
              <Text style={styles.body}>{t("На сегодня приёмов нет")}</Text>
            </Card>
          )}

          {groups.map((g) => (
            <View key={g.time} style={{ gap: 10 }}>
              <Text style={styles.groupTime}>{g.time}</Text>
              {g.list.map(({ dose, item, status }) => {
                const pendingTaken = undo?.doseId === dose.id;
                const shown: DoseStatus = pendingTaken ? "taken" : status;
                const badge = STATUS[shown];
                const done = shown === "taken";
                return (
                  <Card key={dose.id} style={done ? { ...styles.dose, opacity: 0.62 } : styles.dose}>
                    <View style={styles.doseRow}>
                      {item.image ? (
                        <Image source={{ uri: API_URL + item.image }} style={styles.pack} accessibilityLabel={item.medicine_name} />
                      ) : (
                        <View style={[styles.pack, styles.packStub]}>
                          <Ionicons name="medical-outline" size={26} color={colors.accent} />
                        </View>
                      )}
                      <View style={{ flex: 1, gap: 4 }}>
                        <Text style={styles.name}>{item.medicine_name}</Text>
                        <Text style={styles.small}>
                          {item.dosage} · {item.meal_label}
                        </Text>
                        <View style={styles.chips}>
                          {badge && <Text style={[styles.chip, { color: badge.color, backgroundColor: badge.bg }]}>{t(badge.label)}</Text>}
                          <Text style={styles.dayText}>{tf("день {0} из {1}", dose.day_number, dose.total_days)}</Text>
                        </View>
                      </View>
                      <Pressable
                        accessibilityRole="button"
                        accessibilityLabel={done ? t("Выпито") : t("Выпил")}
                        disabled={done}
                        onPress={() => onCheck(dose, isEarly(dose, new Date()) ? "upcoming" : status, item.medicine_name, timeOf(dose.scheduled_at))}
                        style={({ pressed }) => [styles.check, done && styles.checkDone, pressed && styles.checkPressed]}
                      >
                        {done ? <Ionicons name="checkmark" size={28} color="#FFFFFF" /> : <Text style={styles.checkLabel}>{t("Выпил")}</Text>}
                      </Pressable>
                    </View>
                    <View style={styles.links}>
                      {item.explanation && (
                        <Pressable
                          accessibilityRole="button"
                          style={styles.link}
                          onPress={() => router.push({ pathname: "/explain", params: { itemId: String(item.id) } })}
                        >
                          <Ionicons name="information-circle-outline" size={17} color={colors.accentDark} />
                          <Text style={styles.linkText}>{t("О лекарстве")}</Text>
                        </Pressable>
                      )}
                      <Pressable
                        accessibilityRole="button"
                        style={styles.link}
                        onPress={() => router.push({ pathname: "/shops", params: { name: item.medicine_name } })}
                      >
                        <Ionicons name="storefront-outline" size={17} color={colors.accentDark} />
                        <Text style={styles.linkText}>{t("Где купить")}</Text>
                      </Pressable>
                    </View>
                  </Card>
                );
              })}
            </View>
          ))}

          <Button title={t("Сканировать ещё рецепт")} variant="secondary" onPress={() => router.push("/scan")} />
        </>
      )}
      <SosButton />
    </ScrollView>
    <Modal visible={!!askLate} transparent animationType="fade" onRequestClose={() => setAskLate(null)}>
      <Pressable style={styles.backdrop} onPress={() => setAskLate(null)} accessibilityLabel={t("Отмена")}>
        <Pressable style={styles.sheet} onPress={() => undefined}>
          <Text style={styles.sheetTitle}>{t("Когда вы выпили?")}</Text>
          <Text style={styles.sheetText}>{askLate ? tf("{0}: приём был в {1}", askLate.name, askLate.time) : ""}</Text>
          <Button title={askLate ? tf("Вовремя, в {0}", askLate.time) : ""} onPress={() => answerLate("on_time")} />
          <Button title={t("Только что")} variant="secondary" onPress={() => answerLate("now")} />
          <Pressable accessibilityRole="button" onPress={() => setAskLate(null)} style={styles.sheetCancel}>
            <Text style={styles.sheetCancelText}>{t("Не выпил")}</Text>
          </Pressable>
        </Pressable>
      </Pressable>
    </Modal>
    {undo && (
      <View style={styles.snack} accessibilityLiveRegion="polite">
        <Ionicons name="checkmark-circle" size={22} color="#7EE0C3" />
        <Text style={styles.snackText} numberOfLines={2}>{tf("{0}: отмечено «Выпито»", undo.name)}</Text>
        <Pressable accessibilityRole="button" onPress={cancelUndo} style={styles.snackBtn}>
          <Text style={styles.snackBtnText}>{t("Отменить")}</Text>
        </Pressable>
      </View>
    )}
    </View>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg },
  content: { padding: 16, paddingTop: 20, gap: 16, paddingBottom: 32 },
  offline: { backgroundColor: colors.warningSoft, borderRadius: radius.control, padding: 12 },
  offlineText: { fontSize: 16, fontFamily: "Inter_600SemiBold", color: colors.warning },
  hero: { flexDirection: "row", alignItems: "center", gap: 12 },
  heroHello: { fontSize: 28, fontFamily: fonts.title, color: colors.text, letterSpacing: -0.6 },
  heroDate: { fontSize: 16, fontFamily: "Inter_400Regular", color: colors.muted, marginTop: 2, textTransform: "capitalize" },
  heroCount: { alignItems: "center", backgroundColor: colors.card, borderRadius: 18, paddingVertical: 10, paddingHorizontal: 16, ...shadow },
  heroNum: { fontSize: 24, fontFamily: fonts.title, color: colors.accent },
  heroLabel: { fontSize: 13, fontFamily: "Inter_600SemiBold", color: colors.muted },
  track: { height: 8, borderRadius: 4, backgroundColor: colors.accentSoft, overflow: "hidden", marginTop: -4 },
  fill: { height: 8, borderRadius: 4, backgroundColor: colors.accent },
  groupTime: { fontSize: 14, fontFamily: "Inter_700Bold", color: colors.muted, letterSpacing: 1, marginTop: 4 },
  dose: { gap: 10, paddingVertical: 14, paddingHorizontal: 14 },
  doseRow: { flexDirection: "row", alignItems: "center", gap: 12 },
  pack: { width: 64, height: 41, borderRadius: 7 },
  packStub: { backgroundColor: colors.accentSoft, alignItems: "center", justifyContent: "center" },
  name: { fontSize: 19, fontFamily: fonts.heading, color: colors.text },
  small: { fontSize: 15, fontFamily: "Inter_400Regular", color: colors.muted },
  chips: { flexDirection: "row", flexWrap: "wrap", alignItems: "center", columnGap: 8, rowGap: 4, marginTop: 2 },
  dayText: { fontSize: 14, fontFamily: "Inter_600SemiBold", color: colors.muted },
  chip: { flexShrink: 0, fontSize: 13, fontFamily: "Inter_600SemiBold", paddingHorizontal: 9, paddingVertical: 4, borderRadius: 999, overflow: "hidden" },
  chipMuted: { color: colors.muted, backgroundColor: colors.accentFaint },
  // Не отмечено — пустой круг с подписью «Выпил»; отмечено — зелёный круг с галочкой.
  check: { width: 64, height: 64, borderRadius: 32, borderWidth: 2, borderColor: colors.accent, alignItems: "center", justifyContent: "center", backgroundColor: colors.card },
  checkPressed: { transform: [{ scale: 0.94 }], backgroundColor: colors.accentSoft },
  checkLabel: { fontSize: 14, fontFamily: "Inter_700Bold", color: colors.accentDark },
  checkDone: { backgroundColor: colors.success, borderColor: colors.success },
  links: { flexDirection: "row", gap: 4, borderTopWidth: 1, borderTopColor: colors.border, marginHorizontal: -14, paddingHorizontal: 6, marginBottom: -6, paddingTop: 2 },
  link: { minHeight: 44, paddingHorizontal: 8, flexDirection: "row", gap: 6, alignItems: "center" },
  linkText: { fontSize: 15, fontFamily: "Inter_600SemiBold", color: colors.accentDark },
  cancelled: { flexDirection: "row", gap: 10, backgroundColor: "#FDF1EF", borderRadius: radius.control, padding: 14, borderLeftWidth: 3, borderLeftColor: colors.error },
  cancelledTitle: { fontSize: 16, fontFamily: "Inter_700Bold", color: colors.error },
  cancelledText: { fontSize: 15, fontFamily: "Inter_400Regular", color: colors.text },
  backdrop: { flex: 1, backgroundColor: "rgba(10,30,28,0.45)", justifyContent: "flex-end", padding: 12 },
  sheet: { backgroundColor: colors.card, borderRadius: 20, padding: 20, gap: 12, width: "100%", maxWidth: 480, alignSelf: "center" },
  sheetTitle: { fontSize: 22, fontFamily: fonts.title, color: colors.text },
  sheetText: { fontSize: 16, fontFamily: "Inter_400Regular", color: colors.muted, marginBottom: 4 },
  sheetCancel: { minHeight: 48, alignItems: "center", justifyContent: "center" },
  sheetCancelText: { fontSize: 16, fontFamily: "Inter_600SemiBold", color: colors.muted },
  snack: { position: "absolute", left: 16, right: 16, bottom: 16, backgroundColor: "#17302D", borderRadius: 14, paddingLeft: 14, paddingRight: 6, minHeight: 56, flexDirection: "row", alignItems: "center", gap: 10, ...shadow },
  snackText: { flex: 1, color: "#FFFFFF", fontSize: 16, fontFamily: "Inter_600SemiBold" },
  snackBtn: { minHeight: 44, paddingHorizontal: 14, justifyContent: "center", borderRadius: 10 },
  snackBtnText: { color: "#7EE0C3", fontSize: 16, fontFamily: "Inter_700Bold" },
  body: { fontSize: 17, fontFamily: "Inter_400Regular", color: colors.muted },
  empty: { alignItems: "stretch", gap: 14, backgroundColor: colors.card, borderRadius: radius.card, padding: 24, ...shadow },
  emptyIcon: { width: 88, height: 88, borderRadius: 24, backgroundColor: colors.accentSoft, alignItems: "center", justifyContent: "center", alignSelf: "center" },
  hello: { fontSize: 26, fontFamily: fonts.title, color: colors.text, textAlign: "center", letterSpacing: -0.5 },
  emptyText: { fontSize: 17, fontFamily: "Inter_400Regular", color: colors.muted, textAlign: "center" },
});
