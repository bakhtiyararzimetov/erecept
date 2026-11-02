import { Alert, Linking, Pressable, StyleSheet, Text, View } from "react-native";
import { api } from "../lib/api";
import { colors, radius, sizes } from "../lib/theme";

let lastSos = 0;

function call() {
  const now = Date.now();
  if (now - lastSos > 10_000) {
    lastSos = now; // защита от двойного нажатия: второй push не уходит
    api.sos().catch((e) => {
      lastSos = 0; // не дошло: повторное нажатие снова уведомит родственников
      console.warn("sos: не отправлено", e);
    }); // без сети звонок всё равно работает
  }
  Linking.openURL("tel:103").catch((e) => {
    console.warn("sos: не открылся набор номера", e);
    Alert.alert("Не удалось позвонить", "Наберите 103 вручную");
  });
}

export function SosButton() {
  return (
    <View style={styles.wrap}>
      <Pressable
        accessibilityRole="button"
        style={({ pressed }) => [styles.btn, pressed && { opacity: 0.7 }]}
        onPress={() =>
          Alert.alert("Позвонить в скорую 103?", undefined, [
            { text: "Отмена", style: "cancel" },
            { text: "Позвонить", onPress: call },
          ])
        }
      >
        <Text style={styles.text}>Мне плохо</Text>
      </Pressable>
      <Text style={styles.hint}>112 — единая служба · 9103 — скорая в Алматы</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { gap: 6 },
  btn: {
    minHeight: sizes.button,
    borderRadius: radius.control,
    borderWidth: 2,
    borderColor: colors.error,
    backgroundColor: colors.card,
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: 16,
  },
  text: { fontSize: sizes.text, fontFamily: "Inter_600SemiBold", color: colors.error },
  hint: { fontSize: 17, fontFamily: "Inter_400Regular", color: colors.muted, textAlign: "center" },
});
