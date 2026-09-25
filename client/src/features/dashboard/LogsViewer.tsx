import { useEffect, useRef, useState } from "react";
import { Pause, Play, Trash2 } from "lucide-react";
import { API_BASE } from "../../lib/api";
import { Kicker } from "../../components/ui";

type Level = "ALL" | "INFO" | "WARN" | "ERROR" | "DEBUG";

function levelOf(line: string): Exclude<Level, "ALL"> {
  if (/error|err\b|fail|exception|fatal|panic/i.test(line)) return "ERROR";
  if (/warn/i.test(line)) return "WARN";
  if (/debug|trace/i.test(line)) return "DEBUG";
  return "INFO";
}

const LEVEL_CLS: Record<Exclude<Level, "ALL">, string> = {
  INFO: "text-ink dark:text-bone",
  WARN: "text-status-amber",
  ERROR: "text-brick",
  DEBUG: "text-muted dark:text-fog",
};

const MAX_LINES = 800;

export function LogsViewer({ container }: { container: string }) {
  const [lines, setLines] = useState<string[]>([]);
  const [live, setLive] = useState(true);
  const [query, setQuery] = useState("");
  const [level, setLevel] = useState<Level>("ALL");
  const [tail, setTail] = useState("200");
  const [error, setError] = useState<string | null>(null);
  const boxRef = useRef<HTMLDivElement>(null);
  const esRef = useRef<EventSource | null>(null);

  useEffect(() => {
    // Intentional: reset + resubscribe the SSE stream when its inputs change.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setLines([]);
    setError(null);
    if (!live || !container) return;
    const url =
      `${API_BASE}/server-hub/api/containers/${encodeURIComponent(container)}` +
      `/logs?tail=${tail}&follow=1`;
    const es = new EventSource(url, { withCredentials: true });
    esRef.current = es;
    es.onmessage = (e) => {
      const chunk = String(e.data ?? "").split("\n").filter((l) => l.length > 0);
      if (chunk.length === 0) return;
      setLines((prev) => [...prev.slice(-MAX_LINES), ...chunk].slice(-MAX_LINES));
    };
    es.onerror = () => {
      setError("Stream interrupted — the shipyard may be unreachable. Resume to retry.");
      es.close();
    };
    return () => {
      es.close();
      esRef.current = null;
    };
  }, [container, tail, live]);

  useEffect(() => {
    const el = boxRef.current;
    if (el && live) el.scrollTop = el.scrollHeight;
  }, [lines, live]);

  const q = query.trim().toLowerCase();
  const visible = lines.filter((l) => {
    if (level !== "ALL" && levelOf(l) !== level) return false;
    if (q && !l.toLowerCase().includes(q)) return false;
    return true;
  });

  const selectCls =
    "bg-white dark:bg-panel border border-line dark:border-edge rounded-input font-body text-[12px] text-ink dark:text-bone px-2 py-1.5 outline-none focus:border-accent dark:focus:border-ember cursor-pointer";
  const btnCls =
    "inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs";

  return (
    <div>
      <div className="flex items-center gap-2 flex-wrap mb-2.5">
        <button className={btnCls} onClick={() => setLive((v) => !v)}>
          {live ? <Pause size={12} /> : <Play size={12} />}
          {live ? "Pause" : "Resume"}
        </button>
        <select
          className={selectCls}
          value={tail}
          onChange={(e) => setTail(e.target.value)}
          aria-label="Tail lines"
          title="Tail lines"
        >
          <option value="100">tail 100</option>
          <option value="500">tail 500</option>
          <option value="1000">tail 1000</option>
        </select>
        <select
          className={selectCls}
          value={level}
          onChange={(e) => setLevel(e.target.value as Level)}
          aria-label="Log level"
          title="Log level"
        >
          <option value="ALL">ALL</option>
          <option value="INFO">INFO</option>
          <option value="WARN">WARN</option>
          <option value="ERROR">ERROR</option>
          <option value="DEBUG">DEBUG</option>
        </select>
        <input
          className="w-full sm:w-auto sm:flex-1 bg-white dark:bg-panel border border-line dark:border-edge rounded-input font-body text-[12px] text-ink dark:text-bone px-2.5 py-1.5 outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog"
          placeholder="Search logs…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <button className={btnCls} onClick={() => setLines([])} title="Clear view">
          <Trash2 size={12} /> Clear
        </button>
        <span className="font-mono text-[11px] text-muted dark:text-fog ml-auto">
          {visible.length}/{lines.length}
        </span>
      </div>

      {error && (
        <div className="bg-red-50 dark:bg-red-950 border border-red-200 dark:border-red-900 text-red-800 dark:text-red-200 rounded-input px-3 py-2.5 text-[13px] mb-2.5">
          {error}
        </div>
      )}

      <div
        ref={boxRef}
        className="font-mono text-[11px] leading-[1.7] bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-3 h-[320px] overflow-y-auto whitespace-pre-wrap break-words"
      >
        {visible.length === 0 && (
          <span className="text-muted dark:text-fog">
            {live ? "— listening on the wire —" : "— paused —"}
          </span>
        )}
        {visible.map((l, i) => (
          <div key={`${i}-${l.length}`} className={LEVEL_CLS[levelOf(l)]}>
            {l}
          </div>
        ))}
      </div>

      <div className="mt-2">
        <Kicker>
          {live ? "LIVE" : "PAUSED"} · {container}
        </Kicker>
      </div>
    </div>
  );
}
