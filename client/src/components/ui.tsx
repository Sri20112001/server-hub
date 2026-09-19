import { useEffect, type ReactNode } from "react";
import { fleetLabel, toFleetStatus, type FleetStatus } from "../lib/types";

/* ---------- primitives following DESIGN.md §Components ---------- */

export function Kicker({ children }: { children: ReactNode }) {
  return (
    <div className="text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog">
      {children}
    </div>
  );
}

const PIP_TONE: Record<FleetStatus, string> = {
  sailing: "bg-moss",
  choppy: "bg-status-amber",
  lost: "bg-brick",
  docked: "bg-stone",
};

export function StatusPill({ status, label }: { status: string; label?: string }) {
  const f: FleetStatus = toFleetStatus(status);
  return (
    <span className="inline-flex items-center gap-1.5 h-6 px-2.5 rounded-full font-mono text-[11px] bg-white dark:bg-panel border border-line dark:border-edge text-ink dark:text-bone whitespace-nowrap">
      <span className={`w-1.5 h-1.5 rounded-full ${PIP_TONE[f]}`} />
      {label ?? fleetLabel[f]}
    </span>
  );
}

/* 5%-bucket widths keep meter fills as static, scannable classes */
const METER_WIDTHS = [
  "w-0", "w-[5%]", "w-[10%]", "w-[15%]", "w-[20%]", "w-[25%]", "w-[30%]",
  "w-[35%]", "w-[40%]", "w-[45%]", "w-1/2", "w-[55%]", "w-[60%]",
  "w-[65%]", "w-[70%]", "w-[75%]", "w-[80%]", "w-[85%]", "w-[90%]",
  "w-[95%]", "w-full",
] as const;

const FILL_TONES = [
  "bg-ink dark:bg-bone",
  "bg-accent dark:bg-ember",
  "bg-brick",
] as const;

export function Meter({
  label,
  pct,
  display,
}: {
  label: string;
  pct: number;
  display: string;
}) {
  const p = Math.max(0, Math.min(100, pct));
  const tone = p >= 90 ? FILL_TONES[2] : p >= 70 ? FILL_TONES[1] : FILL_TONES[0];
  return (
    <div className="my-2">
      <div className="flex justify-between items-baseline text-xs mb-1">
        <span>{label}</span>
        <span className="font-mono">{display}</span>
      </div>
      <div className="h-1.5 rounded-full bg-paper dark:bg-abyss border border-line dark:border-edge overflow-hidden">
        <div className={`h-full rounded-full ${tone} ${METER_WIDTHS[Math.round(p / 5)]}`} />
      </div>
    </div>
  );
}

export function Field({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <div>
      <label className="block text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog mb-1.5">
        {label}
      </label>
      {children}
    </div>
  );
}

export function Toggle({
  checked,
  onChange,
  label,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  label: string;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      className={`w-11 h-6 rounded-full border-0 cursor-pointer relative transition-colors duration-150 shrink-0 ${
        checked ? "bg-accent-deep dark:bg-ember" : "bg-line dark:bg-edge"
      }`}
      onClick={() => onChange(!checked)}
    >
      <span
        className={`absolute top-0.5 left-0.5 w-5 h-5 rounded-full bg-white transition-transform duration-150 shadow-[0_1px_2px_rgba(28,25,23,0.2)] ${
          checked ? "translate-x-5" : "translate-x-0"
        }`}
      />
    </button>
  );
}

export function EmptyState({ title, hint }: { title: string; hint?: string }) {
  return (
    <div className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-8 text-center">
      <div className="font-head font-semibold mb-1">{title}</div>
      {hint && <div className="text-muted dark:text-fog text-[13px]">{hint}</div>}
    </div>
  );
}

/** Generic centered modal. Esc closes. */
export function Modal({ onClose, children }: { onClose: () => void; children: ReactNode }) {
  useEffect(() => {
    const fn = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", fn);
    return () => window.removeEventListener("keydown", fn);
  }, [onClose]);
  return (
    <div
      className="fixed inset-0 z-[60] flex items-start justify-center px-4 pt-[12vh] bg-[rgba(28,25,23,0.28)] dark:bg-[rgba(0,0,0,0.55)]"
      onClick={onClose}
    >
      <div
        className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card w-full max-w-[480px] p-6 shadow-chrome"
        onClick={(e) => e.stopPropagation()}
      >
        {children}
      </div>
    </div>
  );
}

