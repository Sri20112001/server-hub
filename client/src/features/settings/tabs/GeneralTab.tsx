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
            <h2 className="text-xl font-semibold text-gray-900 dark:text-white">Host Configuration</h2>
            <p className="text-gray-500 dark:text-gray-400 text-sm mt-1">Core runtime parameters and daemon routing points.</p>
          </div>
          <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-gray-100 dark:bg-white/10 border border-gray-200 dark:border-white/20 text-gray-800 dark:text-white rounded-md px-2 py-1 whitespace-nowrap shadow-sm">SERVERHUB · MVP</span>
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="bg-gray-50 dark:bg-black/30 border border-gray-200 dark:border-white/10 rounded-xl px-5 py-4 transition-all hover:bg-gray-100 dark:hover:bg-black/40">
            <Kicker>API endpoint</Kicker>
            <div className="font-mono text-sm mt-2 truncate text-gray-800 dark:text-cyan-400">{apiUrl}</div>
          </div>
          <div className="bg-gray-50 dark:bg-black/30 border border-gray-200 dark:border-white/10 rounded-xl px-5 py-4 transition-all hover:bg-gray-100 dark:hover:bg-black/40">
            <div className="flex items-center justify-between gap-4">
              <Kicker>Docker socket</Kicker>
              <span
                className={
                  info?.dockerAvailable
                    ? "w-2.5 h-2.5 rounded-full bg-green-500 shadow-[0_0_8px_rgba(34,197,94,0.6)]"
                    : "w-2.5 h-2.5 rounded-full bg-gray-400 shadow-[0_0_8px_rgba(156,163,175,0.4)]"
                }
              />
            </div>
            <div className="font-mono text-sm mt-2 truncate text-gray-800 dark:text-cyan-400">
              {info ? (info.dockerAvailable ? "/var/run/docker.sock · live" : "unreachable") : "probing…"}
            </div>
          </div>
          <div className="bg-gray-50 dark:bg-black/30 border border-gray-200 dark:border-white/10 rounded-xl px-5 py-4 transition-all hover:bg-gray-100 dark:hover:bg-black/40">
            <Kicker>Uptime</Kicker>
            <div className="font-mono text-sm mt-2 text-gray-800 dark:text-cyan-400">{info ? fmtUptime(info.uptimeSec) : "—"}</div>
          </div>
          <div className="bg-gray-50 dark:bg-black/30 border border-gray-200 dark:border-white/10 rounded-xl px-5 py-4 transition-all hover:bg-gray-100 dark:hover:bg-black/40">
            <Kicker>Fleet census</Kicker>
            <div className="font-mono text-sm mt-2 truncate text-gray-800 dark:text-cyan-400">
              {info
                ? `${info.projects} ships · ${info.services} stations · ${
                    info.runningContainers < 0 ? "?" : info.runningContainers
                  } running`
                : "—"}
            </div>
          </div>
        </div>

        <div className="bg-gray-50 dark:bg-black/30 border border-gray-200 dark:border-white/10 rounded-xl px-5 py-4 mt-4 flex items-center justify-between gap-2 flex-wrap">
          <span className="text-sm font-medium text-gray-900 dark:text-white">Heartbeat interval</span>
          <span className="font-mono text-xs text-gray-500 dark:text-gray-400">
            Every tick <strong className="font-mono text-gray-900 dark:text-cyan-400 ml-1">30s</strong>
          </span>
        </div>
      </GlassCard>

      {/* Alerts & Signals */}
      <GlassCard>
        <div className="flex items-start justify-between gap-3 mb-6">
          <div>
            <h2 className="text-xl font-semibold text-gray-900 dark:text-white">Alerts & Signals</h2>
            <p className="text-gray-500 dark:text-gray-400 text-sm mt-1">Routing policies for internal bus events and dispatch triggers.</p>
          </div>
        </div>
        <div className="flex flex-col gap-0 divide-y divide-gray-200 dark:divide-white/10">
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
        <div className="font-medium text-sm text-gray-900 dark:text-white">{title}</div>
        <div className="text-gray-500 dark:text-gray-400 text-[13px] mt-0.5">{hint}</div>
      </div>
      <NeonToggle checked={on} onChange={onFlip} label={title} />
    </div>
  );
}
