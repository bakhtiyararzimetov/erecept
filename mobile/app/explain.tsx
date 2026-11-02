import { useLocalSearchParams, useRouter } from "expo-router";
import { useEffect, useState } from "react";
import { ScrollView, StyleSheet, Text, View } from "react-native";
import { Button } from "../components/Button";
import { Card } from "../components/Card";
import { loadCourses } from "../lib/storage";
import { colors, radius } from "../lib/theme";
import type { Item } from "../lib/types";

function List({ items }: { items: string[] }) {
  return (
    <View style={{ gap: 4 }}>
      {items.map((t, i) => (
        <Text key={i} style={styles.text}>
          {"• "}
          {t}
        </Text>
      ))}
    </View>
  );
}

function Block({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <View style={{ gap: 4 }}>
      <Text style={styles.sub}>{title}</Text>
      {children}
    </View>
  );
}

export default function Explain() {
  const router = useRouter();
  const { itemId } = useLocalSearchParams<{ itemId: string }>();
  const [item, setItem] = useState<Item | null | undefined>(undefined);

  useEffect(() => {
    loadCourses()
      .then((cs) => {
        const id = Number(itemId);
        setItem(cs.flatMap((c) => c.prescription.items).find((i) => i.id === id) ?? null);
      })
      .catch(() => setItem(null));
  }, [itemId]);

  if (item === undefined) return <View style={styles.screen} />;
  const e = item?.explanation ?? null;

  return (
    <ScrollView style={styles.screen} contentContainerStyle={styles.content}>
      {!item || !e ? (
        <Card style={{ gap: 12 }}>
          <Text style={styles.title}>Памятки нет</Text>
          <Text style={styles.text}>Для этого лекарства краткой памятки пока нет</Text>
        </Card>
      ) : (
        <Card style={{ gap: 16 }}>
          <Text style={styles.title}>{item.medicine_name}</Text>
          <Block title="Зачем">
            <Text style={styles.text}>{e.purpose}</Text>
          </Block>
          <Block title="Как принимать">
            <Text style={styles.text}>{e.how_to_take}</Text>
          </Block>
          {e.do_not_take_if?.length > 0 && (
            <Block title="Кому нельзя">
              <List items={e.do_not_take_if} />
            </Block>
          )}
          <Block title="Если пропустили приём">
            <View style={e.high_risk ? styles.risk : undefined}>
              <Text style={styles.text}>{e.missed_dose}</Text>
            </View>
          </Block>
          <Block title="Если бросить раньше">
            <Text style={styles.text}>{e.if_stopped_early}</Text>
          </Block>
          {e.urgent_signs?.length > 0 && (
            <Block title="Срочно к врачу">
              <List items={e.urgent_signs} />
            </Block>
          )}
          <Text style={styles.small}>
            Сгенерировано ИИ по инструкции НЦЭЛС{e.source?.reg_number ? `, рег. № ${e.source.reg_number}` : ""}. Не заменяет совет врача
          </Text>
        </Card>
      )}
      <Button title="Закрыть" variant="secondary" onPress={() => router.back()} />
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg },
  content: { padding: 16, paddingTop: 32, gap: 16 },
  title: { fontSize: 24, fontFamily: "Inter_700Bold", color: colors.text },
  sub: { fontSize: 17, fontFamily: "Inter_600SemiBold", color: colors.text },
  text: { fontSize: 17, fontFamily: "Inter_400Regular", color: colors.text },
  small: { fontSize: 14, fontFamily: "Inter_400Regular", color: colors.muted },
  risk: { borderWidth: 2, borderColor: colors.error, borderRadius: radius.control, padding: 12 },
});