/**
 * High-risk confirmation. When `requireText` is set, the user must type it
 * exactly before the confirm button enables.
 */
export function ConfirmModal({
  title,
  body,
  confirmLabel,
  requireText,
  busy,
  onClose,
  onConfirm,
}: {
  title: string;
  body: string;
  confirmLabel: string;
  requireText?: string;
  busy?: boolean;
  onClose: () => void;
  onConfirm: () => void;
}) {
  return (
    <Modal onClose={onClose}>
      <h3 className="text-[18px] mb-2">{title}</h3>
      <p className="text-muted dark:text-fog text-[13px] mb-3">{body}</p>
      {requireText && (
        <Field label={`Type ${requireText} to confirm`}>
          <input
            className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-input font-body text-[13px] text-ink dark:text-bone px-3 py-2.5 outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog font-mono"
            id="confirm-text"
            placeholder={requireText}
            autoComplete="off"
            onChange={(e) => {
              const btn = document.getElementById("confirm-go") as HTMLButtonElement | null;
              if (btn) btn.disabled = e.target.value !== requireText;
            }}
          />
        </Field>
      )}
      <div className="flex items-center gap-2 flex-wrap justify-end mt-4">
        <button
          className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss"
          onClick={onClose}
          disabled={busy}
        >
          Cancel
        </button>
        <button
          id="confirm-go"
          className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-transparent border-brick text-brick hover:bg-red-50 dark:hover:bg-red-950"
          disabled={busy || !!requireText}
          onClick={onConfirm}
        >
          {busy ? "Working…" : confirmLabel}
        </button>
      </div>
    </Modal>
  );
}

/* ---------- loading skeletons (see design/*loading_skeleton) ---------- */

function Skel({ className = "" }: { className?: string }) {
  return <div aria-hidden="true" className={`skel ${className}`} />;
}

function SyncPill({ text }: { text: string }) {
  return (
    <div className="inline-flex items-center gap-2 bg-white dark:bg-panel border border-line dark:border-edge rounded-full px-3 py-1 mb-3">
      <span className="w-1.5 h-1.5 rounded-full bg-brick" />
      <span className="text-xs text-muted dark:text-fog">{text}</span>
    </div>
  );
}

function StatSkelMeters() {
  return (
    <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5">
      <div className="flex items-start justify-between gap-3 mb-2">
        <Skel className="w-24 h-3 rounded" />
        <Skel className="w-4 h-4 rounded" />
      </div>
      {[0, 1, 2].map((i) => (
        <div key={i} className="my-2">
          <div className="flex justify-between items-baseline mb-1">
            <Skel className="w-10 h-3 rounded" />
            <Skel className="w-8 h-3 rounded" />
          </div>
          <Skel className="h-1.5 rounded-full" />
        </div>
      ))}
      <Skel className="w-28 h-3 rounded mt-3" />
    </div>
  );
}

function StatSkelNumber() {
  return (
    <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5">
      <div className="flex items-start justify-between gap-3 mb-2">
        <Skel className="w-24 h-3 rounded" />
        <Skel className="w-4 h-4 rounded" />
      </div>
      <Skel className="w-16 h-10 rounded-lg mt-2" />
      <Skel className="w-40 max-w-full h-3 rounded mt-2" />
      <Skel className="w-32 h-3 rounded mt-3" />
    </div>
  );
}

function StatSkelHealth() {
  return (
    <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5">
      <div className="flex items-start justify-between gap-3 mb-2">
        <Skel className="w-24 h-3 rounded" />
        <Skel className="w-4 h-4 rounded" />
      </div>
      <div className="flex flex-col gap-2 mt-2">
        {[0, 1, 2].map((i) => (
          <div key={i} className="flex items-center gap-2 flex-wrap justify-between">
            <div className="flex items-center gap-2">
              <span className="w-1.5 h-1.5 rounded-full bg-skel dark:bg-edge" />
              <Skel className="w-20 h-5 rounded-full" />
            </div>
            <Skel className="w-14 h-3 rounded" />
          </div>
        ))}
      </div>
      <Skel className="w-24 h-3 rounded mt-3" />
    </div>
  );
}

