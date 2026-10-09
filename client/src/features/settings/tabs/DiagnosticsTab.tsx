import { useState } from "react";
import { Wrench, TriangleAlert } from "lucide-react";
import type { ServerInfo } from "../../../lib/types";
import { fmtUptime } from "../../../lib/format";
import { useUi } from "../../../stores/store";
import { api } from "../../../lib/api";
import { Kicker, ConfirmModal } from "../../../components/ui";
import { GlassCard } from "../components/GlassCard";
import { DangerButton } from "../components/DangerButton";

interface DiagnosticsTabProps {
  info: ServerInfo | null;
  apiUrl: string;
}

export function DiagnosticsTab({ info, apiUrl }: DiagnosticsTabProps) {
  const pushToast = useUi((s) => s.pushToast);
  const [confirmRestart, setConfirmRestart] = useState(false);
  const [confirmWipe, setConfirmWipe] = useState(false);

  return (
    <div className="flex flex-col gap-6">
      {/* System Runtime Diagnostics */}
      <GlassCard>
        <div className="flex items-start justify-between gap-3 mb-6">
          <div>
            <div className="flex items-center gap-2">
              <Wrench size={20} className="text-accent dark:text-ember" />
              <h2 className="text-xl font-semibold text-ink dark:text-bone">Advanced Host Diagnostics</h2>
            </div>
            <p className="text-muted dark:text-fog text-sm mt-1">
              Low-level host environment variables, daemon socket endpoints, and runtime statistics.
            </p>
          </div>
          <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-paper dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-input px-2.5 py-1 whitespace-nowrap shadow-sm">
            SERVERHUB · CORE
          </span>
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="bg-paper dark:bg-emboss/40 border border-line dark:border-edge rounded-xl px-5 py-4">
            <Kicker>API Endpoint URL</Kicker>
            <div className="font-mono text-sm mt-2 truncate text-ink dark:text-bone">{apiUrl}</div>
            <p className="text-[11px] text-muted dark:text-fog mt-1">Base URL used for agent telemetry ingestion</p>
          </div>

          <div className="bg-paper dark:bg-emboss/40 border border-line dark:border-edge rounded-xl px-5 py-4">
            <div className="flex items-center justify-between gap-4">
              <Kicker>Docker Daemon Socket</Kicker>
              <span
                className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-[11px] font-mono font-medium ${
                  info?.dockerAvailable
                    ? "bg-moss/10 text-moss border border-moss/30"
                    : "bg-stone/10 text-stone border border-stone/30"
                }`}
              >
                <span className={`w-1.5 h-1.5 rounded-full ${info?.dockerAvailable ? "bg-moss" : "bg-stone"}`} />
                {info?.dockerAvailable ? "Ready" : "Unavailable"}
              </span>
            </div>
            <div className="font-mono text-sm mt-2 truncate text-ink dark:text-bone">
              {info ? (info.dockerAvailable ? "/var/run/docker.sock · live" : "unreachable") : "probing…"}
            </div>
            <p className="text-[11px] text-muted dark:text-fog mt-1">Host socket mounted for container discovery</p>
          </div>

          <div className="bg-paper dark:bg-emboss/40 border border-line dark:border-edge rounded-xl px-5 py-4">
            <Kicker>Hub Process Uptime</Kicker>
            <div className="font-mono text-sm mt-2 text-ink dark:text-bone">{info ? fmtUptime(info.uptimeSec) : "—"}</div>
            <p className="text-[11px] text-muted dark:text-fog mt-1">Elapsed runtime since last appliance restart</p>
          </div>

          <div className="bg-paper dark:bg-emboss/40 border border-line dark:border-edge rounded-xl px-5 py-4">
            <Kicker>Container Summary</Kicker>
            <div className="font-mono text-sm mt-2 truncate text-ink dark:text-bone">
              {info
                ? `${info.projects} projects · ${info.services} services · ${
                    info.runningContainers < 0 ? "?" : info.runningContainers
                  } running`
                : "—"}
            </div>
            <p className="text-[11px] text-muted dark:text-fog mt-1">Live active containers tracked by Docker engine</p>
          </div>
        </div>
      </GlassCard>

      {/* Danger Zone */}
      <GlassCard className="!border-brick/30 !bg-brick/5 dark:!bg-brick/10">
        <div className="flex items-center gap-2.5 flex-wrap text-brick mb-2">
          <TriangleAlert size={20} />
          <h2 className="text-xl font-bold text-ink dark:text-bone">Maintenance & Recovery Zone</h2>
        </div>
        <p className="text-muted dark:text-fog text-sm mb-6">
          High-impact administrative maintenance operations. Exercise caution.
        </p>
        <div className="flex items-center gap-4 flex-wrap">
          <DangerButton onClick={() => setConfirmRestart(true)}>
            Restart ServerHub Appliance
          </DangerButton>
          <DangerButton onClick={() => setConfirmWipe(true)}>
            Clear Deployment History
          </DangerButton>
        </div>
      </GlassCard>

      {confirmRestart && (
        <ConfirmModal
          title="Restart ServerHub?"
          body="ServerHub runs inside Docker and cannot restart its own host daemon directly. Run the command below on your host terminal — it has been copied to your clipboard."
          confirmLabel="Copy Command & Close"
          onClose={() => setConfirmRestart(false)}
          onConfirm={() => {
            void navigator.clipboard.writeText("docker compose restart serverhub").catch(() => undefined);
            pushToast("Command copied: docker compose restart serverhub");
            setConfirmRestart(false);
          }}
        />
      )}

      {confirmWipe && (
        <ConfirmModal
          title="Clear deployment history?"
          body="This permanently deletes all historical build & deployment execution records. Projects, services, and passwords remain untouched. This cannot be undone."
          confirmLabel="Clear History"
          requireText="CLEAR"
          onClose={() => setConfirmWipe(false)}
          onConfirm={async () => {
            try {
              const r = await api.wipeDeployments();
              pushToast(`History cleared. ${r.deleted} entries removed.`);
              setConfirmWipe(false);
            } catch (e) {
              pushToast(e instanceof Error ? e.message : "Clear history failed", true);
            }
          }}
        />
      )}
    </div>
  );
}
