import React, { useCallback, useState } from "react";
import {
  View,
  Text,
  ScrollView,
  TouchableOpacity,
  Switch,
  TextInput,
  RefreshControl,
  ActivityIndicator,
  Alert,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell, BellOff, FlaskConical, RotateCcw } from "lucide-react-native";
import { useAuthStore } from "../../src/stores/authStore";
import { notificationsApi } from "../../src/api/notifications";
import { colors } from "../../src/theme/colors";

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <View className="mb-4">
      <Text className="text-[10px] font-semibold uppercase tracking-widest text-fog px-4 pt-4 pb-2">
        {title}
      </Text>
      <View className="bg-panel border-y border-edge">{children}</View>
    </View>
  );
}

function Row({
  label,
  hint,
  right,
}: {
  label: string;
  hint?: string;
  right?: React.ReactNode;
}) {
  return (
    <View className="flex-row items-center justify-between px-4 py-3 border-b border-edge">
      <View className="flex-1 mr-3">
        <Text className="text-bone text-[14px]">{label}</Text>
        {hint ? <Text className="text-fog text-[12px] mt-0.5">{hint}</Text> : null}
      </View>
      {right}
    </View>
  );
}

function NumField({
  value,
  onChange,
  editable,
}: {
  value: string;
  onChange: (v: string) => void;
  editable: boolean;
}) {
  return (
    <TextInput
      className="bg-emboss border border-edge rounded-lg px-2 py-1.5 text-bone text-sm font-mono w-24 text-right"
      keyboardType="numeric"
      editable={editable}
      value={value}
      onChangeText={onChange}
    />
  );
}

const DEFAULTS = {
  cooldownCriticalSec: 900,
  cooldownWarningSec: 3600,
  cooldownInfoSec: 21600,
  repeatIntervalSec: 3600,
  maxRepeats: 3,
  emailPerHour: 60,
  tgPerHour: 60,
  digestIntervalMin: 60,
};

