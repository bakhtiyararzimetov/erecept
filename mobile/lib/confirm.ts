import { Alert, Platform } from "react-native";

// В браузере (react-native-web) Alert.alert — пустая функция, поэтому там — window.confirm/alert.
export function confirm(title: string, okText: string, cancelText: string, message?: string): Promise<boolean> {
  if (Platform.OS === "web") {
    return Promise.resolve(window.confirm(message ? `${title}\n\n${message}` : title));
  }
  return new Promise((resolve) =>
    Alert.alert(title, message, [
      { text: cancelText, style: "cancel", onPress: () => resolve(false) },
      { text: okText, onPress: () => resolve(true) },
    ], { cancelable: true, onDismiss: () => resolve(false) }),
  );
}

export function notify(title: string, message?: string): void {
  if (Platform.OS === "web") window.alert(message ? `${title}\n\n${message}` : title);
  else Alert.alert(title, message);
}
