import React from "react";
import {
  Modal,
  View,
  Text,
  TouchableOpacity,
  TouchableWithoutFeedback,
} from "react-native";

interface Props {
  visible: boolean;
  title: string;
  body: string;
  confirmLabel: string;
  onCancel: () => void;
  onConfirm: () => void;
  busy?: boolean;
  dark?: boolean;
}

export function ConfirmSheet({
  visible,
  title,
  body,
  confirmLabel,
  onCancel,
  onConfirm,
  busy = false,
  dark = false,
}: Props) {
  return (
    <Modal
      visible={visible}
      transparent
      animationType="slide"
      onRequestClose={onCancel}
    >
      <TouchableWithoutFeedback onPress={onCancel}>
        <View className="flex-1 justify-end bg-black/50">
          <View
            className={`rounded-t-2xl p-6 pb-10 ${
              dark ? "bg-panel" : "bg-white"
            }`}
          >
              <Text
                className={`font-head font-bold text-lg mb-2 ${
                  dark ? "text-bone" : "text-ink"
                }`}
              >
                {title}
              </Text>
              <Text
                className={`text-[13px] mb-6 ${dark ? "text-fog" : "text-muted"}`}
              >
                {body}
              </Text>
              <TouchableOpacity
                onPress={onConfirm}
                disabled={busy}
                className="border border-brick rounded-input py-3 items-center mb-3"
                accessibilityRole="button"
                accessibilityLabel={confirmLabel}
              >
                <Text className="text-brick font-medium text-[13px]">
                  {busy ? "Working…" : confirmLabel}
                </Text>
              </TouchableOpacity>
              <TouchableOpacity
                onPress={onCancel}
                disabled={busy}
                className={`rounded-input py-3 items-center border ${
                  dark ? "border-edge" : "border-line"
                }`}
                accessibilityRole="button"
                accessibilityLabel="Cancel"
              >
                <Text
                  className={`font-medium text-[13px] ${
                    dark ? "text-bone" : "text-ink"
                  }`}
                >
                  Cancel
                </Text>
              </TouchableOpacity>
          </View>
        </View>
      </TouchableWithoutFeedback>
    </Modal>
  );
}
