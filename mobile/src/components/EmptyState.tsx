import React from "react";
import { View, Text } from "react-native";

interface Props {
  title: string;
  hint?: string;
  dark?: boolean;
}

export function EmptyState({ title, hint, dark = false }: Props) {
  return (
    <View
      className={`rounded-xl px-4 py-10 items-center justify-center ${
        dark ? "bg-abyss" : "bg-paper"
      }`}
    >
      <Text
        className={`font-head font-bold text-base text-center mb-1 ${
          dark ? "text-bone" : "text-ink"
        }`}
      >
        {title}
      </Text>
      {hint && (
        <Text
          className={`text-[13px] text-center ${dark ? "text-fog" : "text-muted"}`}
        >
          {hint}
        </Text>
      )}
    </View>
  );
}
