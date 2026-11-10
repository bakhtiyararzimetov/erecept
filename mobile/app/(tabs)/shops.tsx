import * as Location from "expo-location";
import { useFocusEffect, useLocalSearchParams } from "expo-router";
import { useCallback, useEffect, useRef, useState } from "react";
import { Linking, Pressable, ScrollView, StyleSheet, Text, TextInput, View } from "react-native";
import { Button } from "../../components/Button";
import { Card } from "../../components/Card";
import { ApiError, api } from "../../lib/api";
import { formatKm, formatPrice } from "../../lib/format";
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

      <TextInput
        value={query}
        onChangeText={setQuery}
        onSubmitEditing={search}
        returnKeyType="search"
        placeholder={t("Название лекарства")}
        placeholderTextColor={colors.muted}
        style={styles.input}
      />
      <Button title={t("Найти")} variant="secondary" onPress={search} />

      <View style={styles.segments}>
        {(["price", "distance"] as Sort[]).map((s) => {
          const active = sort === s;
          return (
            <Pressable
              key={s}
              accessibilityRole="button"
              onPress={() => chooseSort(s)}
              style={[styles.segment, active && styles.segmentActive]}
            >
              <Text style={[styles.chipText, active && { color: colors.card }]}>
                {s === "price" ? t("По цене") : t("По расстоянию")}
              </Text>
            </Pressable>
          );
        })}
      </View>
      {sort === "price" && !coords && !denied && (
        <Text style={styles.body}>{t("Для сортировки по расстоянию приложению нужна ваша геопозиция. Она нигде не сохраняется")}</Text>
      )}
      {denied && (
        <View style={styles.notice}>
          <Text style={styles.noticeText}>{t("Разрешите геолокацию, чтобы видеть ближайшие")}</Text>
        </View>
      )}

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
      {data && data.medicine && data.offers.length === 0 && !loading && (
        <Card>
          <Text style={styles.body}>{t("Сейчас нет в наличии ни в одной аптеке")}</Text>
        </Card>
      )}
      {data?.offers.map((o) => (
        <Card key={o.pharmacy.id} style={{ gap: 8 }}>
          <Text style={styles.name}>{o.pharmacy.name}</Text>
          <Text style={styles.price}>{formatPrice(o.price)}</Text>
          <Text style={styles.body}>{o.pharmacy.address}</Text>
          <Text style={styles.body}>{o.pharmacy.work_hours}</Text>
          {o.distance_km !== null && <Text style={styles.body}>{formatKm(o.distance_km)}</Text>}
          <Button
            title={t("На карте")}
            variant="secondary"
            onPress={() => Linking.openURL(`https://maps.google.com/?q=${o.pharmacy.lat},${o.pharmacy.lng}`).catch(() => undefined)}
          />
          {!!o.pharmacy.phone && (
            <Button
              title={t("Позвонить")}
              variant="secondary"
              onPress={() => Linking.openURL(`tel:${o.pharmacy.phone.replace(/[^\d+]/g, "")}`).catch(() => undefined)}
            />
          )}
        </Card>
      ))}

      {data && data.analogs.length > 0 && (
        <>
          <Text style={styles.title}>{t("Аналоги")}</Text>
          <View style={styles.notice}>
            <Text style={styles.noticeText}>{data.analog_note}</Text>
          </View>
          {data.analogs.map((a) => (
            <Card key={a.medicine.id} style={{ gap: 4 }}>
              <Text style={styles.name}>{a.medicine.name}</Text>
              <Text style={styles.body}>
                {[a.medicine.active_substance, a.medicine.form].filter(Boolean).join(", ")}
              </Text>
              <Text style={styles.text}>
                {tf("от {0}, аптек: {1}", formatPrice(a.min_price), a.pharmacies_count)}
              </Text>
            </Card>
          ))}
        </>
      )}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg },
  content: { padding: 16, paddingTop: 56, gap: 16 },
  title: { fontSize: 24, fontFamily: "Inter_700Bold", color: colors.text },
  name: { fontSize: 20, fontFamily: "Inter_600SemiBold", color: colors.text },
  price: { fontSize: 28, fontFamily: "Inter_700Bold", color: colors.text },
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
