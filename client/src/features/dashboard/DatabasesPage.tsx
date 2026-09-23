import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router";
import { ArrowLeft, Database, KeyRound, PlusCircle, RefreshCw, Server } from "lucide-react";
import { api } from "../../lib/api";
import { fmtBytes } from "../../lib/format";
import type { DbItem, DbServerInfo, DbTarget, Project } from "../../lib/types";
import { useUi } from "../../stores/store";
import { DashboardSkeleton, EmptyState, Kicker } from "../../components/ui";

const chip =
  "inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap";
const btnGhost =
  "inline-flex items-center gap-2 rounded-input font-medium cursor-pointer whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss px-3 py-1.5 text-xs";
const btnAccent =
  "inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-accent dark:bg-ember text-white dark:text-black hover:bg-accent-hover dark:hover:bg-ember-hover";
const inputCls =
  "bg-white dark:bg-panel border border-line dark:border-edge rounded-input px-3 py-2 text-[13px] text-ink dark:text-bone outline-none w-full focus:border-accent dark:focus:border-ember";

function serverTarget(s: DbServerInfo): DbTarget {
  return { engine: s.engine, source: s.source, host: s.host, port: s.port, container: s.container };
}

// Databases: servers are auto-detected (Docker images, host ports, fleet
// services). Pick a server, browse what's inside it, tick the databases you
// need, register them as services under a project. Credentials are asked
// only when the server demands them and are stored encrypted.
export function DatabasesPage({ setOnline }: { setOnline: (v: boolean) => void }) {
  const pushToast = useUi((s) => s.pushToast);
  const nav = useNavigate();
  const [servers, setServers] = useState<DbServerInfo[] | null>(null);
  const [dockerAvailable, setDockerAvailable] = useState(true);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [dbs, setDbs] = useState<DbItem[] | null>(null);
  const [browsing, setBrowsing] = useState(false);
  const [authRequired, setAuthRequired] = useState(false);
  const [browseErr, setBrowseErr] = useState<string | null>(null);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [checked, setChecked] = useState<Set<string>>(new Set());
  const [projects, setProjects] = useState<Project[]>([]);
  const [projectId, setProjectId] = useState<string>("");
  const [registering, setRegistering] = useState(false);

  const loadServers = useCallback(async () => {
    try {
      const r = await api.dbServers();
      setServers(r.servers);
      setDockerAvailable(r.dockerAvailable);
      setOnline(true);
    } catch {
      setServers([]);
    }
  }, [setOnline]);

  useEffect(() => {
    void loadServers();
    api.projects().then(setProjects).catch(() => setProjects([]));
  }, [loadServers]);

  const selected = useMemo(
    () => servers?.find((s) => s.key === selectedKey) ?? null,
    [servers, selectedKey],
  );

  const browse = useCallback(
    async (srv: DbServerInfo, user?: string, pass?: string) => {
      setBrowsing(true);
      setBrowseErr(null);
      setAuthRequired(false);
      try {
        const t = { ...serverTarget(srv), username: user ?? "", password: pass ?? "" };
        const r = await api.browseDatabases(t);
        setDbs(r.databases);
        setChecked(new Set());
      } catch (e) {
        const msg = e instanceof Error ? e.message : "Browse failed";
        if (msg.toLowerCase().includes("password") || msg.toLowerCase().includes("username") || msg.toLowerCase().includes("auth")) {
          setAuthRequired(true);
          setDbs(null);
        } else {
          setBrowseErr(msg);
          setDbs(null);
        }
      } finally {
        setBrowsing(false);
      }
    },
    [],
  );

  const pick = (srv: DbServerInfo) => {
    setSelectedKey(srv.key);
    setUsername("");
    setPassword("");
    setDbs(null);
    void browse(srv);
  };

  const connectAndBrowse = async () => {
    if (!selected) return;
    setBrowsing(true);
    setBrowseErr(null);
    try {
      await api.connectDatabase({ ...serverTarget(selected), username, password });
      pushToast(`Connected to “${selected.name}”. Credential saved.`);
      await loadServers();
      await browse(selected, username, password);
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Connect failed";
      if (msg.toLowerCase().includes("password") || msg.toLowerCase().includes("username") || msg.toLowerCase().includes("auth") || msg.toLowerCase().includes("invalid")) {
        setAuthRequired(true);
        pushToast(msg, true);
      } else {
        setBrowseErr(msg);
      }
    } finally {
      setBrowsing(false);
    }
  };

  const toggle = (name: string) => {
    setChecked((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  };

  const tickable = useMemo(() => (dbs ?? []).filter((d) => !d.registered), [dbs]);
  const allTicked = tickable.length > 0 && tickable.every((d) => checked.has(d.name));

  const register = async () => {
    if (!selected || checked.size === 0 || !projectId) return;
    setRegistering(true);
    try {
      const r = await api.registerDatabases(
        Number(projectId),
        serverTarget(selected),
        [...checked],
        username || undefined,
        password || undefined,
      );
      const names = r.registered.map((x) => x.database).join(", ");
      pushToast(
        r.registered.length === 0
          ? "Nothing new — already registered."
          : `${r.registered.length} database${r.registered.length === 1 ? "" : "s"} registered: ${names}.`,
      );
      setChecked(new Set());
      await loadServers();
      await browse(selected, username || undefined, password || undefined);
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Register failed", true);
    } finally {
      setRegistering(false);
    }
  };

  if (servers === null) {
    return <DashboardSkeleton />;
  }

  return (
    <main className="w-full max-w-7xl mx-auto px-10 max-md:px-4 pt-24 pb-36">
      <div className="flex items-end justify-between gap-4 flex-wrap">
        <div>
          <button
            className="inline-flex items-center gap-1.5 bg-transparent border-0 cursor-pointer text-muted dark:text-fog hover:text-ink dark:hover:text-bone text-[13px] mb-2 px-0"
            onClick={() => nav("/")}
          >
            <ArrowLeft size={14} /> Dashboard
          </button>
          <h1 className="font-head text-[32px] font-bold tracking-[-0.03em] leading-[1.2] max-md:text-[26px]">
            Databases
          </h1>
          <p className="text-muted dark:text-fog mt-1.5">
            <Kicker>
              {servers.length} server{servers.length === 1 ? "" : "s"} detected
              {!dockerAvailable && " · Docker unreachable"}
            </Kicker>
          </p>
        </div>
        <button className={btnGhost} onClick={() => void loadServers()}>
          <RefreshCw size={13} /> Re-scan
        </button>
      </div>

      {servers.length === 0 ? (
        <div className="mt-6">
          <EmptyState
            title="No database servers found"
            hint="Start a Postgres, MySQL, Redis or Mongo container — or expose one on localhost — and hit Re-scan."
          />
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-[320px_1fr] mt-6 items-start">
          {/* servers */}
          <div className="flex flex-col gap-3">
            {servers.map((s) => (
              <button
                key={s.key}
                onClick={() => pick(s)}
                className={`text-left bg-white dark:bg-panel border rounded-card p-4 cursor-pointer transition-colors duration-150 w-full ${
                  s.key === selectedKey
                    ? "border-accent dark:border-ember"
                    : "border-line dark:border-edge hover:border-accent dark:hover:border-ember"
                }`}
              >
                <div className="flex items-center gap-2 flex-wrap">
                  <Server size={14} className="text-muted dark:text-fog" />
                  <span className="font-head font-bold text-[15px]">{s.name}</span>
                </div>
                <div className="flex items-center gap-1.5 flex-wrap mt-2">
                  <span className={chip}>{s.engine}</span>
                  <span className={chip}>{s.source}</span>
                  {s.version && <span className={chip}>{s.version}</span>}
                  {s.hasCreds && (
                    <span className={chip} title="Credential saved">
                      <KeyRound size={11} /> saved
                    </span>
                  )}
                </div>
                <div className="text-muted dark:text-fog text-xs mt-2 font-mono">
                  {s.container || `${s.host}:${s.port}`} · {s.state}
                  {s.registered.length > 0 && ` · ${s.registered.length} registered`}
                </div>
              </button>
            ))}
          </div>

          {/* databases */}
          <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5 min-h-[280px]">
            {!selected ? (
              <EmptyState
                title="Pick a server"
                hint="Select a detected server on the left to browse the databases inside it."
              />
            ) : (
              <>
                <div className="flex items-center justify-between gap-3 flex-wrap mb-4">
                  <div className="flex items-center gap-2 flex-wrap">
                    <Database size={15} />
                    <h3 className="font-head font-bold text-[17px]">{selected.name}</h3>
                  </div>
                  <button className={btnGhost} disabled={browsing} onClick={() => void browse(selected, username || undefined, password || undefined)}>
                    <RefreshCw size={12} /> {browsing ? "Browsing…" : "Refresh"}
                  </button>
                </div>

                {browsing && dbs === null && !authRequired && !browseErr && (
                  <div className="text-muted dark:text-fog text-[13px]">Reading databases…</div>
                )}

                {browseErr && (
                  <div className="bg-red-50 dark:bg-red-950 border border-red-200 dark:border-red-900 text-red-800 dark:text-red-200 rounded-input px-3 py-2.5 text-[13px] mb-4">
                    {browseErr}
                  </div>
                )}

                {(authRequired || selected.hasCreds || username || password) && (
                  <div className="grid grid-cols-1 sm:grid-cols-[1fr_1fr_auto] gap-2 mb-4 items-end">
                    <label className="flex flex-col gap-1 text-xs text-muted dark:text-fog">
                      Username
                      <input className={inputCls} value={username} onChange={(e) => setUsername(e.target.value)} placeholder={selected.engine === "postgres" ? "postgres" : selected.engine === "mysql" ? "root" : "optional"} autoComplete="off" />
                    </label>
                    <label className="flex flex-col gap-1 text-xs text-muted dark:text-fog">
                      Password
                      <input className={inputCls} type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="••••••" autoComplete="new-password" />
                    </label>
                    <button className={btnGhost} disabled={browsing} onClick={() => void connectAndBrowse()}>
                      <KeyRound size={12} /> {browsing ? "Connecting…" : "Save & connect"}
                    </button>
                  </div>
                )}
                {authRequired && dbs === null && (
                  <div className="text-muted dark:text-fog text-[13px] mb-4">
                    This server needs a username/password. Enter it above — it will be verified and stored encrypted.
                  </div>
                )}

                {dbs !== null && (
                  <>
                    {dbs.length === 0 ? (
                      <EmptyState title="No databases" hint="The server answered but reported no databases." />
                    ) : (
                      <>
                        <label className="flex items-center gap-2 text-[13px] mb-2 cursor-pointer select-none">
                          <input
                            type="checkbox"
                            checked={allTicked}
                            onChange={() =>
                              setChecked(allTicked ? new Set() : new Set(tickable.map((d) => d.name)))
                            }
                          />
                          Select all ({tickable.length} available)
                        </label>
                        <div className="flex flex-col gap-2 max-h-[42vh] overflow-y-auto pr-1">
                          {dbs.map((d) => (
                            <label
                              key={d.name}
                              className={`flex items-center gap-3 bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-2.5 text-[13px] ${
                                d.registered ? "opacity-70" : "cursor-pointer"
                              }`}
                            >
                              <input
                                type="checkbox"
                                checked={d.registered || checked.has(d.name)}
                                disabled={d.registered}
                                onChange={() => toggle(d.name)}
                              />
                              <span className="font-mono font-medium">{d.name}</span>
                              {d.system && <span className={chip}>system</span>}
                              {d.registered ? (
                                <span className={chip}>registered</span>
                              ) : (
                                <span className="ml-auto font-mono text-muted dark:text-fog text-xs">
                                  {d.sizeBytes < 0
                                    ? "—"
                                    : selected.engine === "redis"
                                      ? `${d.sizeBytes} keys`
                                      : fmtBytes(d.sizeBytes)}
                                </span>
                              )}
                            </label>
                          ))}
                        </div>
                        <div className="flex items-center gap-2 flex-wrap mt-4">
                          <select
                            className={`${inputCls} !w-auto min-w-[180px]`}
                            value={projectId}
                            onChange={(e) => setProjectId(e.target.value)}
                          >
                            <option value="">Target project…</option>
                            {projects.map((p) => (
                              <option key={p.id} value={p.id}>
                                {p.name}
                              </option>
                            ))}
                          </select>
                          <button
                            className={btnAccent}
                            disabled={registering || checked.size === 0 || !projectId}
                            onClick={() => void register()}
                          >
                            <PlusCircle size={14} />{" "}
                            {registering ? "Registering…" : `Register ${checked.size} selected`}
                          </button>
                        </div>
                      </>
                    )}
                  </>
                )}
              </>
            )}
          </div>
        </div>
      )}
    </main>
  );
}