function ShipSkel() {
  return (
    <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5">
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-2 flex-wrap">
          <span className="w-2 h-2 rounded-full bg-skel dark:bg-edge" />
          <Skel className="w-32 h-5 rounded" />
        </div>
        <Skel className="w-14 h-5 rounded-md" />
      </div>
      <Skel className="w-44 max-w-full h-3 rounded mt-2" />
      <div className="flex items-center gap-2 flex-wrap mt-2.5">
        <Skel className="w-14 h-5 rounded-full" />
        <Skel className="w-16 h-5 rounded-full" />
        <Skel className="w-12 h-5 rounded-full" />
      </div>
      <div className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-3.5 flex items-center gap-2 flex-wrap justify-between mt-3">
        <div className="flex items-center gap-4 flex-wrap">
          <Skel className="w-12 h-6 rounded" />
          <Skel className="w-12 h-6 rounded" />
        </div>
        <Skel className="w-16 h-5 rounded-md" />
      </div>
      <div className="flex items-center justify-between gap-4 mt-3">
        <Skel className="w-24 h-3 rounded" />
        <Skel className="w-24 h-9 rounded-lg" />
      </div>
    </div>
  );
}

export function DashboardSkeleton() {
  return (
    <main
      className="w-full max-w-[1240px] mx-auto px-10 max-md:px-4 pt-24 pb-36"
      aria-busy="true"
      role="status"
    >
      <span className="sr-only">Loading ServerHub telemetry</span>
      <SyncPill text="Syncing telemetry" />

      <div className="flex items-end justify-between gap-4 flex-wrap">
        <div>
          <Skel className="w-36 h-3 rounded" />
          <Skel className="w-[320px] max-w-full h-10 rounded-lg mt-2" />
          <Skel className="w-[360px] max-w-full h-4 rounded mt-2" />
        </div>
        <div className="flex items-center gap-2 flex-wrap">
          <Skel className="w-32 h-9 rounded-lg" />
          <Skel className="w-32 h-9 rounded-lg" />
        </div>
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4 mt-5">
        <StatSkelMeters />
        <StatSkelNumber />
        <StatSkelNumber />
        <StatSkelHealth />
      </div>

      <div className="flex items-center justify-between gap-4 mt-8 mb-4">
        <div className="flex items-center gap-2 flex-wrap">
          <Skel className="w-24 h-6 rounded" />
          <Skel className="w-28 h-5 rounded-md" />
        </div>
        <Skel className="w-24 h-9 rounded-lg" />
      </div>
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
        {[0, 1, 2, 3, 4, 5].map((i) => (
          <ShipSkel key={i} />
        ))}
      </div>

      <div className="mt-8 mb-4">
        <Skel className="w-40 h-3 rounded" />
        <Skel className="w-56 h-6 rounded mt-2" />
      </div>
      <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-0 overflow-hidden">
        <div className="flex items-center justify-between gap-4 px-4 py-2.5 bg-paper dark:bg-abyss">
          {[0, 1, 2, 3, 4, 5].map((i) => (
            <Skel key={i} className="h-3 rounded w-16 max-md:hidden first:max-md:block" />
          ))}
        </div>
        {[0, 1, 2, 3, 4].map((i) => (
          <div
            key={i}
            className="flex items-center justify-between gap-4 px-4 py-3 border-t border-line dark:border-edge"
          >
            <Skel className="w-40 max-w-[40%] h-5 rounded-full" />
            <Skel className="w-24 h-4 rounded max-md:hidden" />
            <Skel className="w-16 h-4 rounded max-md:hidden" />
            <Skel className="w-20 h-5 rounded-full" />
          </div>
        ))}
        <div className="flex items-center justify-between gap-4 px-4 py-[0.7rem] border-t border-line dark:border-edge">
          <Skel className="w-48 h-3 rounded" />
          <Skel className="w-24 h-8 rounded-lg" />
        </div>
      </div>

      <div className="mt-8">
        <div className="flex items-center justify-between gap-4 mb-3">
          <div className="flex items-center gap-2 flex-wrap">
            <Skel className="w-40 h-5 rounded" />
            <Skel className="w-16 h-5 rounded-md" />
          </div>
          <Skel className="w-20 h-5 rounded-md" />
        </div>
        <div className="font-mono text-xs leading-[1.7] bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-5 py-4">
          <Skel className="h-3 rounded w-full" />
          <Skel className="h-3 rounded w-4/5 mt-2" />
          <Skel className="h-3 rounded w-3/5 mt-2" />
        </div>
      </div>
    </main>
  );
}

