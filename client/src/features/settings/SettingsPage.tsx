import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../lib/api";
import type { NotifySettings } from "../../lib/types";
import { SettingsSkeleton } from "../../components/ui";
import { Kicker } from "../../components/ui";
import { SegmentedTabs } from "./components/SegmentedTabs";
import { MonitoringTab } from "./tabs/MonitoringTab";
import { NotificationsTab } from "./tabs/NotificationsTab";
import { TeamTab } from "./tabs/TeamTab";
import { DiagnosticsTab } from "./tabs/DiagnosticsTab";
import { Activity, Bell, Users, Wrench } from "lucide-react";

export function SettingsPage({ setOnline }: { setOnline: (v: boolean) => void }) {
  const qc = useQueryClient();
  const [activeTab, setActiveTab] = useState("monitoring");

  const { data: settingsData } = useQuery({
    queryKey: ["settings-data"],
    queryFn: async () => {
      const [serverRes, notifyRes, groupsRes] = await Promise.allSettled([
        api.server(),
        api.notifySettings(),
        api.notificationGroups(),
      ]);

      const isOnline = serverRes.status === "fulfilled";
      setOnline(isOnline);

      return {
        info: serverRes.status === "fulfilled" ? serverRes.value : null,
        notify: notifyRes.status === "fulfilled" ? notifyRes.value : null,
        groups: groupsRes.status === "fulfilled" ? groupsRes.value : [],
      };
    },
    staleTime: 60_000,
  });

  const [notify, setNotify] = useState<NotifySettings | null>(null);
  // Tracks the last synced remote value so the draft resets when fresh
  // settings arrive (e.g. after refreshGroups), without an effect: this is
  // React's documented "adjust state during render" pattern for prop/state
  // syncing, keeping the NotificationsTab draft editable in between.
  const [prevRemoteNotify, setPrevRemoteNotify] = useState<NotifySettings | null>(null);

  const info = settingsData?.info ?? null;
  const groups = settingsData?.groups ?? [];
  const remoteNotify = settingsData?.notify ?? null;
  if (remoteNotify !== prevRemoteNotify) {
    setPrevRemoteNotify(remoteNotify);
    setNotify(remoteNotify);
  }

  const refreshGroups = async () => {
    try {
      await qc.invalidateQueries({ queryKey: ["settings-data"] });
    } catch {
      /* ignore */
    }
  };

  const apiUrl = (import.meta.env.VITE_API_URL as string | undefined) ?? "http://localhost:4000";

  if (!info || !notify) {
    return <SettingsSkeleton />;
  }

  const tabs = [
    { id: "monitoring", label: "Monitoring", icon: <Activity size={16} /> },
    { id: "notifications", label: "Notifications", icon: <Bell size={16} /> },
    { id: "team", label: "Team & Access", icon: <Users size={16} /> },
    { id: "diagnostics", label: "Advanced Diagnostics", icon: <Wrench size={16} /> },
  ];

  return (
    <main className="w-full max-w-[840px] mx-auto px-6 max-md:px-4 pt-24 pb-36">
      <Kicker>Appliance & System Preferences</Kicker>
      <h1 className="font-head text-[36px] font-bold tracking-[-0.03em] leading-[1.2] max-md:text-[28px] mt-2 mb-2 text-ink dark:text-bone">
        Settings
      </h1>
      <p className="text-muted dark:text-fog mb-8 text-base">
        Manage health check thresholds, alert dispatch channels, team access levels, and host diagnostics.
      </p>

      <div className="mb-8 relative z-20">
        <SegmentedTabs tabs={tabs} activeId={activeTab} onChange={setActiveTab} />
      </div>

      <div className="transition-all duration-500 relative z-10">
        {activeTab === "monitoring" && <MonitoringTab />}
        {activeTab === "notifications" && (
          <NotificationsTab notify={notify} setNotify={setNotify} groups={groups} refreshGroups={refreshGroups} />
        )}
        {activeTab === "team" && <TeamTab />}
        {activeTab === "diagnostics" && <DiagnosticsTab info={info} apiUrl={apiUrl} />}
      </div>
    </main>
  );
}
