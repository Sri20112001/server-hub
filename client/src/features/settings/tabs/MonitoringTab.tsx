import { Activity, Bell, Sliders } from "lucide-react";
import { Kicker, Toggle } from "../../../components/ui";
import { GlassCard } from "../components/GlassCard";
import { useAlerts } from "../hooks/useAlerts";

export function MonitoringTab() {
  const { alerts, flip } = useAlerts();

  return (
    <div className="flex flex-col gap-6">
      {/* Monitoring & Polling Policy */}
      <GlassCard>
        <div className="flex items-start justify-between gap-3 mb-6">
          <div>
            <div className="flex items-center gap-2">
              <Activity size={20} className="text-accent dark:text-ember" />
              <h2 className="text-xl font-semibold text-ink dark:text-bone">Health & Polling Intervals</h2>
            </div>
            <p className="text-muted dark:text-fog text-sm mt-1">
              Configure how frequently ServerHub checks server availability and ingests metrics.
            </p>
          </div>
          <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-paper dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-input px-2.5 py-1 whitespace-nowrap shadow-sm">
            AUTO-PROBE
          </span>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
          <div className="bg-paper dark:bg-emboss/40 border border-line dark:border-edge rounded-xl px-5 py-4">
            <Kicker>Heartbeat Timeout</Kicker>
            <div className="font-mono text-lg font-bold mt-2 text-ink dark:text-bone">30 seconds</div>
            <p className="text-[11px] text-muted dark:text-fog mt-1">Expected ping interval from remote agents</p>
          </div>

          <div className="bg-paper dark:bg-emboss/40 border border-line dark:border-edge rounded-xl px-5 py-4">
            <Kicker>Offline Threshold</Kicker>
            <div className="font-mono text-lg font-bold mt-2 text-status-amber">90 seconds</div>
            <p className="text-[11px] text-muted dark:text-fog mt-1">Mark server offline after 3 missed heartbeats</p>
          </div>

          <div className="bg-paper dark:bg-emboss/40 border border-line dark:border-edge rounded-xl px-5 py-4">
            <Kicker>Metrics History</Kicker>
            <div className="font-mono text-lg font-bold mt-2 text-moss">30 days</div>
            <p className="text-[11px] text-muted dark:text-fog mt-1">Telemetry retention window for graphs</p>
          </div>
        </div>
      </GlassCard>

      {/* Threshold Policies */}
      <GlassCard>
        <div className="flex items-start justify-between gap-3 mb-6">
          <div>
            <div className="flex items-center gap-2">
              <Sliders size={20} className="text-accent dark:text-ember" />
              <h2 className="text-xl font-semibold text-ink dark:text-bone">Alert Thresholds</h2>
            </div>
            <p className="text-muted dark:text-fog text-sm mt-1">
              Resource utilization boundaries that trigger warnings and notifications.
            </p>
          </div>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
          <div className="bg-paper dark:bg-emboss/40 border border-line dark:border-edge rounded-xl px-4 py-3">
            <span className="text-xs text-muted dark:text-fog">CPU High Load Warning</span>
            <div className="text-base font-bold font-mono text-ink dark:text-bone mt-1">&gt; 85% for 5 min</div>
          </div>
          <div className="bg-paper dark:bg-emboss/40 border border-line dark:border-edge rounded-xl px-4 py-3">
            <span className="text-xs text-muted dark:text-fog">Memory Usage Warning</span>
            <div className="text-base font-bold font-mono text-ink dark:text-bone mt-1">&gt; 85% for 5 min</div>
          </div>
          <div className="bg-paper dark:bg-emboss/40 border border-line dark:border-edge rounded-xl px-4 py-3">
            <span className="text-xs text-muted dark:text-fog">Disk Capacity Warning</span>
            <div className="text-base font-bold font-mono text-ink dark:text-bone mt-1">&gt; 90% space used</div>
          </div>
        </div>
      </GlassCard>

      {/* In-App Notifications */}
      <GlassCard>
        <div className="flex items-start justify-between gap-3 mb-4">
          <div>
            <div className="flex items-center gap-2">
              <Bell size={20} className="text-accent dark:text-ember" />
              <h2 className="text-xl font-semibold text-ink dark:text-bone">In-App Alert Banners</h2>
            </div>
            <p className="text-muted dark:text-fog text-sm mt-1">Real-time alerts displayed while browsing the console.</p>
          </div>
        </div>

        <div className="flex flex-col gap-0 divide-y divide-line dark:divide-edge">
          <AlertRow
            title="Deployment status"
            hint="Display toast notification when a project deployment succeeds or fails"
            on={alerts.deployFinished}
            onFlip={() => flip("deployFinished")}
          />
          <AlertRow
            title="Service health changes"
            hint="Display notification when a service or host turns degraded or goes offline"
            on={alerts.healthChanged}
            onFlip={() => flip("healthChanged")}
          />
          <AlertRow
            title="High resource consumption"
            hint="Display notification when CPU or RAM remains over 85% for prolonged periods"
            on={alerts.highUsage}
            onFlip={() => flip("highUsage")}
          />
        </div>
      </GlassCard>
    </div>
  );
}

function AlertRow({ title, hint, on, onFlip }: { title: string; hint: string; on: boolean; onFlip: () => void }) {
  return (
    <div className="flex items-center justify-between gap-4 py-4">
      <div>
        <div className="font-medium text-sm text-ink dark:text-bone">{title}</div>
        <div className="text-muted dark:text-fog text-[13px] mt-0.5">{hint}</div>
      </div>
      <Toggle checked={on} onChange={onFlip} label={title} />
    </div>
  );
}
