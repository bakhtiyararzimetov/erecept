import * as Notifications from "expo-notifications";
import { useFocusEffect, useRouter } from "expo-router";
import { useCallback, useState } from "react";
import { ScrollView, Share, StyleSheet, Text, View } from "react-native";
import { Button } from "../../components/Button";
import { Card } from "../../components/Card";
import { API_URL, ApiError, api } from "../../lib/api";
import { pushProjectId, registerPush } from "../../lib/push";
import { colors } from "../../lib/theme";

export default function Settings() {
  const router = useRouter();
  const [code, setCode] = useState<string | null>(null);
  const [inviteError, setInviteError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [notif, setNotif] = useState<string>("");
  const pushAvailable = !!pushProjectId();

  useFocusEffect(
    useCallback(() => {
      Notifications.getPermissionsAsync()
        .then((p) => setNotif(p.granted ? "Уведомления включены" : "Уведомления выключены. Включите их в настройках телефона"))
        .catch(() => setNotif("Состояние уведомлений неизвестно"));
      registerPush().catch(() => undefined);
    }, []),
  );

  async function invite() {
    setBusy(true);
    setInviteError(null);
    try {
      const r = await api.createInvite();
      setCode(r.code);
    } catch (e) {
      console.warn("settings: приглашение", e);
      setInviteError(e instanceof ApiError ? e.message : "Не удалось создать код");
    } finally {
      setBusy(false);
    }
  }

  return (
    <ScrollView style={styles.screen} contentContainerStyle={styles.content}>
      <Text style={styles.title}>Настройки</Text>

      <Card style={{ gap: 12 }}>
        <Text style={styles.name}>Родственник</Text>
        <Text style={styles.body}>Родственник увидит, пьёте ли вы лекарства вовремя</Text>
        <Button title="Пригласить родственника" onPress={invite} disabled={busy} />
        {inviteError && (
          <>
            <Text style={styles.error}>{inviteError}</Text>
            <Button title="Повторить" variant="secondary" onPress={invite} />
          </>
        )}
        {code && (
          <View style={{ gap: 8 }}>
            <Text style={styles.code} accessibilityLabel={`Код ${code.split("").join(" ")}`}>
              {code}
            </Text>
            <Text style={styles.body}>Действует 24 часа</Text>
            <Button
              title="Поделиться"
              variant="secondary"
              onPress={() => Share.share({ message: `Код для приложения еРецепт: ${code}` }).catch(() => undefined)}
            />
          </View>
        )}
      </Card>

      <Card style={{ gap: 12 }}>
        <Text style={styles.name}>Я родственник</Text>
        <Button title="Я родственник — ввести код" variant="secondary" onPress={() => router.push("/join")} />
      </Card>

      <Card style={{ gap: 8 }}>
        <Text style={styles.name}>Уведомления</Text>
        <Text style={styles.body}>{notif}</Text>
        {!pushAvailable && <Text style={styles.body}>Push недоступен: нужна сборка с EAS (см. README)</Text>}
      </Card>

      <Text style={styles.small}>Сервер: {API_URL}</Text>
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg },
  content: { padding: 16, paddingTop: 56, gap: 16 },
  title: { fontSize: 24, fontFamily: "Inter_700Bold", color: colors.text },
  name: { fontSize: 20, fontFamily: "Inter_600SemiBold", color: colors.text },
  body: { fontSize: 17, fontFamily: "Inter_400Regular", color: colors.muted },
  error: { fontSize: 17, fontFamily: "Inter_600SemiBold", color: colors.error },
  code: { fontSize: 36, fontFamily: "Inter_700Bold", color: colors.text, letterSpacing: 8 },
  small: { fontSize: 17, fontFamily: "Inter_400Regular", color: colors.muted },
});
