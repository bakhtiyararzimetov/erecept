import * as Location from "expo-location";
import { useFocusEffect, useLocalSearchParams } from "expo-router";
import { useCallback, useEffect, useRef, useState } from "react";
import { Image, Linking, Pressable, ScrollView, StyleSheet, Text, TextInput, View } from "react-native";
import { Button } from "../../components/Button";
import { Card } from "../../components/Card";
import { API_URL, ApiError, api } from "../../lib/api";
import { formatKm, formatPrice, openStatus } from "../../lib/format";
import { loadCourses } from "../../lib/storage";
import { colors, radius, sizes } from "../../lib/theme";
import type { OffersResponse } from "../../lib/types";
import { locale, t, tf } from "../../lib/i18n";

type Sort = "price" | "distance";
type Coords = { lat: number; lng: number };

export default function Shops() {
  const params = useLocalSearchParams<{ name?: string }>();
  const [names, setNames] = useState<string[]>([]);
  const [name, setName] = useState<string>("");
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState<Sort>("price");
  const [coords, setCoords] = useState<Coords | null>(null);
  const [denied, setDenied] = useState(false);
  const [data, setData] = useState<OffersResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [openOnly, setOpenOnly] = useState(false);
  const reqId = useRef(0);

  useFocusEffect(
    useCallback(() => {
      loadCourses().then((cs) => {
        const uniq = Array.from(new Set(cs.flatMap((c) => c.prescription.items.map((i) => i.medicine_name))));
        setNames(uniq);
      });
    }, []),
  );

  useEffect(() => {
    if (params.name) {
      setName(params.name);
      setQuery(params.name);
    }
  }, [params.name]);

  const load = useCallback(async () => {
    if (!name) return;
    const id = ++reqId.current;
    setData(null);
    setLoading(true);
    setError(null);
    try {
      const useDistance = sort === "distance" && coords;
      const res = await api.offers({
        name,
        sort: useDistance ? "distance" : "price",
        lat: useDistance ? coords.lat : undefined,
        lng: useDistance ? coords.lng : undefined,
      });
      if (id === reqId.current) setData(res);
    } catch (e) {
      if (id !== reqId.current) return;
      console.warn("shops: не удалось загрузить предложения", e);
      setData(null);
      setError(e instanceof ApiError ? e.message : t("Не удалось загрузить данные"));
    } finally {
      if (id === reqId.current) setLoading(false);
    }
  }, [name, sort, coords]);

  useEffect(() => {
    load();
  }, [load]);

  async function chooseSort(next: Sort) {
    if (next === "distance" && !coords) {
      try {
        const perm = await Location.requestForegroundPermissionsAsync();
        if (!perm.granted) throw new Error("denied");
        const pos = await Location.getCurrentPositionAsync({ accuracy: Location.Accuracy.Balanced });
        setCoords({ lat: pos.coords.latitude, lng: pos.coords.longitude });
        setDenied(false);
      } catch (e) {
        console.warn("shops: геолокация недоступна", e);
        setDenied(true);
        setSort("price");
        return;
      }
    }
    setSort(next);
  }

  function search() {
    const q = query.trim();
    if (q) setName(q);
  }

  const now = new Date();
  const offers = (data?.offers ?? []).filter((o) => !openOnly || openStatus(o.pharmacy.work_hours, now) !== "closed");
  const minPrice = data && data.offers.length > 0 ? Math.min(...data.offers.map((o) => o.price)) : null;

  return (
    <ScrollView style={styles.screen} contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled">
      <Text style={styles.title}>{t("Где купить")}</Text>

      {names.length > 0 && (
        <View style={styles.chips}>
          {names.map((n) => {
            const active = n === name;
            return (
              <Pressable
                key={n}
                accessibilityRole="button"
                onPress={() => {
                  setName(n);
                  setQuery(n);
                }}
                style={[styles.chip, active && styles.chipActive]}
              >
                <Text style={[styles.chipText, active && { color: colors.card }]}>{n}</Text>
              </Pressable>
            );
          })}
        </View>
      )}

      <View style={styles.searchRow}>
        <TextInput
          value={query}
          onChangeText={setQuery}
          onSubmitEditing={search}
          returnKeyType="search"
          placeholder={t("Название лекарства")}
          placeholderTextColor={colors.muted}
          style={[styles.input, { flex: 1 }]}
        />
        <Pressable accessibilityRole="button" onPress={search} style={styles.searchBtn}>
          <Text style={[styles.chipText, { color: colors.card }]}>{t("Найти")}</Text>
        </Pressable>
      </View>

      {!name && <Text style={styles.body}>{t("Выберите лекарство или введите название")}</Text>}
      {loading && <Text style={styles.body}>{t("Загрузка...")}</Text>}
      {error && (
        <Card style={{ gap: 12 }}>
          <Text style={styles.errorText}>{error}</Text>
          <Button title={t("Повторить")} onPress={load} />
        </Card>
      )}

      {data && !loading && data.medicine === null && (
        <Card>
          <Text style={styles.body}>{t("Такое лекарство не найдено. Проверьте название")}</Text>
        </Card>
      )}

      {data && data.medicine && !loading && (
        <Card style={styles.medCard}>
          {data.medicine.image ? <Image source={{ uri: API_URL + data.medicine.image }} style={styles.packBig} accessibilityLabel={data.medicine.name} /> : null}
          <Text style={styles.medName}>{data.medicine.name}</Text>
          <Text style={styles.body}>{[data.medicine.active_substance, data.medicine.form].filter(Boolean).join(" · ")}</Text>
          {minPrice !== null && (
            <View style={styles.summary}>
              <Text style={styles.summaryPrice}>{tf("от {0}", formatPrice(minPrice))}</Text>
              <Text style={styles.summaryText}>{tf("в {0} аптеках", data.offers.length)}</Text>
            </View>
          )}
          <Text style={styles.illustration}>{t("Фото упаковки — иллюстрация")}</Text>
        </Card>
      )}

      {data && data.medicine && !loading && data.offers.length > 0 && (
        <View style={styles.filters}>
          {(["price", "distance"] as Sort[]).map((sv) => {
            const active = sort === sv;
            return (
              <Pressable key={sv} accessibilityRole="button" onPress={() => chooseSort(sv)} style={[styles.filter, active && styles.filterActive]}>
                <Text style={[styles.filterText, active && { color: colors.card }]}>{sv === "price" ? t("Дешевле") : t("Ближе")}</Text>
              </Pressable>
            );
          })}
          <Pressable accessibilityRole="button" onPress={() => setOpenOnly(!openOnly)} style={[styles.filter, openOnly && styles.filterActive]}>
            <Text style={[styles.filterText, openOnly && { color: colors.card }]}>{t("Открыто сейчас")}</Text>
          </Pressable>
        </View>
      )}
      {denied && (
        <View style={styles.notice}>
          <Text style={styles.noticeText}>{t("Разрешите геолокацию, чтобы видеть ближайшие")}</Text>
        </View>
      )}

      {data && data.medicine && data.offers.length === 0 && !loading && (
        <Card>
          <Text style={styles.body}>{t("Сейчас нет в наличии ни в одной аптеке")}</Text>
        </Card>
      )}
      {!loading &&
        offers.map((o) => {
          const st = openStatus(o.pharmacy.work_hours, now);
          const cheapest = o.price === minPrice;
          return (
            <Card key={o.pharmacy.id} style={cheapest ? { ...styles.row, ...styles.rowBest } : styles.row}>
              <View style={styles.rowTop}>
                <View style={{ flex: 1, gap: 4 }}>
                  <Text style={styles.pharmacy}>{o.pharmacy.name}</Text>
                  <View style={styles.tags}>
                    {st && <Text style={[styles.tag, st === "open" ? styles.tagOpen : styles.tagClosed]}>{st === "open" ? t("Открыто") : t("Закрыто")}</Text>}
                    {cheapest && <Text style={[styles.tag, styles.tagBest]}>{t("Дешевле всего")}</Text>}
                  </View>
                </View>
                <Text style={styles.price}>{formatPrice(o.price)}</Text>
              </View>
              <Text style={styles.small}>{o.pharmacy.address}</Text>
              <Text style={styles.small}>
                {[o.pharmacy.work_hours, o.distance_km !== null ? formatKm(o.distance_km) : null].filter(Boolean).join(" · ")}
              </Text>
              <View style={styles.actions}>
                <Pressable
                  accessibilityRole="button"
                  style={styles.action}
                  onPress={() => Linking.openURL(`https://maps.google.com/?q=${o.pharmacy.lat},${o.pharmacy.lng}`).catch(() => undefined)}
                >
                  <Text style={styles.actionText}>{t("Маршрут")}</Text>
                </Pressable>
                {!!o.pharmacy.phone && (
                  <Pressable
                    accessibilityRole="button"
                    style={styles.action}
                    onPress={() => Linking.openURL(`tel:${o.pharmacy.phone.replace(/[^\d+]/g, "")}`).catch(() => undefined)}
                  >
                    <Text style={styles.actionText}>{t("Позвонить")}</Text>
                  </Pressable>
                )}
              </View>
            </Card>
          );
        })}

      {data && data.analogs.length > 0 && !loading && (
        <>
          <Text style={styles.title}>{t("Аналоги")}</Text>
          <View style={styles.notice}>
            <Text style={styles.noticeText}>{data.analog_note}</Text>
          </View>
          {data.analogs.map((a) => (
            <Pressable
              key={a.medicine.id}
              accessibilityRole="button"
              onPress={() => {
                setName(a.medicine.name);
                setQuery(a.medicine.name);
              }}
            >
              <Card style={styles.analog}>
                {a.medicine.image ? <Image source={{ uri: API_URL + a.medicine.image }} style={styles.packSmall} accessibilityLabel={a.medicine.name} /> : null}
                <View style={{ flex: 1, gap: 2 }}>
                  <Text style={styles.pharmacy}>{a.medicine.name}</Text>
                  <Text style={styles.small}>{[a.medicine.active_substance, a.medicine.form].filter(Boolean).join(" · ")}</Text>
                  <Text style={styles.analogPrice}>{tf("от {0}, аптек: {1}", formatPrice(a.min_price), a.pharmacies_count)}</Text>
                </View>
              </Card>
            </Pressable>
          ))}
        </>
      )}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  packBig: { width: "100%", aspectRatio: 600 / 380, borderRadius: 12 },
  packSmall: { width: 96, height: 61, borderRadius: 8 },
  illustration: { fontSize: 13, color: colors.muted },
  medCard: { gap: 6 },
  medName: { fontSize: 26, fontFamily: "Inter_700Bold", color: colors.text, marginTop: 4 },
  summary: { flexDirection: "row", alignItems: "baseline", gap: 10, marginTop: 6 },
  summaryPrice: { fontSize: 24, fontFamily: "Inter_700Bold", color: colors.accent },
  summaryText: { fontSize: 16, fontFamily: "Inter_400Regular", color: colors.muted },
  searchRow: { flexDirection: "row", gap: 8 },
  searchBtn: { minHeight: 56, paddingHorizontal: 18, borderRadius: radius.control, backgroundColor: colors.accent, justifyContent: "center" },
  filters: { flexDirection: "row", flexWrap: "wrap", gap: 8 },
  filter: { paddingHorizontal: 14, paddingVertical: 10, borderRadius: 999, borderWidth: 1, borderColor: colors.border, backgroundColor: colors.card },
  filterActive: { backgroundColor: colors.accent, borderColor: colors.accent },
  filterText: { fontSize: 15, fontFamily: "Inter_600SemiBold", color: colors.text },
  row: { gap: 6 },
  rowBest: { borderColor: colors.accent, borderWidth: 2 },
  rowTop: { flexDirection: "row", alignItems: "flex-start", gap: 12 },
  pharmacy: { fontSize: 18, fontFamily: "Inter_600SemiBold", color: colors.text },
  tags: { flexDirection: "row", gap: 6, flexWrap: "wrap" },
  tag: { fontSize: 13, fontFamily: "Inter_600SemiBold", paddingHorizontal: 8, paddingVertical: 3, borderRadius: 6, overflow: "hidden" },
  tagOpen: { color: "#166534", backgroundColor: "#DCFCE7" },
  tagClosed: { color: colors.muted, backgroundColor: colors.border },
  tagBest: { color: colors.card, backgroundColor: colors.accent },
  small: { fontSize: 15, fontFamily: "Inter_400Regular", color: colors.muted },
  actions: { flexDirection: "row", gap: 8, marginTop: 6 },
  action: { flex: 1, minHeight: 44, borderRadius: radius.control, borderWidth: 1, borderColor: colors.border, alignItems: "center", justifyContent: "center" },
  actionText: { fontSize: 15, fontFamily: "Inter_600SemiBold", color: colors.accent },
  analog: { flexDirection: "row", alignItems: "center", gap: 12 },
  analogPrice: { fontSize: 15, fontFamily: "Inter_600SemiBold", color: colors.text },
  screen: { flex: 1, backgroundColor: colors.bg },
  content: { padding: 16, paddingTop: 16, gap: 14 },
  title: { fontSize: 24, fontFamily: "Inter_700Bold", color: colors.text },
  name: { fontSize: 20, fontFamily: "Inter_600SemiBold", color: colors.text },
  price: { fontSize: 24, fontFamily: "Inter_700Bold", color: colors.text },
  body: { fontSize: 17, fontFamily: "Inter_400Regular", color: colors.muted },
  text: { fontSize: 17, fontFamily: "Inter_600SemiBold", color: colors.text },
  errorText: { fontSize: 17, fontFamily: "Inter_600SemiBold", color: colors.error },
  chips: { flexDirection: "row", flexWrap: "wrap", gap: 8 },
  chip: {
    minHeight: sizes.button, justifyContent: "center", paddingHorizontal: 16, borderRadius: radius.control,
    borderWidth: 1, borderColor: colors.border, backgroundColor: colors.card,
  },
  chipActive: { backgroundColor: colors.accent, borderColor: colors.accent },
  chipText: { fontSize: 17, fontFamily: "Inter_600SemiBold", color: colors.text },
  input: {
    minHeight: 56, borderWidth: 1, borderColor: colors.border, borderRadius: radius.control,
    backgroundColor: colors.card, paddingHorizontal: 16, fontSize: 17, fontFamily: "Inter_400Regular", color: colors.text,
  },
  segments: { flexDirection: "row", gap: 8 },
  segment: {
    flex: 1, minHeight: sizes.button, alignItems: "center", justifyContent: "center", borderRadius: radius.control,
    borderWidth: 1, borderColor: colors.border, backgroundColor: colors.card,
  },
  segmentActive: { backgroundColor: colors.accent, borderColor: colors.accent },
  notice: { backgroundColor: colors.border, borderRadius: radius.control, padding: 12 },
  noticeText: { fontSize: 17, fontFamily: "Inter_600SemiBold", color: colors.text },
});
