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
    <main className="w-full max-w-5xl mx-auto px-6 pt-12 pb-28">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-end justify-between gap-4 mb-8">
        <div>
          <div className="flex items-center gap-2 mb-1.5">
            <span className="inline-flex items-center justify-center w-6 h-6 rounded-md bg-primary/10 text-primary">
              <ClipboardList size={15} />
            </span>
            <span className="text-[11px] font-semibold text-outline uppercase tracking-wider">Activity & Governance</span>
          </div>
          <div className="flex items-center gap-3">
            <h1 className="text-[32px] sm:text-[36px] font-bold text-on-surface tracking-tight leading-tight">Activity Log</h1>
            <span className="text-on-surface-variant text-[13px] font-mono font-medium bg-surface-container-low px-2.5 py-0.5 rounded-full">
              {logs.length} events
            </span>
          </div>
          <p className="text-[15px] text-on-surface-variant max-w-xl mt-1">
            Immutable security, authentication, and operational execution trail.
          </p>
        </div>
        <div className="flex items-center gap-2">
          {/* Limit pill segment */}
          <div className="flex items-center gap-1 p-1 bg-surface-container rounded-full shadow-inner">
            {[50, 100, 200].map((n) => (
              <button
                key={n}
                onClick={() => setLimit(n)}
                className={`px-3 py-1 rounded-full text-[12px] font-medium transition-all cursor-pointer border-0 ${
                  limit === n
                    ? "bg-surface-container-lowest text-primary font-semibold shadow-sm"
                    : "text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high"
                }`}
                type="button"
              >
                {n}
              </button>
            ))}
          </div>

          <button
            onClick={() => void refetch()}
            className="w-10 h-10 rounded-full bg-surface-container-lowest border border-outline-variant/30 flex items-center justify-center text-on-surface-variant hover:text-on-surface shadow-sm cursor-pointer transition-colors"
            aria-label="Refresh"
            type="button"
          >
            <RefreshCw size={15} />
          </button>
        </div>
      </div>

      <div className="bg-surface-container-lowest border border-outline-variant/20 rounded-2xl overflow-hidden shadow-sm">
        <div className="overflow-x-auto">
          <table className="w-full text-[13px] border-collapse">
            <thead>
              <tr className="bg-surface-container-low/60 border-b border-outline-variant/20 text-on-surface-variant">
                {["Timestamp", "Actor", "Action", "Target Resource", "Result"].map((h) => (
                  <th key={h} className="text-left text-[11px] px-5 py-3 font-semibold uppercase tracking-wider">
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {isLoading ? (
                <tr>
                  <td colSpan={5} className="px-5 py-12 text-center text-on-surface-variant text-[14px]">
                    Loading activity stream…
                  </td>
                </tr>
              ) : logs.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-5 py-12 text-center text-on-surface-variant text-[14px]">
                    No activity records found.
                  </td>
                </tr>
              ) : (
                logs.map((log) => (
                  <tr key={log.id} className="border-t border-outline-variant/15 hover:bg-surface-container-low/40 transition-colors">
                    <td className="px-5 py-3 font-mono text-[12px] text-on-surface-variant whitespace-nowrap">
                      {new Date(log.timestamp).toLocaleString()}
                    </td>
                    <td className="px-5 py-3 font-mono text-[12px] text-on-surface font-semibold">{log.actor}</td>
                    <td className="px-5 py-3 font-mono text-[12px] text-on-surface">{log.action}</td>
                    <td className="px-5 py-3 font-mono text-[12px] text-on-surface-variant">
                      {log.resource}{log.resourceId ? ` #${log.resourceId}` : ""}
                    </td>
                    <td className="px-5 py-3 font-mono text-[12px]">
                      <span className={`px-2.5 py-0.5 rounded-full text-[11px] font-semibold border ${
                        log.result === "ok"
                          ? "bg-secondary-container/30 border-secondary/30 text-secondary"
                          : "bg-error-container/30 border-error/30 text-error"
                      }`}>
                        {log.result}
                      </span>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
    </main>
  );
}