export default function NotificationSettingsScreen() {
  const qc = useQueryClient();
  const user = useAuthStore((s) => s.user);
  const isAdmin = user?.role === "admin";
  const [saving, setSaving] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);
  const [draft, setDraft] = useState<Record<string, string> | null>(null);

  const settingsQ = useQuery({
    queryKey: ["notify-settings"],
    queryFn: notificationsApi.settings,
  });
  const policyQ = useQuery({
    queryKey: ["notify-policy"],
    queryFn: notificationsApi.policy,
  });
  const deliveriesQ = useQuery({
    queryKey: ["notify-deliveries"],
    queryFn: () => notificationsApi.deliveries({ limit: 20 }),
    refetchInterval: 30_000,
  });

  const settings = settingsQ.data;
  const policy = policyQ.data;
  const failed = (deliveriesQ.data ?? []).filter((d) => d.status === "failed" || d.status === "deferred");

  const num = useCallback(
    (key: string, fallback: number) => {
      if (draft && draft[key] !== undefined) return draft[key];
      const v = (policy as unknown as Record<string, number> | undefined)?.[key];
      return String(v ?? fallback);
    },
    [draft, policy],
  );
  const setNum = (key: string) => (v: string) =>
    setDraft((d) => ({ ...(d ?? {}), [key]: v.replace(/[^0-9]/g, "") }));

  const savePolicy = async (patch: object) => {
    setSaving(true);
    setMsg(null);
    try {
      await notificationsApi.updatePolicy(patch);
      setDraft(null);
      await Promise.all([
        qc.invalidateQueries({ queryKey: ["notify-policy"] }),
        qc.invalidateQueries({ queryKey: ["notify-deliveries"] }),
      ]);
      setMsg("Policy saved.");
    } catch (e) {
      setMsg(e instanceof Error ? e.message : "Save failed.");
    } finally {
      setSaving(false);
    }
  };

  const saveDraft = () => {
    if (!draft) return;
    const patch: Record<string, number> = {};
    for (const [k, v] of Object.entries(draft)) {
      if (v !== "") patch[k] = parseInt(v, 10);
    }
    void savePolicy(patch);
  };

  const restoreDefaults = () => {
    Alert.alert("Restore safe defaults?", "Overwrites the current delivery policy.", [
      { text: "Cancel", style: "cancel" },
      {
        text: "Restore",
        style: "destructive",
        onPress: () => void savePolicy({ ...DEFAULTS, notifyOnRecovery: true }),
      },
    ]);
  };

  const sendTest = async () => {
    setSaving(true);
    setMsg(null);
    try {
      const res = await notificationsApi.test();
      setMsg(`Test: telegram=${res.telegram} email=${res.email}`);
      void qc.invalidateQueries({ queryKey: ["notify-deliveries"] });
    } catch (e) {
      setMsg(e instanceof Error ? e.message : "Test failed.");
    } finally {
      setSaving(false);
    }
  };

  const toggleChannel = (which: "email" | "telegram", value: boolean) => {
    if (!isAdmin) return;
    setSaving(true);
    notificationsApi
      .updateSettings({ [which]: { enabled: value } })
      .then(() => qc.invalidateQueries({ queryKey: ["notify-settings"] }))
      .catch((e: unknown) => setMsg(e instanceof Error ? e.message : "Save failed."))
      .finally(() => setSaving(false));
  };

  const loading = settingsQ.isLoading || policyQ.isLoading;
  const error = settingsQ.error ?? policyQ.error;

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: colors.abyss }}>
      <View className="flex-row items-center gap-2 px-4 py-3 border-b border-edge">
        {policy?.emergencyPause ? (
          <BellOff size={18} color={colors.brick} />
        ) : (
          <Bell size={18} color={colors.ember} />
        )}
        <Text className="font-head font-bold text-[18px] text-bone">Notifications</Text>
      </View>

      {loading ? (
        <View className="flex-1 items-center justify-center">
          <ActivityIndicator color={colors.ember} />
        </View>
      ) : error ? (
        <View className="flex-1 items-center justify-center p-6">
          <Text className="text-fog text-sm text-center">
            Could not load notification settings.{"\n"}
            {error instanceof Error ? error.message : ""}
          </Text>
        </View>
      ) : (
        <ScrollView
          contentContainerStyle={{ paddingBottom: 60 }}
          refreshControl={
            <RefreshControl
              refreshing={settingsQ.isRefetching || policyQ.isRefetching}
              onRefresh={() => {
                void settingsQ.refetch();
                void policyQ.refetch();
                void deliveriesQ.refetch();
              }}
              tintColor={colors.ember}
            />
          }
        >
          {!isAdmin && (
            <Text className="text-fog text-[12px] px-4 pt-3">
              Organization-wide rules are read-only for your role. Channel delivery and
              policy changes require an administrator.
            </Text>
          )}
          {policy?.emergencyPause ? (
            <View className="mx-4 mt-3 bg-panel border border-edge rounded-xl p-3">
              <Text className="text-brick text-sm font-semibold">
                Emergency pause active{policy.pauseUntil ? ` until ${new Date(policy.pauseUntil).toLocaleString()}` : ""}
              </Text>
              {policy.pauseReason ? (
                <Text className="text-fog text-xs mt-1">{policy.pauseReason}</Text>
              ) : null}
              <Text className="text-fog text-xs mt-1">
                Monitoring and alert evaluation continue; only message delivery is held.
              </Text>
            </View>
          ) : null}
          {msg ? (
            <Text className="text-bone text-[12px] px-4 pt-3 font-mono">{msg}</Text>
          ) : null}

          <Section title="Channels">
            <Row
              label="Email"
              hint={
                settings?.email.hasPassword
                  ? `Configured (${settings.email.host || "custom SMTP"})`
                  : "Not configured — set SMTP in web Settings or backend config."
              }
              right={
                <Switch
                  value={!!settings?.email.enabled}
                  disabled={!isAdmin || saving}
                  onValueChange={(v) => toggleChannel("email", v)}
                />
              }
            />
            <Row
              label="Telegram"
              hint={
                settings?.telegram.hasToken
                  ? `Configured (chat ${settings.telegram.chatId || "set"})`
                  : "Not configured — set the bot token first."
              }
              right={
                <Switch
                  value={!!settings?.telegram.enabled}
                  disabled={!isAdmin || saving}
                  onValueChange={(v) => toggleChannel("telegram", v)}
                />
              }
            />
            <Row
              label="In-app notifications"
              hint="Always on. Alert events appear in the in-app notification center."
              right={<Text className="text-moss text-xs font-mono">ON</Text>}
            />
          </Section>

          <Section title="Delivery policy (shared by all channels)">
            <Row
              label="Critical cooldown (sec)"
              hint="Minimum gap between repeats of one critical incident."
              right={<NumField value={num("cooldownCriticalSec", DEFAULTS.cooldownCriticalSec)} onChange={setNum("cooldownCriticalSec")} editable={isAdmin} />}
            />
            <Row
              label="Warning cooldown (sec)"
              hint="Warnings wait longer between repeats."
              right={<NumField value={num("cooldownWarningSec", DEFAULTS.cooldownWarningSec)} onChange={setNum("cooldownWarningSec")} editable={isAdmin} />}
            />
            <Row
              label="Info cooldown (sec)"
              hint="Informational incidents rarely re-page."
              right={<NumField value={num("cooldownInfoSec", DEFAULTS.cooldownInfoSec)} onChange={setNum("cooldownInfoSec")} editable={isAdmin} />}
            />
            <Row
              label="Repeat interval (sec)"
              hint="Reminders while an incident stays active."
              right={<NumField value={num("repeatIntervalSec", DEFAULTS.repeatIntervalSec)} onChange={setNum("repeatIntervalSec")} editable={isAdmin} />}
            />
            <Row
              label="Max repeats"
              hint="Reminders per incident cycle, then quiet until resolved."
              right={<NumField value={num("maxRepeats", DEFAULTS.maxRepeats)} onChange={setNum("maxRepeats")} editable={isAdmin} />}
            />
            <Row
              label="Email budget / hour"
              hint="Excess is deferred into an overflow summary, never dropped silently."
              right={<NumField value={num("emailPerHour", DEFAULTS.emailPerHour)} onChange={setNum("emailPerHour")} editable={isAdmin} />}
            />
            <Row
              label="Telegram budget / hour"
              hint="Same overflow-summary behavior as email."
              right={<NumField value={num("tgPerHour", DEFAULTS.tgPerHour)} onChange={setNum("tgPerHour")} editable={isAdmin} />}
            />
            <Row
              label="Resolution notifications"
              hint="One message when an incident resolves after it paged."
              right={
                <Switch
                  value={!!policy?.notifyOnRecovery}
                  disabled={!isAdmin || saving}
                  onValueChange={(v) => void savePolicy({ notifyOnRecovery: v })}
                />
              }
            />
            {isAdmin && (
              <View className="px-4 py-3 flex-row gap-2">
                <TouchableOpacity
                  onPress={saveDraft}
                  disabled={saving || !draft}
                  className="flex-1 py-2.5 rounded-xl items-center"
                  style={{ backgroundColor: !draft ? colors.emboss : colors.ember }}
                >
                  <Text className="text-black font-semibold text-sm">
                    {saving ? "Saving…" : "Save policy"}
                  </Text>
                </TouchableOpacity>
                <TouchableOpacity
                  onPress={restoreDefaults}
                  disabled={saving}
                  className="px-3 py-2.5 rounded-xl border border-edge items-center flex-row gap-1"
                >
                  <RotateCcw size={13} color={colors.fog} />
                  <Text className="text-fog text-sm">Defaults</Text>
                </TouchableOpacity>
              </View>
            )}
          </Section>

          <Section title="Emergency pause">
            <Row
              label="Pause all delivery"
              hint="Monitoring keeps evaluating; only messages are held. Prefer an expiry over an open-ended pause."
              right={
                <Switch
                  value={!!policy?.emergencyPause}
                  disabled={!isAdmin || saving}
                  onValueChange={(v) =>
                    v
                      ? void savePolicy({
                          emergencyPause: true,
                          pauseUntil: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
                          pauseReason: "Paused from mobile",
                        })
                      : void savePolicy({ clearPause: true })
                  }
                />
              }
            />
          </Section>

          <Section title="Test & recent failures">
            <Row
              label="Send test notification"
              hint={isAdmin ? "Limited to one per minute per user." : "Administrators only."}
              right={
                <TouchableOpacity
                  onPress={sendTest}
                  disabled={!isAdmin || saving}
                  className="flex-row items-center gap-1 bg-emboss border border-edge rounded-lg px-2.5 py-1.5"
                >
                  <FlaskConical size={12} color={colors.fog} />
                  <Text className="text-fog text-xs">Test</Text>
                </TouchableOpacity>
              }
            />
            {failed.length === 0 ? (
              <Text className="text-fog text-[12px] px-4 py-3">No recent delivery failures.</Text>
            ) : (
              failed.slice(0, 5).map((d) => (
                <View key={d.id} className="px-4 py-2.5 border-b border-edge">
                  <Text className="text-bone text-[13px]" numberOfLines={1}>
                    {d.title}
                  </Text>
                  <Text className="text-brick text-[11px] font-mono mt-0.5" numberOfLines={2}>
                    {d.channel} · {d.status} · {d.attempts} attempt(s) · {d.lastError || "no detail"}
                  </Text>
                </View>
              ))
            )}
          </Section>

          <Text className="text-fog text-[11px] px-4 pb-2">
            Quiet hours live on notification groups and maintenance windows are managed
            from the web app; delivery history above shows what the shared policy decided.
          </Text>
        </ScrollView>
      )}
    </SafeAreaView>
  );
}
