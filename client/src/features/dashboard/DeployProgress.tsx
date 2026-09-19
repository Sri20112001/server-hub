import { useEffect, useState } from "react";
import { Check, Loader2, Rocket, X } from "lucide-react";
import { api } from "../../lib/api";
import type { Operation } from "../../lib/types";
import { useUi } from "../../stores/store";
import { Modal } from "../../components/ui";

const STAGE_DOT: Record<string, string> = {
  done: "bg-moss",
  active: "bg-status-amber",
  failed: "bg-brick",
  skipped: "bg-stone",
  pending: "bg-stone",
};

export function DeployProgress({
  projectName,
  operationId,
  onClose,
  onDone,
}: {
  projectName: string;
  operationId: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const pushToast = useUi((s) => s.pushToast);
  const [op, setOp] = useState<Operation | null>(null);
  const [done, setDone] = useState(false);

  useEffect(() => {
    let alive = true;
    let timer = 0;
    const poll = async () => {
      try {
        const cur = await api.operation(operationId);
        if (!alive) return;
        setOp(cur);
        if (cur.status === "SUCCESS" || cur.status === "FAILED" || cur.status === "CANCELLED") {
          if (!done) {
            setDone(true);
            onDone();
          }
          return;
        }
      } catch {
        /* transient — keep polling */
      }
      if (alive) timer = window.setTimeout(poll, 2000);
    };
    void poll();
    return () => {
      alive = false;
      window.clearTimeout(timer);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [operationId]);

  const terminal = op && op.status !== "QUEUED" && op.status !== "RUNNING";
  const failed = op?.status === "FAILED";

  return (
    <Modal onClose={onClose}>
      <div className="flex items-center gap-2 flex-wrap mb-1">
        <Rocket size={16} />
        <h3 className="text-[18px]">Dispatching {projectName}</h3>
      </div>
      <p className="text-muted dark:text-fog text-[13px] mb-4 font-mono">
        {operationId}
      </p>

      {!op && <div className="text-muted dark:text-fog text-[13px]">Opening channel…</div>}

      {op && (
        <div className="flex flex-col gap-2.5">
          {op.stages.map((s) => (
            <div key={s.name} className="flex items-center gap-2.5">
              {s.state === "done" ? (
                <span className="w-5 h-5 rounded-full bg-moss text-white flex items-center justify-center shrink-0">
                  <Check size={12} />
                </span>
              ) : s.state === "active" ? (
                <span className="w-5 h-5 rounded-full border border-line dark:border-edge flex items-center justify-center shrink-0">
                  <Loader2 size={12} className="animate-spin text-status-amber" />
                </span>
              ) : s.state === "failed" ? (
                <span className="w-5 h-5 rounded-full bg-brick text-white flex items-center justify-center shrink-0">
                  <X size={12} />
                </span>
              ) : (
                <span
                  className={`w-2 h-2 rounded-full ml-1.5 mr-1.5 shrink-0 ${STAGE_DOT[s.state] ?? "bg-stone"}`}
                />
              )}
              <span className="text-[13px]">{s.name}</span>
              {s.state === "active" && (
                <span className="font-mono text-[11px] text-status-amber ml-auto">◌ working</span>
              )}
            </div>
          ))}
        </div>
      )}

      {failed && op?.error && (
        <div className="bg-red-50 dark:bg-red-950 border border-red-200 dark:border-red-900 text-red-800 dark:text-red-200 rounded-input px-3 py-2.5 text-[13px] mt-4 max-h-40 overflow-y-auto whitespace-pre-wrap break-words font-mono text-[12px]">
          {op.error.slice(0, 2000)}
        </div>
      )}

      {terminal && !failed && (
        <div className="bg-green-50 dark:bg-green-950 border border-green-200 dark:border-green-900 text-green-800 dark:text-green-200 rounded-input px-3 py-2.5 text-[13px] mt-4">
          Complete — ship is sailing.
        </div>
      )}

      <div className="flex items-center gap-2 flex-wrap justify-end mt-4">
        {!terminal && (
          <span className="font-mono text-[11px] text-muted dark:text-fog mr-auto">
            live · polling operation
          </span>
        )}
        <button
          className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss"
          onClick={() => {
            if (!terminal) pushToast("Dispatch continues in the background.");
            onClose();
          }}
        >
          {terminal ? "Close" : "Watch in background"}
        </button>
      </div>
    </Modal>
  );
}
