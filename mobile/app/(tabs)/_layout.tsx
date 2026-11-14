import Ionicons from "@expo/vector-icons/Ionicons";
import { Tabs } from "expo-router";
import { Text, View } from "react-native";
import { colors } from "../../lib/theme";
import { t } from "../../lib/i18n";
import { LangSwitch } from "../../components/LangSwitch";

export default function TabsLayout() {
  return (
    <Tabs
      screenOptions={{
        // Шапка: название приложения и переключатель языка RU · EN
        headerShown: true,
        headerTitle: () => (
          <View style={{ flexDirection: "row", alignItems: "center", gap: 10 }}>
            <View style={{ width: 28, height: 28, borderRadius: 8, backgroundColor: colors.accent, alignItems: "center", justifyContent: "center" }}><Ionicons name="add" size={20} color="#FFFFFF" /></View>
            <Text style={{ fontFamily: "Manrope_800ExtraBold", fontSize: 20, letterSpacing: -0.4, color: colors.text }}>{t("еРецепт")}</Text>
          </View>
        ),
        headerStyle: { backgroundColor: colors.card },
        headerShadowVisible: false,
        headerRight: () => <LangSwitch />,
        tabBarActiveTintColor: colors.accent,
        tabBarInactiveTintColor: colors.muted,
        tabBarLabelStyle: { fontSize: 13, fontFamily: "Inter_600SemiBold" },
        tabBarLabelPosition: "below-icon",
        tabBarStyle: { height: 72, paddingTop: 8, paddingBottom: 10, backgroundColor: colors.card, borderTopColor: colors.border, borderTopWidth: 1 },
        tabBarItemStyle: { justifyContent: "center", paddingHorizontal: 0 },
        sceneStyle: { backgroundColor: colors.bg },
      }}
    >
      <Tabs.Screen name="index" options={{ title: t("Сегодня"), tabBarIcon: ({ focused, color }) => <Ionicons name={focused ? "today" : "today-outline"} size={24} color={color as string} /> }} />
      <Tabs.Screen name="course" options={{ title: t("Курс"), tabBarIcon: ({ focused, color }) => <Ionicons name={focused ? "stats-chart" : "stats-chart-outline"} size={24} color={color as string} /> }} />
      <Tabs.Screen name="shops" options={{ title: t("Аптеки"), tabBarIcon: ({ focused, color }) => <Ionicons name={focused ? "medkit" : "medkit-outline"} size={24} color={color as string} /> }} />
      <Tabs.Screen name="settings" options={{ title: t("Настройки"), tabBarIcon: ({ focused, color }) => <Ionicons name={focused ? "settings" : "settings-outline"} size={24} color={color as string} /> }} />
    </Tabs>
  );
}
