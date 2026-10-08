import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ClipboardList, RefreshCw } from "lucide-react";
import { api } from "../../lib/api";

export function AuditLogPage() {
  const [limit, setLimit] = useState(100);

  const { data: logs = [], isLoading, refetch } = useQuery({
    queryKey: ["audit", limit],
    queryFn: () => api.audit(limit),
    refetchInterval: 60_000,
  });

  return (
    <main className="w-full max-w-5xl mx-auto px-4 sm:px-6 pt-24 pb-36">
      <div className="flex items-center justify-between mb-6 flex-wrap gap-4">
        <div>
          <div className="flex items-center gap-3">
            <ClipboardList size={22} className="text-accent dark:text-ember" />
            <h1 className="text-ink dark:text-bone font-head font-bold text-[24px] tracking-[-0.02em]">Audit Log</h1>
            <span className="text-muted dark:text-fog text-xs font-mono">{logs.length} records</span>
          </div>
          <p className="text-muted dark:text-fog text-xs mt-1 font-mono">
            Security & operational audit trail
          </p>
        </div>
        <div className="flex items-center gap-2">
          <select
            className="bg-white dark:bg-panel border border-line dark:border-edge rounded-lg px-3 py-1.5 text-ink dark:text-bone text-xs font-mono outline-none focus:border-accent dark:focus:border-ember shadow-sm cursor-pointer"
            value={limit}
            onChange={(e) => setLimit(Number(e.target.value))}
          >
            {[50, 100, 200, 500].map((n) => (
              <option key={n} value={n}>Last {n}</option>
            ))}
          </select>
          <button onClick={() => void refetch()}
            className="w-9 h-9 rounded-full bg-white dark:bg-panel border border-line dark:border-edge flex items-center justify-center text-muted dark:text-fog hover:text-ink dark:hover:text-bone transition-colors shadow-sm cursor-pointer"
            aria-label="Refresh"
          >
            <RefreshCw size={15} />
          </button>
        </div>
      </div>

      <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card overflow-hidden shadow-sm">
        <div className="overflow-x-auto">
          <table className="w-full text-sm border-collapse">
            <thead>
              <tr className="bg-paper dark:bg-abyss border-b border-line dark:border-edge">
                {["Time", "Actor", "Action", "Resource", "Result"].map((h) => (
                  <th key={h} className="text-left text-muted dark:text-fog text-[11px] px-4 py-2.5 font-semibold uppercase tracking-wider">
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {isLoading ? (
                <tr><td colSpan={5} className="px-4 py-8 text-center text-muted dark:text-fog text-sm">Loading…</td></tr>
              ) : logs.length === 0 ? (
                <tr><td colSpan={5} className="px-4 py-8 text-center text-muted dark:text-fog text-sm">No audit records.</td></tr>
              ) : logs.map((log) => (
                <tr key={log.id} className="border-t border-line dark:border-edge hover:bg-paper/50 dark:hover:bg-abyss/50 transition-colors">
                  <td className="px-4 py-2.5 font-mono text-xs text-muted dark:text-fog whitespace-nowrap">
                    {new Date(log.timestamp).toLocaleString()}
                  </td>
                  <td className="px-4 py-2.5 font-mono text-xs text-ink dark:text-bone font-medium">{log.actor}</td>
                  <td className="px-4 py-2.5 font-mono text-xs text-ink dark:text-bone">{log.action}</td>
                  <td className="px-4 py-2.5 font-mono text-xs text-muted dark:text-fog">
                    {log.resource}{log.resourceId ? ` #${log.resourceId}` : ""}
                  </td>
                  <td className="px-4 py-2.5 font-mono text-xs">
                    <span className={`px-2 py-0.5 rounded text-[11px] font-semibold border ${
                      log.result === "ok"
                        ? "bg-moss/10 border-moss/30 text-moss"
                        : "bg-brick/10 border-brick/30 text-brick"
                    }`}>
                      {log.result}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </main>
  );
}