export function SettingsSkeleton() {
  return (
    <main
      className="w-full max-w-[760px] mx-auto px-6 max-md:px-4 pt-24 pb-36"
      aria-busy="true"
      role="status"
    >
      <span className="sr-only">Loading settings</span>
      <div className="flex justify-end">
        <div className="inline-flex items-center gap-2 bg-white dark:bg-panel border border-line dark:border-edge rounded-full px-3 py-1 mb-3">
          <span className="w-1.5 h-1.5 rounded-full bg-brick" />
          <span className="text-xs text-muted dark:text-fog">Syncing preferences</span>
        </div>
      </div>
      <Skel className="w-56 h-3 rounded" />
      <Skel className="w-64 h-9 rounded-lg mt-2" />
      <Skel className="w-[420px] max-w-full h-4 rounded mt-2" />

      <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5 mt-5">
        <div className="flex items-start justify-between gap-3 mb-4">
          <div>
            <Skel className="w-32 h-5 rounded" />
            <Skel className="w-64 max-w-full h-3 rounded mt-2" />
          </div>
          <Skel className="w-16 h-5 rounded-md" />
        </div>
        <div className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-3.5 flex items-center justify-between gap-3 flex-wrap">
          <div className="flex items-center gap-3 flex-wrap">
            <span className="w-11 h-11 rounded-full bg-skel dark:bg-edge" />
            <div>
              <Skel className="w-28 h-4 rounded" />
              <Skel className="w-40 h-3 rounded mt-1.5" />
            </div>
          </div>
          <Skel className="w-24 h-6 rounded-full" />
        </div>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 mt-4">
          <div>
            <Skel className="w-32 h-3 rounded mb-1.5" />
            <Skel className="h-10 rounded-input" />
          </div>
          <div>
            <Skel className="w-32 h-3 rounded mb-1.5" />
            <Skel className="h-10 rounded-input" />
          </div>
        </div>
        <div className="flex items-center justify-end gap-2 flex-wrap mt-4">
          <Skel className="w-32 h-9 rounded-input" />
        </div>
      </div>

      <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5 mt-4">
        <div className="flex items-start justify-between gap-3 mb-4">
          <div>
            <Skel className="w-36 h-5 rounded" />
            <Skel className="w-64 max-w-full h-3 rounded mt-2" />
          </div>
          <Skel className="w-20 h-5 rounded-md" />
        </div>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          {[0, 1, 2, 3].map((i) => (
            <div key={i} className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-3.5">
              <Skel className="w-24 h-3 rounded" />
              <Skel className="w-3/4 h-4 rounded mt-2" />
            </div>
          ))}
        </div>
        <div className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-3.5 mt-4 flex items-center justify-between gap-2 flex-wrap">
          <Skel className="w-36 h-4 rounded" />
          <Skel className="w-24 h-4 rounded" />
        </div>
      </div>

      <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5 mt-4">
        <div className="flex items-start justify-between gap-3 mb-4">
          <div>
            <Skel className="w-32 h-5 rounded" />
            <Skel className="w-64 max-w-full h-3 rounded mt-2" />
          </div>
          <Skel className="w-4 h-4 rounded" />
        </div>
        {[0, 1, 2].map((i) => (
          <div
            key={i}
            className="flex items-center justify-between gap-4 py-3.5 border-t border-line dark:border-edge"
          >
            <div>
              <Skel className="w-36 h-4 rounded" />
              <Skel className="w-56 max-w-full h-3 rounded mt-1.5" />
            </div>
            <span className="w-11 h-6 rounded-full border border-line dark:border-edge bg-paper dark:bg-abyss shrink-0" />
          </div>
        ))}
      </div>

      <div className="bg-butter dark:bg-danger-night border border-line dark:border-edge rounded-card p-5 mt-4">
        <Skel className="w-40 h-5 rounded" />
        <Skel className="w-72 max-w-full h-3 rounded mt-2" />
        <div className="flex items-center gap-3 flex-wrap mt-3">
          <Skel className="w-44 h-9 rounded-input" />
          <Skel className="w-44 h-9 rounded-input" />
        </div>
      </div>

      <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5 mt-4">
        <div className="flex items-center justify-between gap-2 flex-wrap">
          <div className="flex items-center gap-2 flex-wrap">
            <Skel className="w-16 h-5 rounded-md" />
            <Skel className="w-28 h-3 rounded" />
          </div>
          <Skel className="w-24 h-3 rounded" />
        </div>
      </div>
    </main>
  );
}
