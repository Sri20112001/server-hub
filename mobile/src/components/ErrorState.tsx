import React from "react";
import { View, Text, TouchableOpacity } from "react-native";
import { CircleAlert } from "lucide-react-native";
import { colors } from "../theme/colors";

interface Props {
  message: string;
  onRetry?: () => void;
  dark?: boolean;
}

export function ErrorState({ message, onRetry, dark = false }: Props) {
  return (
    <View
      className={`rounded-xl border px-4 py-8 items-center justify-center ${
        dark ? "bg-abyss border-edge" : "bg-paper border-line"
      }`}
    >
      <CircleAlert size={28} color={colors.brick} style={{ marginBottom: 8 }} />
      <Text
        className={`font-head font-bold text-base text-center mb-1 ${
          dark ? "text-bone" : "text-ink"
        }`}
      >
        Unable to connect
      </Text>
      <Text
        className={`text-[13px] text-center mb-4 ${dark ? "text-fog" : "text-muted"}`}
      >
        {message}
      </Text>
      {onRetry && (
        <TouchableOpacity
          onPress={onRetry}
          className="bg-accent rounded-input px-4 py-2"
          accessibilityLabel="Retry"
          accessibilityRole="button"
        >
          <Text className="text-white text-[13px] font-medium">Retry</Text>
        </TouchableOpacity>
      )}
    </View>
  );
}
