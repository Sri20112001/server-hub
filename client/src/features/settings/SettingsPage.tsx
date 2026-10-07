import { useEffect, useState } from "react";
import { api } from "../../lib/api";
import type { NotificationGroup, NotifySettings, ServerInfo } from "../../lib/types";
import { SettingsSkeleton } from "../../components/ui";
import { Kicker } from "../../components/ui";
import { SegmentedTabs } from "./components/SegmentedTabs";
import { GeneralTab } from "./tabs/GeneralTab";
import { SecurityTab } from "./tabs/SecurityTab";
import { NotificationsTab } from "./tabs/NotificationsTab";
import { Settings, Shield, Bell } from "lucide-react";

export function SettingsPage({ setOnline }: { setOnline: (v: boolean) => void }) {
  const [info, setInfo] = useState<ServerInfo | null>(null);
  const [notify, setNotify] = useState<NotifySettings | null>(null);
  const [groups, setGroups] = useState<NotificationGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [activeTab, setActiveTab] = useState("general");

  const refreshGroups = async () => {
    try {
      const list = await api.notificationGroups();
      setGroups(list);
    } catch {
      /* ignore */
    }
  };

  useEffect(() => {
    let mounted = true;
    Promise.allSettled([
      api.server(),
      api.notifySettings(),
      api.notificationGroups()
    ]).then(([serverRes, notifyRes, groupsRes]) => {
      if (!mounted) return;
      
      if (serverRes.status === "fulfilled") {
        setInfo(serverRes.value);
        setOnline(true);
      } else {
        setOnline(false);
      }

      if (notifyRes.status === "fulfilled") {
        setNotify(notifyRes.value);
      }

      if (groupsRes.status === "fulfilled") {
        setGroups(groupsRes.value);
      }

      setLoading(false);
    });

    return () => { mounted = false; };
  }, [setOnline]);

  const apiUrl = (import.meta.env.VITE_API_URL as string | undefined) ?? "http://localhost:4000";

  if (loading) {
    return <SettingsSkeleton />;
  }

  const tabs = [
    { id: "general", label: "General", icon: <Settings size={16} /> },
    { id: "security", label: "Security", icon: <Shield size={16} /> },
    { id: "notifications", label: "Notifications", icon: <Bell size={16} /> },
  ];

  return (
    <main className="w-full max-w-[800px] mx-auto px-6 max-md:px-4 pt-24 pb-36">
      <Kicker>Server preferences & access</Kicker>
      <h1 className="font-head text-[36px] font-bold tracking-[-0.03em] leading-[1.2] max-md:text-[28px] mt-2 mb-2 text-gray-900 dark:text-white drop-shadow-sm">
        Settings
      </h1>
      <p className="text-gray-500 dark:text-gray-400 mb-8 text-base">
        Configure host environment, telemetry notification channels, and operational daemon privileges.
      </p>

      <div className="mb-8 relative z-20">
        <SegmentedTabs tabs={tabs} activeId={activeTab} onChange={setActiveTab} />
      </div>

      <div className="transition-all duration-500 relative z-10">
        {activeTab === "general" && <GeneralTab info={info} apiUrl={apiUrl} />}
        {activeTab === "security" && <SecurityTab />}
        {activeTab === "notifications" && <NotificationsTab notify={notify} setNotify={setNotify} groups={groups} refreshGroups={refreshGroups} />}
      </div>
    </main>
  );
}
