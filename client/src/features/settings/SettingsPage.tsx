import { useEffect, useState } from "react";
import { BellRing, Check, Eye, EyeOff, Send, TriangleAlert } from "lucide-react";
import { api } from "../../lib/api";
import type { NotifySettings, ServerInfo } from "../../lib/types";
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
  const [notify, setNotify] = useState<NotifySettings | null>(null);
  const [ntBusy, setNtBusy] = useState(false);
  const [ntTest, setNtTest] = useState<{ telegram: string; email: string } | null>(null);
  const [tgToken, setTgToken] = useState("");
  const [smtpPass, setSmtpPass] = useState("");

  useEffect(() => {
    api
      .server()
      .then((s) => {
        setInfo(s);
        setOnline(true);
      })
      .catch(() => setOnline(false))
      .finally(() => setLoading(false));
    api.notifySettings().then(setNotify).catch(() => setNotify(null));
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

  const patchNotify = (patch: Partial<NotifySettings>) =>
    setNotify((n) => (n ? { ...n, ...patch } : n));

  const saveNotify = async () => {
    if (!notify) return;
    setNtBusy(true);
    try {
      await api.saveNotifySettings({
        enabled: notify.enabled,
        events: notify.events,
        telegram: { enabled: notify.telegram.enabled, chatId: notify.telegram.chatId, token: tgToken || undefined },
        email: {
          enabled: notify.email.enabled,
          host: notify.email.host,
          port: notify.email.port,
          username: notify.email.username,
          from: notify.email.from,
          to: notify.email.to,
          tls: notify.email.tls,
          password: smtpPass || undefined,
        },
      });
      setTgToken("");
      setSmtpPass("");
      setNtTest(null);
      pushToast("Notification routing saved.");
      const fresh = await api.notifySettings();
      setNotify(fresh);
    } catch (err) {
      pushToast(err instanceof Error ? err.message : "Save failed", true);
    } finally {
      setNtBusy(false);
    }
  };

  const probeNotify = async () => {
    try {
      const r = await api.testNotify();
      setNtTest(r);
    } catch (err) {
      pushToast(err instanceof Error ? err.message : "Probe failed", true);
    }
  };

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

      {/* Notifications (server-side: Telegram / email on failures) */}
      <section className={`${"bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5"} mt-4`}>
        <div className="flex items-start justify-between gap-3 mb-4">
          <div>
            <h2 className="text-lg">Notifications</h2>
            <p className="text-muted dark:text-fog text-[13px]">Server-side signals to Telegram and email when things break — no browser needed.</p>
          </div>
          {notify && (
            <Toggle checked={notify.enabled} onChange={() => patchNotify({ enabled: !notify.enabled })} label="Notifications master switch" />
          )}
        </div>
        {!notify ? (
          <p className="text-muted dark:text-fog text-[13px]">Loading notification settings…</p>
        ) : (
          <>
            <AlertRow
              title="Deploy failed"
              hint="Ping on failed dispatches"
              on={notify.events.deployFailed}
              onFlip={() => patchNotify({ events: { ...notify.events, deployFailed: !notify.events.deployFailed } })}
            />
            <AlertRow
              title="Resource pressure"
              hint="Ping on CPU/RAM/disk threshold crossings"
              on={notify.events.threshold}
              onFlip={() => patchNotify({ events: { ...notify.events, threshold: !notify.events.threshold } })}
            />
            <AlertRow
              title="Backup failed"
              hint="Ping on failed snapshots and restores"
              on={notify.events.backupFailed}
              onFlip={() => patchNotify({ events: { ...notify.events, backupFailed: !notify.events.backupFailed } })}
            />
            <AlertRow
              title="Project action failed"
              hint="Ping on failed wake/nap/restart"
              on={notify.events.projectFailed}
              onFlip={() => patchNotify({ events: { ...notify.events, projectFailed: !notify.events.projectFailed } })}
              last
            />

            <h3 className="font-medium text-sm mt-5 mb-2">Telegram</h3>
            <div className="flex items-center justify-between gap-4 py-2">
              <span className="text-[13px] text-muted dark:text-fog">Enabled (needs bot token + chat id)</span>
              <Toggle
                checked={notify.telegram.enabled}
                onChange={() => patchNotify({ telegram: { ...notify.telegram, enabled: !notify.telegram.enabled } })}
                label="Telegram channel"
              />
            </div>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 mt-1">
              <Field label="Bot token">
                <input
                  className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-input font-body text-[13px] text-ink dark:text-bone px-3 py-2.5 outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog font-mono"
                  type="password"
                  value={tgToken}
                  onChange={(e) => setTgToken(e.target.value)}
                  placeholder={notify.telegram.hasToken ? "Stored — leave blank to keep" : "123456:ABC-DEF…"}
                  autoComplete="new-password"
                />
              </Field>
              <Field label="Chat id">
                <input
                  className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-input font-body text-[13px] text-ink dark:text-bone px-3 py-2.5 outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog font-mono"
                  value={notify.telegram.chatId}
                  onChange={(e) => patchNotify({ telegram: { ...notify.telegram, chatId: e.target.value } })}
                  placeholder="123456789 (from @userinfobot)"
                  autoComplete="off"
                />
              </Field>
            </div>

            <h3 className="font-medium text-sm mt-5 mb-2">Email (SMTP)</h3>
            <div className="flex items-center justify-between gap-4 py-2">
              <span className="text-[13px] text-muted dark:text-fog">Enabled</span>
              <Toggle
                checked={notify.email.enabled}
                onChange={() => patchNotify({ email: { ...notify.email, enabled: !notify.email.enabled } })}
                label="Email channel"
              />
            </div>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 mt-1">
              <Field label="SMTP host">
                <input
                  className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-input font-body text-[13px] text-ink dark:text-bone px-3 py-2.5 outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog font-mono"
                  value={notify.email.host}
                  onChange={(e) => patchNotify({ email: { ...notify.email, host: e.target.value } })}
                  placeholder="smtp.gmail.com"
                  autoComplete="off"
                />
              </Field>
              <Field label="Port">
                <input
                  className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-input font-body text-[13px] text-ink dark:text-bone px-3 py-2.5 outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog font-mono"
                  value={notify.email.port}
                  onChange={(e) => patchNotify({ email: { ...notify.email, port: e.target.value } })}
                  placeholder="587"
                  autoComplete="off"
                />
              </Field>
              <Field label="Username">
                <input
                  className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-input font-body text-[13px] text-ink dark:text-bone px-3 py-2.5 outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog font-mono"
                  value={notify.email.username}
                  onChange={(e) => patchNotify({ email: { ...notify.email, username: e.target.value } })}
                  placeholder="you@example.com"
                  autoComplete="off"
                />
              </Field>
              <Field label="Password">
                <input
                  className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-input font-body text-[13px] text-ink dark:text-bone px-3 py-2.5 outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog font-mono"
                  type="password"
                  value={smtpPass}
                  onChange={(e) => setSmtpPass(e.target.value)}
                  placeholder={notify.email.hasPassword ? "Stored — leave blank to keep" : "app password"}
                  autoComplete="new-password"
                />
              </Field>
              <Field label="From">
                <input
                  className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-input font-body text-[13px] text-ink dark:text-bone px-3 py-2.5 outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog font-mono"
                  value={notify.email.from}
                  onChange={(e) => patchNotify({ email: { ...notify.email, from: e.target.value } })}
                  placeholder="serverhub@example.com"
                  autoComplete="off"
                />
              </Field>
              <Field label="To">
                <input
                  className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-input font-body text-[13px] text-ink dark:text-bone px-3 py-2.5 outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog font-mono"
                  value={notify.email.to}
                  onChange={(e) => patchNotify({ email: { ...notify.email, to: e.target.value } })}
                  placeholder="you@example.com"
                  autoComplete="off"
                />
              </Field>
            </div>
            <div className="flex items-center justify-between gap-4 py-2 mt-1">
              <span className="text-[13px] text-muted dark:text-fog">Use TLS (off = plain, on = STARTTLS or :465 implicit)</span>
              <Toggle
                checked={notify.email.tls}
                onChange={() => patchNotify({ email: { ...notify.email, tls: !notify.email.tls } })}
                label="SMTP TLS"
              />
            </div>

            <div className="flex items-center gap-2 flex-wrap mt-4">
              <button
                className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-accent dark:bg-ember text-white dark:text-black hover:bg-accent-hover dark:hover:bg-ember-hover"
                disabled={ntBusy}
                onClick={() => void saveNotify()}
              >
                <Check size={13} /> {ntBusy ? "Saving…" : "Save routing"}
              </button>
              <button
                className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss"
                onClick={() => void probeNotify()}
              >
                <Send size={13} /> Send test
              </button>
            </div>
            {ntTest && (
              <p className="font-mono text-xs text-muted dark:text-fog mt-2">
                telegram: {ntTest.telegram} · email: {ntTest.email}
              </p>
            )}
          </>
        )}
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
