import type { ServerInfo } from '../../../lib/types';
import { fmtUptime } from '../../../lib/format';
import { Kicker } from '../../../components/ui';
import { GlassCard } from '../components/GlassCard';
import { useAlerts } from '../hooks/useAlerts';
import { NeonToggle } from '../components/NeonToggle';

interface GeneralTabProps {
  info: ServerInfo | null;
  apiUrl: string;
}

export function GeneralTab({ info, apiUrl }: GeneralTabProps) {
  const { alerts, flip } = useAlerts();

  return (
    <div className="flex flex-col gap-6">
      {/* Host Configuration */}
      <GlassCard>
        <div className="flex items-start justify-between gap-3 mb-6">
          <div>
            <h2 className="text-xl font-semibold text-ink dark:text-bone">Host Configuration</h2>
            <p className="text-muted dark:text-fog text-sm mt-1">Core runtime parameters and daemon routing points.</p>
          </div>
          <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-paper dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-input px-2 py-1 whitespace-nowrap shadow-sm">SERVERHUB · MVP</span>
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="bg-paper dark:bg-emboss/40 border border-line dark:border-edge rounded-xl px-5 py-4 transition-colors hover:bg-paper/80 dark:hover:bg-emboss/70">
            <Kicker>API endpoint</Kicker>
            <div className="font-mono text-sm mt-2 truncate text-ink dark:text-bone">{apiUrl}</div>
          </div>
          <div className="bg-paper dark:bg-emboss/40 border border-line dark:border-edge rounded-xl px-5 py-4 transition-colors hover:bg-paper/80 dark:hover:bg-emboss/70">
            <div className="flex items-center justify-between gap-4">
              <Kicker>Docker socket</Kicker>
              <span
                className={
                  info?.dockerAvailable
                    ? "w-2.5 h-2.5 rounded-full bg-moss"
                    : "w-2.5 h-2.5 rounded-full bg-stone"
                }
              />
            </div>
            <div className="font-mono text-sm mt-2 truncate text-ink dark:text-bone">
              {info ? (info.dockerAvailable ? "/var/run/docker.sock · live" : "unreachable") : "probing…"}
            </div>
          </div>
          <div className="bg-paper dark:bg-emboss/40 border border-line dark:border-edge rounded-xl px-5 py-4 transition-colors hover:bg-paper/80 dark:hover:bg-emboss/70">
            <Kicker>Uptime</Kicker>
            <div className="font-mono text-sm mt-2 text-ink dark:text-bone">{info ? fmtUptime(info.uptimeSec) : "—"}</div>
          </div>
          <div className="bg-paper dark:bg-emboss/40 border border-line dark:border-edge rounded-xl px-5 py-4 transition-colors hover:bg-paper/80 dark:hover:bg-emboss/70">
            <Kicker>Fleet census</Kicker>
            <div className="font-mono text-sm mt-2 truncate text-ink dark:text-bone">
              {info
                ? `${info.projects} ships · ${info.services} stations · ${
                    info.runningContainers < 0 ? "?" : info.runningContainers
                  } running`
                : "—"}
            </div>
          </div>
        </div>

        <div className="bg-paper dark:bg-emboss/40 border border-line dark:border-edge rounded-xl px-5 py-4 mt-4 flex items-center justify-between gap-2 flex-wrap">
          <span className="text-sm font-medium text-ink dark:text-bone">Heartbeat interval</span>
          <span className="font-mono text-xs text-muted dark:text-fog">
            Every tick <strong className="font-mono text-ink dark:text-bone ml-1">30s</strong>
          </span>
        </div>
      </GlassCard>

      {/* Alerts & Signals */}
      <GlassCard>
        <div className="flex items-start justify-between gap-3 mb-6">
          <div>
            <h2 className="text-xl font-semibold text-ink dark:text-bone">Alerts & Signals</h2>
            <p className="text-muted dark:text-fog text-sm mt-1">Routing policies for internal bus events and dispatch triggers.</p>
          </div>
        </div>
        <div className="flex flex-col gap-0 divide-y divide-line dark:divide-edge">
          <AlertRow
            title="Deployment finished"
            hint="Toast upon successful container ship"
            on={alerts.deployFinished}
            onFlip={() => flip("deployFinished")}
          />
          <AlertRow
            title="Health state changed"
            hint="Toast if a ship turns Choppy or Lost signal"
            on={alerts.healthChanged}
            onFlip={() => flip("healthChanged")}
          />
          <AlertRow
            title="High resource usage"
            hint="Toast if CPU/RAM exceeds 85% for > 5 mins"
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
      <NeonToggle checked={on} onChange={onFlip} label={title} />
    </div>
  );
}
