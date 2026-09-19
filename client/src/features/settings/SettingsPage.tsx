import { useEffect, useState } from "react";
import { BellRing, Check, Eye, EyeOff, TriangleAlert } from "lucide-react";
import { api } from "../../lib/api";
import type { ServerInfo } from "../../lib/types";
import { fmtUptime } from "../../lib/format";
import { useAuth, useUi } from "../../stores/store";
import { ConfirmModal, Field, Kicker, Toggle, SettingsSkeleton } from "../../components/ui";

const LS_KEY = "serverhub.alerts";

interface Alerts {
  deployFinished: boolean;
  healthChanged: boolean;
  highUsage: boolean;
}

function loadAlerts(): Alerts {
  try {
    const raw = localStorage.getItem(LS_KEY);
    if (raw) return { deployFinished: true, healthChanged: true, highUsage: false, ...JSON.parse(raw) };
  } catch {
    /* fall through */
  }
  return { deployFinished: true, healthChanged: true, highUsage: false };
}

export function SettingsPage({ setOnline }: { setOnline: (v: boolean) => void }) {
  const { user } = useAuth();
  const pushToast = useUi((s) => s.pushToast);

  const [info, setInfo] = useState<ServerInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [alerts, setAlerts] = useState<Alerts>(loadAlerts);
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [again, setAgain] = useState("");
  const [pwBusy, setPwBusy] = useState(false);
  const [showPw, setShowPw] = useState(false);
  const [confirmWipe, setConfirmWipe] = useState(false);
  const [confirmRestart, setConfirmRestart] = useState(false);

  useEffect(() => {
    api
      .server()
      .then((s) => {
        setInfo(s);
        setOnline(true);
      })
      .catch(() => setOnline(false))
      .finally(() => setLoading(false));
  }, [setOnline]);

  const flip = (key: keyof Alerts) => {
    setAlerts((a) => {
      const nextAlerts = { ...a, [key]: !a[key] };
      localStorage.setItem(LS_KEY, JSON.stringify(nextAlerts));
      return nextAlerts;
    });
    pushToast("Signal routing saved.");
  };

  const changePassword = async (e: React.FormEvent) => {
    e.preventDefault();
    if (next !== again) {
      pushToast("New passwords do not match", true);
      return;
    }
    setPwBusy(true);
    try {
      await api.changePassword(current, next);
      pushToast("Master password rotated.");
      setCurrent("");
      setNext("");
      setAgain("");
    } catch (err) {
      pushToast(err instanceof Error ? err.message : "Rotation failed", true);
    } finally {
      setPwBusy(false);
    }
  };

  const apiUrl = (import.meta.env.VITE_API_URL as string | undefined) ?? "http://localhost:4000";

  if (loading) {
    return <SettingsSkeleton />;
  }

  return (
    <main className="w-full max-w-[760px] mx-auto px-6 max-md:px-4 pt-24 pb-36">
      <Kicker>Server preferences & access</Kicker>
      <h1 className="font-head text-[32px] font-bold tracking-[-0.03em] leading-[1.2] max-md:text-[26px] mt-1.5">
        Settings
      </h1>
      <p className="text-muted dark:text-fog mt-1.5">
        Configure host environment, telemetry notification channels, and operational daemon privileges.
      </p>

      {/* Profile & Access */}
      <section className={`${"bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5"} mt-4`}>
        <div className="flex items-start justify-between gap-3 mb-4">
          <div>
            <h2 className="text-lg">Profile & Access</h2>
            <p className="text-muted dark:text-fog text-[13px]">Manage operator session identity and root authentication vectors.</p>
          </div>
          <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">NODE-01</span>
        </div>

        <div className={`bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-3.5 flex items-center justify-between gap-3 flex-wrap`}>
          <div className="flex items-center gap-3 flex-wrap">
            <span className="w-11 h-11 rounded-full bg-tint dark:bg-emboss border border-line dark:border-edge flex items-center justify-center font-head font-bold text-lg text-accent-deep dark:text-ember">
              {(user?.username ?? "C").slice(0, 1).toUpperCase()}
            </span>
            <div>
              <div className="flex items-center gap-2 flex-wrap">
                <strong className="text-ink dark:text-bone">{user?.username ?? "—"}</strong>
                <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">{user?.role ?? "admin"}</span>
              </div>
              <div className="font-mono text-muted dark:text-fog text-[11px]">single-operator station · cookie session</div>
            </div>
          </div>
          <span className="inline-flex items-center gap-1.5 h-6 px-2.5 rounded-full font-mono text-[11px] bg-white dark:bg-panel border border-line dark:border-edge text-ink dark:text-bone whitespace-nowrap">
            <span className="w-1.5 h-1.5 rounded-full bg-moss" /> Auth: Local
          </span>
        </div>

        <form onSubmit={(e) => void changePassword(e)} className="mt-4 flex flex-col gap-4">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Field label="Current master password">
              <div className="relative">
                <input
                  className={`w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-input font-body text-[13px] text-ink dark:text-bone px-3 py-2.5 outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog pr-10 font-mono`}
                  type={showPw ? "text" : "password"}
                  value={current}
                  onChange={(e) => setCurrent(e.target.value)}
                  autoComplete="current-password"
                />
                <button
                  type="button"
                  className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted dark:text-fog bg-transparent border-0 cursor-pointer flex p-1"
                  onClick={() => setShowPw((v) => !v)}
                  aria-label="Toggle visibility"
                >
                  {showPw ? <EyeOff size={15} /> : <Eye size={15} />}
                </button>
              </div>
            </Field>
            <Field label="New master password (8+ chars)">
              <input
                className={`w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-input font-body text-[13px] text-ink dark:text-bone px-3 py-2.5 outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog font-mono`}
                type={showPw ? "text" : "password"}
                value={next}
                onChange={(e) => setNext(e.target.value)}
                autoComplete="new-password"
              />
            </Field>
          </div>
          <Field label="Confirm new password">
            <input
              className={`w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-input font-body text-[13px] text-ink dark:text-bone px-3 py-2.5 outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog font-mono`}
              type={showPw ? "text" : "password"}
              value={again}
              onChange={(e) => setAgain(e.target.value)}
              autoComplete="new-password"
            />
          </Field>
          <div className="flex items-center justify-end gap-2 flex-wrap">
            <button
              type="submit"
              className={`inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-accent dark:bg-ember text-white dark:text-black hover:bg-accent-hover dark:hover:bg-ember-hover`}
              disabled={pwBusy || !current || next.length < 8 || next !== again}
            >
              <Check size={14} /> {pwBusy ? "Rotating…" : "Save changes"}
            </button>
          </div>
        </form>
      </section>

      {/* Host Configuration */}
      <section className={`${"bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5"} mt-4`}>
        <div className="flex items-start justify-between gap-3 mb-4">
          <div>
            <h2 className="text-lg">Host Configuration</h2>
            <p className="text-muted dark:text-fog text-[13px]">Core runtime parameters and daemon routing points.</p>
          </div>
          <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">SERVERHUB · MVP</span>
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-3.5">
            <Kicker>API endpoint</Kicker>
            <div className="font-mono text-[13px] mt-1.5 truncate">{apiUrl}</div>
          </div>
          <div className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-3.5">
            <div className="flex items-center justify-between gap-4">
              <Kicker>Docker socket</Kicker>
              <span
                className={
                  info?.dockerAvailable
                    ? "w-2 h-2 rounded-full bg-moss shadow-[0_0_0_3px_rgba(22,163,74,0.18)]"
                    : "w-2 h-2 rounded-full bg-stone shadow-[0_0_0_3px_rgba(168,162,158,0.25)]"
                }
              />
            </div>
            <div className="font-mono text-[13px] mt-1.5 truncate">
              {info ? (info.dockerAvailable ? "/var/run/docker.sock · live" : "unreachable") : "probing…"}
            </div>
          </div>
          <div className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-3.5">
            <Kicker>Uptime</Kicker>
            <div className="font-mono text-[13px] mt-1.5">{info ? fmtUptime(info.uptimeSec) : "—"}</div>
          </div>
          <div className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-3.5">
            <Kicker>Fleet census</Kicker>
            <div className="font-mono text-[13px] mt-1.5 truncate">
              {info
                ? `${info.projects} ships · ${info.services} stations · ${
                    info.runningContainers < 0 ? "?" : info.runningContainers
                  } running`
                : "—"}
            </div>
          </div>
        </div>

        <div className={`bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-3.5 mt-4 flex items-center justify-between gap-2 flex-wrap`}>
          <span className="text-[13px] font-medium text-ink dark:text-bone">Heartbeat interval</span>
          <span className="font-mono text-xs text-muted dark:text-fog">
            Every tick <strong className="font-mono text-ink dark:text-bone">30s</strong>
          </span>
        </div>
      </section>

      {/* Alerts & Signals */}
      <section className={`${"bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5"} mt-4`}>
        <div className="flex items-start justify-between gap-3 mb-4">
          <div>
            <h2 className="text-lg">Alerts & Signals</h2>
            <p className="text-muted dark:text-fog text-[13px]">Routing policies for internal bus events and dispatch triggers.</p>
          </div>
          <span className="text-muted dark:text-fog flex">
            <BellRing size={16} />
          </span>
        </div>
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
          last
        />
      </section>

      {/* Danger Zone */}
      <section className={`${"bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5"} bg-butter dark:bg-danger-night mt-4`}>
        <div className="flex items-center gap-2.5 flex-wrap text-brick">
          <TriangleAlert size={18} />
          <h2 className="text-lg font-bold text-ink dark:text-bone">Danger Zone</h2>
        </div>
        <p className="text-muted dark:text-fog text-[13px] mt-1">
          Irreversible operations. Proceed with operational clearance.
        </p>
        <div className="flex items-center gap-3 flex-wrap mt-3">
          <button
            className={`inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-transparent border-brick text-brick hover:bg-red-50 dark:hover:bg-red-950`}
            onClick={() => setConfirmRestart(true)}
          >
            Fresh start (Restart ServerHub)
          </button>
          <button
            className={`inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-transparent border-brick text-brick hover:bg-red-50 dark:hover:bg-red-950`}
            onClick={() => setConfirmWipe(true)}
          >
            Wipe deployment history
          </button>
        </div>
      </section>

      {/* About */}
      <section className={`${"bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5"} mt-4`}>
        <div className="flex items-center justify-between gap-2 flex-wrap">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap">v1.0-mvp</span>
            <span className="font-mono text-xs text-muted dark:text-fog flex items-center gap-1.5">
              <span className="inline-block w-1.5 h-1.5 rounded-full bg-moss shadow-[0_0_0_3px_rgba(22,163,74,0.18)]" />
              bridge manned
            </span>
          </div>
          <em className="text-muted dark:text-fog text-[13px] not-italic">Made for one server.</em>
        </div>
      </section>

      {confirmRestart && (
        <ConfirmModal
          title="Fresh-start ServerHub?"
          body="The bridge cannot restart itself from inside. Run the command below on the host instead — it has been copied to your clipboard."
          confirmLabel="Copy command & close"
          onClose={() => setConfirmRestart(false)}
          onConfirm={() => {
            void navigator.clipboard.writeText("docker compose restart serverhub").catch(() => undefined);
            pushToast("Command copied: docker compose restart serverhub");
            setConfirmRestart(false);
          }}
        />
      )}

      {confirmWipe && (
        <ConfirmModal
          title="Wipe deployment history?"
          body="This deletes every recorded launch. Projects, services and secrets are untouched. This cannot be undone."
          confirmLabel="Wipe history"
          requireText="WIPE"
          onClose={() => setConfirmWipe(false)}
          onConfirm={async () => {
            try {
              const r = await api.wipeDeployments();
              pushToast(`History wiped. ${r.deleted} entries cleared.`);
              setConfirmWipe(false);
            } catch (e) {
              pushToast(e instanceof Error ? e.message : "Wipe failed", true);
            }
          }}
        />
      )}
    </main>
  );
}

function AlertRow({
  title,
  hint,
  on,
  onFlip,
  last,
}: {
  title: string;
  hint: string;
  on: boolean;
  onFlip: () => void;
  last?: boolean;
}) {
  return (
    <div
      className={`flex items-center justify-between gap-4 py-3.5 border-t border-line dark:border-edge ${
        last ? "border-b-0" : ""
      }`}
    >
      <div>
        <div className="font-medium text-sm text-ink dark:text-bone">{title}</div>
        <div className="text-muted dark:text-fog text-[13px]">{hint}</div>
      </div>
      <Toggle checked={on} onChange={onFlip} label={title} />
    </div>
  );
}
