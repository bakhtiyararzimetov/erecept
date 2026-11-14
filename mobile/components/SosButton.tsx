import Ionicons from "@expo/vector-icons/Ionicons";
import { Alert, Linking, Pressable, StyleSheet, Text, View } from "react-native";
import { api } from "../lib/api";
import { colors, radius, sizes } from "../lib/theme";
import { t, tf } from "../lib/i18n";

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
    Alert.alert(t("Не удалось позвонить"), t("Наберите 103 вручную"));
  });
}

export function SosButton() {
  return (
    <View style={styles.wrap}>
      <Pressable
        accessibilityRole="button"
        style={({ pressed }) => [styles.btn, pressed && { opacity: 0.7 }]}
        onPress={() =>
          Alert.alert(t("Позвонить в скорую 103?"), undefined, [
            { text: t("Отмена"), style: "cancel" },
            { text: t("Позвонить"), onPress: call },
          ])
        }
      >
        <Ionicons name="call" size={20} color={colors.error} />
        <Text style={styles.text}>{t("Мне плохо")}</Text>
      </Pressable>
      <Text style={styles.hint}>{t("112 — единая служба · 103 — скорая помощь")}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { gap: 6 },
  btn: {
    minHeight: sizes.button,
    borderRadius: radius.control,
    backgroundColor: colors.errorSoft,
    flexDirection: "row",
    gap: 8,
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: 16,
  },
  text: { fontSize: sizes.text, fontFamily: "Inter_600SemiBold", color: colors.error },
  hint: { fontSize: 14, fontFamily: "Inter_400Regular", color: colors.muted, textAlign: "center" },
});
