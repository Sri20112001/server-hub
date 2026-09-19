import { useEffect, useRef, useState } from "react";
import { SquareTerminal, X } from "lucide-react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { API_BASE, api } from "../../lib/api";
import type { Service } from "../../lib/types";
import { ConfirmModal, Field, Kicker } from "../../components/ui";

function wsBase(): string {
  return API_BASE.replace(/^http/, "ws");
}

export function TerminalModal({
  projectName,
  services,
  initialContainer,
  onClose,
}: {
  projectName: string;
  services: Service[];
  initialContainer?: string;
  onClose: () => void;
}) {
  const candidates = services.filter((s) => s.containerName);
  const [container, setContainer] = useState(
    initialContainer ?? candidates[0]?.containerName ?? "",
  );
  const [shell, setShell] = useState("/bin/sh");
  const [confirmed, setConfirmed] = useState(false);
  const [status, setStatus] = useState("connecting");
  const [error, setError] = useState<string | null>(null);
  const boxRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<Terminal | null>(null);

  useEffect(() => {
    if (!confirmed || !container) return;
    const box = boxRef.current;
    if (!box) return;

    const term = new Terminal({
      fontFamily: "'JetBrains Mono', ui-monospace, monospace",
      fontSize: 13,
      cursorBlink: true,
      theme: {
        background: "#171512",
        foreground: "#EDE8E4",
        cursor: "#EA580C",
        selectionBackground: "rgba(234, 88, 12, 0.35)",
      },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(box);
    termRef.current = term;
    setStatus("connecting");
    setError(null);

    let ws: WebSocket | null = null;
    let alive = true;

    (async () => {
      try {
        const grant = await api.execToken(container, shell);
        if (!alive) return;
        try {
          fit.fit();
        } catch {
          /* sized on first frame */
        }
        const dims = fit.proposeDimensions();
        ws = new WebSocket(`${wsBase()}/server-hub/api/exec/${grant.token}`);
        ws.binaryType = "arraybuffer";
        ws.onopen = () => {
          if (!alive) return;
          setStatus("connected");
          if (dims) ws?.send(JSON.stringify({ cols: dims.cols, rows: dims.rows }));
          term.focus();
        };
        ws.onmessage = (ev) => {
          if (!alive) return;
          const data = ev.data as string | ArrayBuffer;
          if (typeof data === "string") {
            term.write(data);
          } else {
            new Blob([data]).text().then((t) => {
              if (alive) term.write(t);
            });
          }
        };
        const end = () => {
          if (!alive) return;
          setStatus("ended");
          term.write("\r\n\x1b[33m[session ended]\x1b[0m\r\n");
        };
        ws.onclose = end;
        ws.onerror = () => {
          if (!alive) return;
          setError("Socket error — the session dropped.");
        };
        term.onData((chunk) => {
          if (ws && ws.readyState === WebSocket.OPEN) ws.send(chunk);
        });
      } catch (e) {
        if (!alive) return;
        setError(e instanceof Error ? e.message : "Could not open terminal");
        setStatus("ended");
      }
    })();

    const onResize = () => {
      try {
        fit.fit();
        const dims = fit.proposeDimensions();
        if (dims && ws && ws.readyState === WebSocket.OPEN) {
          ws.send(JSON.stringify({ cols: dims.cols, rows: dims.rows }));
        }
      } catch {
        /* ignore */
      }
    };
    window.addEventListener("resize", onResize);

    return () => {
      alive = false;
      window.removeEventListener("resize", onResize);
      try {
        ws?.close();
      } catch {
        /* ignore */
      }
      term.dispose();
      termRef.current = null;
    };
  }, [confirmed, container, shell]);

  if (!confirmed) {
    return (
      <ConfirmModal
        title={`Open a shell in ${container || "this station"}?`}
        body="This opens an interactive shell INSIDE the container via Docker exec. It never touches the host shell. The session is audit-logged as a high-risk operation."
        confirmLabel="Open terminal"
        onClose={onClose}
        onConfirm={() => setConfirmed(true)}
      />
    );
  }

  const selectCls =
    "bg-white dark:bg-panel border border-line dark:border-edge rounded-input font-body text-[12px] text-ink dark:text-bone px-2 py-1.5 outline-none focus:border-accent dark:focus:border-ember cursor-pointer";

  return (
    <div
      className="fixed inset-0 z-[60] flex items-center justify-center px-4 bg-[rgba(28,25,23,0.28)] dark:bg-[rgba(0,0,0,0.55)]"
      onClick={onClose}
    >
      <div
        className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card w-full max-w-[860px] p-5 shadow-chrome"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between gap-4 mb-3">
          <div className="flex items-center gap-2 flex-wrap">
            <SquareTerminal size={16} />
            <h3 className="text-[18px]">
              {projectName} / terminal
            </h3>
            <span
              className={`inline-flex items-center gap-1.5 h-6 px-2.5 rounded-full font-mono text-[11px] border whitespace-nowrap ${
                status === "connected"
                  ? "bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone"
                  : "bg-paper dark:bg-abyss border-line dark:border-edge text-muted dark:text-fog"
              }`}
            >
              <span
                className={`w-1.5 h-1.5 rounded-full ${
                  status === "connected" ? "bg-moss" : "bg-stone"
                }`}
              />
              {status}
            </span>
          </div>
          <div className="flex items-center gap-2 flex-wrap">
            {candidates.length > 1 && (
              <select
                className={selectCls}
                value={container}
                onChange={(e) => setContainer(e.target.value)}
                aria-label="Station"
              >
                {candidates.map((s) => (
                  <option key={s.id} value={s.containerName}>
                    {s.name}
                  </option>
                ))}
              </select>
            )}
            <button
              className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs"
              onClick={onClose}
              aria-label="Close terminal"
            >
              <X size={14} />
            </button>
          </div>
        </div>

        <div className="flex items-center gap-2 flex-wrap mb-2.5">
          <Field label="Shell">
            <select
              className={selectCls}
              value={shell}
              onChange={(e) => setShell(e.target.value)}
              aria-label="Shell"
            >
              <option value="/bin/sh">/bin/sh</option>
              <option value="/bin/bash">/bin/bash</option>
            </select>
          </Field>
          <span className="font-mono text-[11px] text-muted dark:text-fog">
            session is audit-logged
          </span>
        </div>

        {error && (
          <div className="bg-red-50 dark:bg-red-950 border border-red-200 dark:border-red-900 text-red-800 dark:text-red-200 rounded-input px-3 py-2.5 text-[13px] mb-2.5">
            {error}
          </div>
        )}

        <div
          ref={boxRef}
          className="rounded-xl overflow-hidden border border-line dark:border-edge h-[420px] p-2 bg-[#171512]"
        />
        <div className="mt-2">
          <Kicker>PTY · docker exec · single-use token · closes with the modal</Kicker>
        </div>
      </div>
    </div>
  );
}
