import { useLocalSearchParams } from "expo-router";
import { Text, View } from "react-native";
import { colors } from "../../lib/theme";

export default function Screen() {
  const { name } = useLocalSearchParams<{ name?: string }>();
  return (
    <View style={{ flex: 1, padding: 16, paddingTop: 64, backgroundColor: colors.bg }}>
      <Text style={{ fontSize: 24, fontFamily: "Inter_700Bold", color: colors.text }}>Где купить</Text>
      <Text style={{ fontSize: 17, fontFamily: "Inter_400Regular", color: colors.muted, marginTop: 8 }}>
        Скоро здесь будет поиск лекарств по аптекам{name ? ` (${name})` : ""}
      </Text>
    </View>
  );
}
