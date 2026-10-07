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
    <div className="min-h-screen bg-abyss pt-20 pb-28 px-4 sm:px-6 max-w-5xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-3">
          <ClipboardList size={22} className="text-ember" />
          <h1 className="text-bone font-bold text-xl">Audit Log</h1>
          <span className="text-fog text-xs font-mono">{logs.length} records</span>
        </div>
        <div className="flex items-center gap-2">
          <select
            className="bg-emboss border border-edge rounded-lg px-3 py-1.5 text-bone text-xs font-mono outline-none"
            value={limit}
            onChange={(e) => setLimit(Number(e.target.value))}
          >
            {[50, 100, 200, 500].map((n) => (
              <option key={n} value={n}>Last {n}</option>
            ))}
          </select>
          <button onClick={() => void refetch()}
            className="w-9 h-9 rounded-full bg-emboss flex items-center justify-center text-fog hover:text-bone">
            <RefreshCw size={15} />
          </button>
        </div>
      </div>

      <div className="bg-panel border border-edge rounded-xl overflow-hidden">
        <table className="w-full text-sm border-collapse">
          <thead>
            <tr className="bg-emboss">
              {["Time", "Actor", "Action", "Resource", "Result"].map((h) => (
                <th key={h} className="text-left text-fog text-xs px-4 py-2.5 font-semibold uppercase tracking-wider first:rounded-tl-xl last:rounded-tr-xl">
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {isLoading ? (
              <tr><td colSpan={5} className="px-4 py-8 text-center text-fog text-sm">Loading…</td></tr>
            ) : logs.length === 0 ? (
              <tr><td colSpan={5} className="px-4 py-8 text-center text-fog text-sm">No audit records.</td></tr>
            ) : logs.map((log) => (
              <tr key={log.id} className="border-t border-edge hover:bg-emboss/40 transition-colors">
                <td className="px-4 py-2.5 font-mono text-xs text-fog whitespace-nowrap">
                  {new Date(log.timestamp).toLocaleString()}
                </td>
                <td className="px-4 py-2.5 font-mono text-xs text-bone">{log.actor}</td>
                <td className="px-4 py-2.5 font-mono text-xs text-bone">{log.action}</td>
                <td className="px-4 py-2.5 font-mono text-xs text-fog">
                  {log.resource}{log.resourceId ? ` #${log.resourceId}` : ""}
                </td>
                <td className={`px-4 py-2.5 font-mono text-xs ${log.result === "ok" ? "text-green-400" : "text-red-400"}`}>
                  {log.result}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
