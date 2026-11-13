import { Tabs } from "expo-router";
import { colors } from "../../lib/theme";
import { t } from "../../lib/i18n";
import { LangSwitch } from "../../components/LangSwitch";

export default function TabsLayout() {
  return (
    <Tabs
      screenOptions={{
        // Шапка: название приложения и переключатель языка RU · EN
        headerShown: true,
        headerTitle: t("еРецепт"),
        headerTitleStyle: { fontFamily: "Inter_700Bold", fontSize: 20, color: colors.text },
        headerStyle: { backgroundColor: colors.card },
        headerShadowVisible: false,
        headerRight: () => <LangSwitch />,
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
      <Tabs.Screen name="index" options={{ title: t("Сегодня") }} />
      <Tabs.Screen name="course" options={{ title: t("Курс") }} />
      <Tabs.Screen name="shops" options={{ title: t("Аптеки") }} />
      <Tabs.Screen name="settings" options={{ title: t("Настройки") }} />
    </Tabs>
  );
}
