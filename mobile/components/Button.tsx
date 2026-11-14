import { Pressable, StyleSheet, Text } from "react-native";
import { colors, radius, sizes } from "../lib/theme";

type Props = {
  title: string;
  onPress: () => void;
  variant?: "primary" | "secondary";
  disabled?: boolean;
};

export function Button({ title, onPress, variant = "primary", disabled }: Props) {
  const primary = variant === "primary";
  return (
    <Pressable
      accessibilityRole="button"
      onPress={onPress}
      disabled={disabled}
      style={({ pressed }) => [
        styles.base,
        primary ? styles.primary : styles.secondary,
        (pressed || disabled) && { opacity: 0.7 },
      ]}
    >
      <Text style={[styles.text, { color: primary ? "#FFFFFF" : colors.accentDark }]}>{title}</Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  base: {
    minHeight: sizes.button,
    borderRadius: radius.control,
    paddingHorizontal: 16,
    alignItems: "center",
    justifyContent: "center",
  },
  primary: { backgroundColor: colors.accent, boxShadow: "0px 6px 14px -6px rgba(15, 118, 110, 0.6)" },
  secondary: { backgroundColor: colors.accentSoft },
  text: { fontSize: sizes.text, fontFamily: "Inter_600SemiBold" },
});
