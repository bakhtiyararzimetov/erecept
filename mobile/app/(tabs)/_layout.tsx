import { Tabs } from "expo-router";
import { colors } from "../../lib/theme";

export default function TabsLayout() {
  return (
    <Tabs
      screenOptions={{
        headerShown: false,
        tabBarActiveTintColor: colors.accent,
        tabBarInactiveTintColor: colors.muted,
        tabBarLabelStyle: { fontSize: 17, fontFamily: "Inter_400Regular", paddingHorizontal: 0, marginHorizontal: 0 },
        tabBarIconStyle: { display: "none" },
        tabBarLabelPosition: "beside-icon",
        tabBarStyle: { height: 64, backgroundColor: colors.card, borderTopColor: colors.border, borderTopWidth: 1 },
        tabBarItemStyle: { justifyContent: "center", paddingHorizontal: 0 },
        sceneStyle: { backgroundColor: colors.bg },
      }}
    >
      <Tabs.Screen name="index" options={{ title: "Сегодня" }} />
      <Tabs.Screen name="course" options={{ title: "Курс" }} />
      <Tabs.Screen name="shops" options={{ title: "Аптеки" }} />
      <Tabs.Screen name="settings" options={{ title: "Настройки" }} />
    </Tabs>
  );
}
