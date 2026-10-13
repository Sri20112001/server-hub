import { useState } from "react";
import { DatabaseBackup } from "lucide-react";
import { OverviewTab } from "./OverviewTab";
import { JobsTab } from "./JobsTab";
import { RunsTab } from "./RunsTab";
import { RestoresTab } from "./RestoresTab";
import { AgentsTab } from "./AgentsTab";
import { StorageTab } from "./StorageTab";
import { ConnectionsTab } from "./ConnectionsTab";
import { PoliciesTab } from "./PoliciesTab";

export type DbBackupsTab =
  | "overview"
  | "jobs"
  | "runs"
  | "restores"
  | "agents"
  | "storage"
  | "connections"
  | "policies";

const TABS: { id: DbBackupsTab; label: string }[] = [
  { id: "overview", label: "Overview" },
  { id: "jobs", label: "Jobs" },
  { id: "runs", label: "Runs" },
  { id: "restores", label: "Restores" },
  { id: "agents", label: "Agents" },
  { id: "storage", label: "Storage" },
  { id: "connections", label: "Connections" },
  { id: "policies", label: "Policies" },
];

export function DatabaseBackupsPage() {
  const [tab, setTab] = useState<DbBackupsTab>("overview");

  return (
    <main className="w-full max-w-5xl mx-auto px-6 pt-12 pb-28">
      <div className="flex flex-col gap-8">
        {/* 1. In-Canvas Title Area */}
        <div className="flex flex-col sm:flex-row sm:items-end justify-between gap-4">
          <div className="flex flex-col gap-1.5">
            <div className="flex items-center gap-2">
              <span className="inline-flex items-center justify-center w-6 h-6 rounded-md bg-primary/10 text-primary">
                <DatabaseBackup size={15} />
              </span>
              <span className="text-[11px] font-semibold text-outline uppercase tracking-wider">Storage Engine</span>
            </div>
            <h1 className="text-[32px] sm:text-[36px] font-bold text-on-surface tracking-tight leading-tight">Database Backups</h1>
            <p className="text-[15px] text-on-surface-variant max-w-xl">
              Automated database snapshots, point-in-time recovery points, and storage destinations.
            </p>
          </div>
          <div className="flex items-center">
            <button
              onClick={() => setTab("jobs")}
              className="inline-flex items-center gap-2 px-5 py-2.5 rounded-full bg-primary-container hover:bg-primary text-on-primary text-[14px] font-medium shadow-sm hover:shadow-md transition-all cursor-pointer border-0"
              type="button"
            >
              <span>Manage Jobs</span>
            </button>
          </div>
        </div>

        {/* 2. Contextual In-Page Pill Navigation */}
        <div className="flex items-center overflow-x-auto pb-1 gap-1 p-1 bg-surface-container rounded-full max-w-fit shadow-inner" role="tablist">
          {TABS.map((t) => (
            <button
              key={t.id}
              role="tab"
              aria-selected={tab === t.id}
              onClick={() => setTab(t.id)}
              className={`px-4 py-1.5 rounded-full text-[13px] font-medium transition-all cursor-pointer border-0 whitespace-nowrap ${
                tab === t.id
                  ? "bg-surface-container-lowest text-primary font-semibold shadow-sm"
                  : "text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high"
              }`}
            >
              {t.label}
            </button>
          ))}
        </div>

        {tab === "overview" && <OverviewTab go={setTab} />}
        {tab === "jobs" && <JobsTab />}
        {tab === "runs" && <RunsTab />}
        {tab === "restores" && <RestoresTab />}
        {tab === "agents" && <AgentsTab />}
        {tab === "storage" && <StorageTab />}
        {tab === "connections" && <ConnectionsTab />}
        {tab === "policies" && <PoliciesTab />}
      </div>
    </main>
  );
}
