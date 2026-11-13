import { Pressable, StyleSheet, Text, View } from "react-native";
import { getLang, setLang, type Lang } from "../lib/i18n";
import { colors } from "../lib/theme";

// Переключатель языка вверху экрана: RU · EN. Смена перерисовывает всё приложение (_layout).
export function LangSwitch() {
  const cur = getLang();
  const item = (l: Lang, label: string) => (
    <Pressable
      key={l}
      accessibilityRole="button"
      accessibilityState={{ selected: cur === l }}
      onPress={() => cur !== l && setLang(l)}
      style={[styles.item, cur === l && styles.active]}
    >
      <Text style={[styles.text, cur === l && styles.activeText]}>{label}</Text>
    </Pressable>
  );
  return <View style={styles.box}>{[item("ru", "RU"), item("en", "EN")]}</View>;
}

const styles = StyleSheet.create({
  box: { flexDirection: "row", borderWidth: 1, borderColor: colors.border, borderRadius: 20, overflow: "hidden", marginRight: 12 },
  item: { paddingHorizontal: 14, paddingVertical: 6, backgroundColor: colors.card },
  active: { backgroundColor: colors.accent },
  text: { fontSize: 15, fontFamily: "Inter_600SemiBold", color: colors.text },
  activeText: { color: colors.card },
});
