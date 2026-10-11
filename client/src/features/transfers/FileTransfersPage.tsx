import { useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  ArrowDownUp,
  Copy,
  Database,
  Download,
  File as FileIcon,
  Folder,
  FolderPlus,
  Move,
  Pencil,
  RefreshCw,
  Search,
  Trash2,
  Undo2,
  Upload,
} from "lucide-react";
import { ApiError, transfersApi } from "../../lib/api";
import type { TransferFileItem } from "../../lib/types";
import { useAuth, useUi } from "../../stores/store";

function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return "—";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

function joinPath(dir: string, name: string): string {
  return dir === "/" ? `/${name}` : `${dir.replace(/\/+$/, "")}/${name}`;
}

function parentPath(p: string): string | null {
  if (p === "/" || !p.includes("/")) return null;
  const cut = p.replace(/\/+$/, "").lastIndexOf("/");
  return cut <= 0 ? "/" : p.slice(0, cut);
}

type Tab = "files" | "search" | "trash" | "activity";

type PromptKind = "mkdir" | "rename" | "move" | "copy";
interface PromptState {
  kind: PromptKind;
  item?: TransferFileItem;
  value: string;
  destValue?: string;
}

function errMsg(e: unknown, fallback: string): string {
  return e instanceof ApiError ? e.message : e instanceof Error ? e.message : fallback;
}

export function FileTransfersPage() {
  const { pushToast } = useUi();
  const { user } = useAuth();
  const qc = useQueryClient();
  const canMutate = user?.role === "operator" || user?.role === "admin";

  const [tab, setTab] = useState<Tab>("files");
  const [path, setPath] = useState("/");
  const [query, setQuery] = useState("");
  const [prompt, setPrompt] = useState<PromptState | null>(null);
  const [busy, setBusy] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  const storageQ = useQuery({
    queryKey: ["transfers-storage"],
    queryFn: transfersApi.storage,
    refetchInterval: 60_000,
    retry: 1,
  });
  const breakdownQ = useQuery({
    queryKey: ["transfers-breakdown"],
    queryFn: transfersApi.breakdown,
    refetchInterval: 60_000,
    retry: 1,
  });
  const listQ = useQuery({
    queryKey: ["transfers-list", path],
    queryFn: () => transfersApi.list(path),
    retry: 1,
  });
  const trashQ = useQuery({
    queryKey: ["transfers-trash"],
    queryFn: transfersApi.trash,
    retry: 1,
    enabled: tab === "trash",
  });
  const activityQ = useQuery({
    queryKey: ["transfers-activity"],
    queryFn: () => transfersApi.activity(50),
    refetchInterval: 30_000,
    retry: 1,
    enabled: tab === "activity",
  });
  const searchQ = useQuery({
    queryKey: ["transfers-search", query, path],
    queryFn: () => transfersApi.search(query, "/"),
    enabled: tab === "search" && query.trim().length >= 2,
    retry: 1,
  });

  const disabled =
    (listQ.error instanceof ApiError && listQ.error.status === 404) ||
    (storageQ.error instanceof ApiError && storageQ.error.status === 404);

  const refreshAll = () => {
    void qc.invalidateQueries({ queryKey: ["transfers-list"] });
    void qc.invalidateQueries({ queryKey: ["transfers-storage"] });
    void qc.invalidateQueries({ queryKey: ["transfers-breakdown"] });
    void qc.invalidateQueries({ queryKey: ["transfers-trash"] });
    void qc.invalidateQueries({ queryKey: ["transfers-activity"] });
  };

  async function run<T>(fn: () => Promise<T>, okMsg: string): Promise<T | null> {
    setBusy(true);
    try {
      const out = await fn();
      refreshAll();
      pushToast(okMsg);
      return out;
    } catch (e) {
      pushToast(errMsg(e, okMsg + " (failed)"), true);
      return null;
    } finally {
      setBusy(false);
    }
  }

  const onUpload = async (f: File) => {
    await run(() => transfersApi.upload(path, f), `Uploaded ${f.name}`);
  };

  const submitPrompt = async () => {
    if (!prompt || !canMutate) return;
    const v = prompt.value.trim();
    if (!v) {
      pushToast("Name is required", true);
      return;
    }
    if (prompt.kind === "mkdir") {
      await run(() => transfersApi.mkdir(joinPath(path, v)), `Created ${v}`);
    } else if (prompt.kind === "rename" && prompt.item) {
      await run(() => transfersApi.rename(prompt.item!.path, v), `Renamed to ${v}`);
    } else if (prompt.kind === "move" && prompt.item) {
      const dest = (prompt.destValue ?? path).trim() || path;
      const target = dest.endsWith(prompt.item.name) ? dest : joinPath(dest, prompt.item.name);
      await run(() => transfersApi.move(prompt.item!.path, target), `Moved to ${target}`);
    } else if (prompt.kind === "copy" && prompt.item) {
      const dest = (prompt.destValue ?? path).trim() || path;
      const target = dest.endsWith(prompt.item.name) ? dest : joinPath(dest, prompt.item.name);
      await run(() => transfersApi.copy(prompt.item!.path, target), `Copied to ${target}`);
    }
    setPrompt(null);
  };

  const crumbs = path === "/" ? [] : path.split("/").filter(Boolean);
  const storage = storageQ.data;
  const usedPct =
    storage && storage.total_bytes > 0
      ? Math.min(100, (storage.used_bytes / storage.total_bytes) * 100)
      : null;

  return (
    <main className="w-full max-w-7xl mx-auto px-10 max-md:px-4 pt-24 pb-36">
      <div className="flex items-end justify-between gap-4 flex-wrap">
        <div>
          <h1 className="font-head text-[32px] font-bold tracking-[-0.03em] leading-[1.2] max-md:text-[26px] text-ink dark:text-bone">
            File Transfers
          </h1>
          <p className="text-muted dark:text-fog mt-1.5">
            Browse, upload and organise files on the ServerHub host.
            {!canMutate && " You have read-only access."}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={refreshAll}
            className="px-4 py-2 text-sm text-muted dark:text-fog hover:text-ink dark:hover:text-bone border border-line dark:border-edge rounded-lg cursor-pointer inline-flex items-center gap-1.5"
          >
            <RefreshCw size={14} /> Refresh
          </button>
        </div>
      </div>

      {disabled ? (
        <div className="mt-6 rounded-card border border-line dark:border-edge p-10 text-center">
          <p className="text-sm font-medium text-ink dark:text-bone">File transfers are disabled.</p>
          <p className="text-xs text-muted dark:text-fog mt-1 font-mono">
            Set FILE_STORAGE_ROOT on the server and restart to enable /transfers.
          </p>
        </div>
      ) : (
        <>
          {/* Storage overview */}
          <div className="mt-6 rounded-card border border-line dark:border-edge p-5 shadow-sm">
            <div className="flex items-center gap-2 mb-3">
              <Database size={15} className="text-muted dark:text-fog" />
              <h2 className="text-sm font-semibold text-ink dark:text-bone">Storage</h2>
            </div>
            {storageQ.isLoading ? (
              <p className="text-sm text-muted dark:text-fog">Loading storage…</p>
            ) : storage ? (
              <>
                <p className="text-2xl font-head font-bold text-ink dark:text-bone">
                  {formatBytes(storage.used_bytes)}{" "}
                  <span className="text-sm font-normal text-muted dark:text-fog">
                    of {formatBytes(storage.total_bytes)}
                  </span>
                </p>
                <div className="h-2 mt-3 rounded-full bg-paper dark:bg-emboss overflow-hidden">
                  <div
                    className="h-full rounded-full bg-accent dark:bg-ember"
                    style={{ width: `${usedPct ?? 0}%` }}
                  />
                </div>
                <p className="text-xs text-muted dark:text-fog mt-2 font-mono">
                  {usedPct !== null ? `${usedPct.toFixed(1)}% used` : "Usage unavailable"} ·{" "}
                  {formatBytes(storage.available_bytes)} free
                </p>
                {breakdownQ.data && breakdownQ.data.folders.length > 0 && (
                  <div className="mt-3 flex flex-wrap gap-2">
                    {breakdownQ.data.folders.slice(0, 8).map((f) => (
                      <button
                        key={f.path}
                        onClick={() => {
                          setPath(f.path);
                          setTab("files");
                        }}
                        className="text-xs font-mono px-2 py-1 rounded border border-line dark:border-edge text-muted dark:text-fog hover:text-ink dark:hover:text-bone cursor-pointer"
                        title={`${f.files} files`}
                      >
                        {f.name} · {formatBytes(f.bytes)}
                      </button>
                    ))}
                  </div>
                )}
              </>
            ) : (
              <p className="text-sm text-muted dark:text-fog">
                Storage figures unavailable — {errMsg(storageQ.error, "file service error")}.
              </p>
            )}
          </div>

          {/* Tabs */}
          <div className="mt-4 flex items-center gap-2">
            {(["files", "search", "trash", "activity"] as Tab[]).map((t) => (
              <button
                key={t}
                onClick={() => setTab(t)}
                className={`px-3 py-1.5 text-sm rounded-lg border cursor-pointer capitalize ${
                  tab === t
                    ? "bg-ink dark:bg-bone text-white dark:text-black border-ink dark:border-bone font-semibold"
                    : "border-line dark:border-edge text-muted dark:text-fog hover:text-ink dark:hover:text-bone"
                }`}
              >
                {t}
              </button>
            ))}
          </div>

          {tab === "files" && (
            <div className="mt-4 rounded-card border border-line dark:border-edge shadow-sm overflow-hidden">
              {/* Breadcrumb + toolbar */}
              <div className="p-4 border-b border-line dark:border-edge flex items-center justify-between gap-3 flex-wrap">
                <div className="flex items-center gap-1 text-sm font-mono min-w-0">
                  <button
                    onClick={() => setPath("/")}
                    className={`px-1.5 py-0.5 rounded cursor-pointer ${path === "/" ? "font-bold text-ink dark:text-bone" : "text-muted dark:text-fog hover:text-ink dark:hover:text-bone"}`}
                  >
                    /
                  </button>
                  {crumbs.map((c, i) => {
                    const p = `/${crumbs.slice(0, i + 1).join("/")}`;
                    const last = i === crumbs.length - 1;
                    return (
                      <span key={p} className="flex items-center gap-1 min-w-0">
                        <span className="text-muted dark:text-fog">/</span>
                        <button
                          onClick={() => setPath(p)}
                          className={`px-1 py-0.5 rounded cursor-pointer truncate ${last ? "font-bold text-ink dark:text-bone" : "text-muted dark:text-fog hover:text-ink dark:hover:text-bone"}`}
                        >
                          {c}
                        </button>
                      </span>
                    );
                  })}
                </div>
                <div className="flex items-center gap-2">
                  {parentPath(path) && (
                    <button
                      onClick={() => setPath(parentPath(path)!)}
                      className="px-3 py-1.5 text-xs border border-line dark:border-edge rounded-lg cursor-pointer inline-flex items-center gap-1"
                    >
                      <ArrowLeft size={13} /> Up
                    </button>
                  )}
                  <button
                    disabled={!canMutate || busy}
                    onClick={() => setPrompt({ kind: "mkdir", value: "" })}
                    className="px-3 py-1.5 text-xs border border-line dark:border-edge rounded-lg cursor-pointer inline-flex items-center gap-1 disabled:opacity-50"
                    title={canMutate ? "New folder" : "Operator role required"}
                  >
                    <FolderPlus size={13} /> New folder
                  </button>
                  <button
                    disabled={!canMutate || busy}
                    onClick={() => fileRef.current?.click()}
                    className="px-3 py-1.5 text-xs bg-accent dark:bg-ember text-white dark:text-black font-semibold rounded-lg cursor-pointer inline-flex items-center gap-1 disabled:opacity-50"
                    title={canMutate ? "Upload file" : "Operator role required"}
                  >
                    <Upload size={13} /> Upload
                  </button>
                  <input
                    ref={fileRef}
                    type="file"
                    className="hidden"
                    onChange={(e) => {
                      const f = e.target.files?.[0];
                      e.target.value = "";
                      if (f) void onUpload(f);
                    }}
                  />
                </div>
              </div>
              {/* Listing */}
              {listQ.isLoading ? (
                <p className="p-8 text-center text-sm text-muted dark:text-fog">Loading…</p>
              ) : listQ.isError ? (
                <p className="p-8 text-center text-sm text-brick">
                  {errMsg(listQ.error, "Could not list directory")}
                </p>
              ) : (listQ.data?.items.length ?? 0) === 0 ? (
                <p className="p-8 text-center text-sm text-muted dark:text-fog">
                  Empty folder. Upload a file or create a folder.
                </p>
              ) : (
                <table className="w-full text-sm border-collapse">
                  <thead>
                    <tr className="bg-paper dark:bg-abyss border-b border-line dark:border-edge">
                      {["Name", "Size", "Modified", "Actions"].map((h) => (
                        <th key={h} className="text-left text-muted dark:text-fog text-[11px] px-4 py-2.5 font-semibold uppercase tracking-wider">
                          {h}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {listQ.data!.items.map((it) => (
                      <tr key={it.path} className="border-t border-line dark:border-edge hover:bg-paper/50 dark:hover:bg-abyss/50">
                        <td className="px-4 py-2.5 min-w-0">
                          <button
                            onClick={() => it.type === "directory" && setPath(it.path)}
                            disabled={it.type !== "directory"}
                            className={`flex items-center gap-2 min-w-0 ${it.type === "directory" ? "cursor-pointer hover:underline" : "cursor-default"}`}
                            title={it.path}
                          >
                            {it.type === "directory" ? <Folder size={15} className="shrink-0 text-accent dark:text-ember" /> : <FileIcon size={15} className="shrink-0 text-muted dark:text-fog" />}
                            <span className="truncate font-mono text-xs text-ink dark:text-bone">{it.name}</span>
                          </button>
                        </td>
                        <td className="px-4 py-2.5 font-mono text-xs text-muted dark:text-fog whitespace-nowrap">
                          {it.type === "directory" ? "—" : formatBytes(it.size)}
                        </td>
                        <td className="px-4 py-2.5 font-mono text-xs text-muted dark:text-fog whitespace-nowrap">
                          {it.modified_at ? new Date(it.modified_at).toLocaleString() : "—"}
                        </td>
                        <td className="px-4 py-2.5 whitespace-nowrap">
                          <div className="flex items-center gap-1">
                            {it.type === "file" && (
                              <a
                                href={transfersApi.downloadUrl(it.path)}
                                className="p-1.5 rounded hover:bg-paper dark:hover:bg-abyss text-muted dark:text-fog hover:text-ink dark:hover:text-bone"
                                title="Download"
                              >
                                <Download size={14} />
                              </a>
                            )}
                            <button
                              disabled={!canMutate}
                              onClick={() => setPrompt({ kind: "rename", item: it, value: it.name })}
                              className="p-1.5 rounded hover:bg-paper dark:hover:bg-abyss text-muted dark:text-fog hover:text-ink dark:hover:text-bone disabled:opacity-40 cursor-pointer"
                              title="Rename"
                            >
                              <Pencil size={14} />
                            </button>
                            <button
                              disabled={!canMutate}
                              onClick={() => setPrompt({ kind: "move", item: it, value: it.name, destValue: parentPath(it.path) ?? "/" })}
                              className="p-1.5 rounded hover:bg-paper dark:hover:bg-abyss text-muted dark:text-fog hover:text-ink dark:hover:text-bone disabled:opacity-40 cursor-pointer"
                              title="Move"
                            >
                              <Move size={14} />
                            </button>
                            <button
                              disabled={!canMutate}
                              onClick={() => setPrompt({ kind: "copy", item: it, value: it.name, destValue: parentPath(it.path) ?? "/" })}
                              className="p-1.5 rounded hover:bg-paper dark:hover:bg-abyss text-muted dark:text-fog hover:text-ink dark:hover:text-bone disabled:opacity-40 cursor-pointer"
                              title="Copy"
                            >
                              <Copy size={14} />
                            </button>
                            <button
                              disabled={!canMutate}
                              onClick={() => void run(() => transfersApi.trashMove(it.path), `Moved ${it.name} to trash`)}
                              className="p-1.5 rounded hover:bg-paper dark:hover:bg-abyss text-muted dark:text-fog hover:text-status-amber disabled:opacity-40 cursor-pointer"
                              title="Move to trash"
                            >
                              <Trash2 size={14} />
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          )}

          {tab === "search" && (
            <div className="mt-4 rounded-card border border-line dark:border-edge p-5 shadow-sm">
              <div className="flex items-center gap-2">
                <div className="relative flex-1">
                  <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted dark:text-fog" />
                  <input
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                    placeholder="Search by file name (min 2 chars)…"
                    className="w-full pl-9 pr-3 py-2 text-sm bg-white dark:bg-panel border border-line dark:border-edge rounded-lg outline-none focus:border-accent dark:focus:border-ember text-ink dark:text-bone"
                  />
                </div>
              </div>
              {searchQ.isFetching ? (
                <p className="text-sm text-muted dark:text-fog py-6 text-center">Searching…</p>
              ) : searchQ.data ? (
                searchQ.data.results.length === 0 ? (
                  <p className="text-sm text-muted dark:text-fog py-6 text-center">No matches for “{searchQ.data.query}”.</p>
                ) : (
                  <div className="mt-3 divide-y divide-line dark:divide-edge">
                    {searchQ.data.results.map((r) => (
                      <div key={r.path} className="py-2 flex items-center justify-between gap-3">
                        <span className="font-mono text-xs text-ink dark:text-bone break-all">{r.path}</span>
                        <button
                          onClick={() => {
                            setPath(parentPath(r.path) ?? "/");
                            setTab("files");
                          }}
                          className="shrink-0 text-xs px-2 py-1 border border-line dark:border-edge rounded cursor-pointer"
                        >
                          Open folder
                        </button>
                      </div>
                    ))}
                  </div>
                )
              ) : (
                <p className="text-sm text-muted dark:text-fog py-6 text-center">Type at least 2 characters to search the whole library.</p>
              )}
            </div>
          )}

          {tab === "trash" && (
            <div className="mt-4 rounded-card border border-line dark:border-edge p-5 shadow-sm">
              <div className="flex items-center justify-between mb-2">
                <h2 className="text-sm font-semibold text-ink dark:text-bone">
                  Trash ({trashQ.data?.items.length ?? 0})
                </h2>
                <button
                  disabled={!canMutate || (trashQ.data?.items.length ?? 0) === 0}
                  onClick={() => void run(() => transfersApi.trashPurge(), "Trash purged")}
                  className="text-xs px-3 py-1.5 border border-brick/30 text-brick rounded-lg cursor-pointer disabled:opacity-50"
                >
                  Empty trash
                </button>
              </div>
              {trashQ.isLoading ? (
                <p className="text-sm text-muted dark:text-fog py-4 text-center">Loading trash…</p>
              ) : (trashQ.data?.items.length ?? 0) === 0 ? (
                <p className="text-sm text-muted dark:text-fog py-4 text-center">Trash is empty.</p>
              ) : (
                trashQ.data!.items.map((t) => (
                  <div key={t.id} className="flex items-center justify-between py-2.5 border-b border-line dark:border-edge last:border-0 gap-3">
                    <div className="min-w-0">
                      <p className="text-sm font-mono text-ink dark:text-bone break-all">{t.original_path}</p>
                      <p className="text-xs text-muted dark:text-fog font-mono">
                        {t.name} · {formatBytes(t.size)} · {new Date(t.deleted_at).toLocaleString()}
                      </p>
                    </div>
                    <button
                      disabled={!canMutate}
                      onClick={() => void run(() => transfersApi.trashRestore(t.id), `Restored ${t.name}`)}
                      className="shrink-0 text-xs px-3 py-1.5 border border-line dark:border-edge rounded-lg cursor-pointer inline-flex items-center gap-1 disabled:opacity-50"
                    >
                      <Undo2 size={13} /> Restore
                    </button>
                  </div>
                ))
              )}
            </div>
          )}

          {tab === "activity" && (
            <div className="mt-4 rounded-card border border-line dark:border-edge p-5 shadow-sm">
              <div className="flex items-center gap-2 mb-2">
                <ArrowDownUp size={15} className="text-muted dark:text-fog" />
                <h2 className="text-sm font-semibold text-ink dark:text-bone">Recent transfers</h2>
              </div>
              {activityQ.isLoading ? (
                <p className="text-sm text-muted dark:text-fog py-4 text-center">Loading activity…</p>
              ) : (activityQ.data?.length ?? 0) === 0 ? (
                <p className="text-sm text-muted dark:text-fog py-4 text-center">
                  No transfer activity yet. Uploads, downloads, moves and deletes appear here.
                </p>
              ) : (
                activityQ.data!.map((a, i) => (
                  <div key={`${a.time}-${i}`} className="flex items-start justify-between py-2.5 border-b border-line dark:border-edge last:border-0 gap-3">
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium text-ink dark:text-bone">
                        <span className="font-mono text-xs bg-paper dark:bg-emboss px-1.5 py-0.5 rounded border border-line dark:border-edge mr-2">
                          {a.action}
                        </span>
                        <span className="break-all">{a.path || a.detail || "—"}</span>
                      </p>
                      <p className="text-xs text-muted dark:text-fog mt-1 font-mono">
                        {a.user} · {a.time ? new Date(a.time).toLocaleString() : "—"}
                      </p>
                    </div>
                  </div>
                ))
              )}
            </div>
          )}

          {/* Prompt modal */}
          {prompt && (
            <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" onClick={() => setPrompt(null)}>
              <div
                className="w-full max-w-md bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5 shadow-lg"
                onClick={(e) => e.stopPropagation()}
              >
                <h3 className="font-head font-bold text-ink dark:text-bone capitalize">
                  {prompt.kind === "mkdir" ? "New folder" : `${prompt.kind} ${prompt.item?.name ?? ""}`}
                </h3>
                <label className="block text-[11px] font-semibold uppercase tracking-wider text-muted dark:text-fog mt-4 mb-1">
                  {prompt.kind === "mkdir" ? "Folder name" : prompt.kind === "rename" ? "New name" : "Destination folder"}
                </label>
                {prompt.kind === "move" || prompt.kind === "copy" ? (
                  <input
                    value={prompt.destValue ?? ""}
                    onChange={(e) => setPrompt({ ...prompt, destValue: e.target.value })}
                    placeholder="/docs"
                    className="w-full px-3 py-2 text-sm font-mono bg-white dark:bg-abyss border border-line dark:border-edge rounded-lg outline-none focus:border-accent dark:focus:border-ember text-ink dark:text-bone"
                  />
                ) : (
                  <input
                    autoFocus
                    value={prompt.value}
                    onChange={(e) => setPrompt({ ...prompt, value: e.target.value })}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") void submitPrompt();
                    }}
                    placeholder={prompt.kind === "mkdir" ? "new-folder" : "name"}
                    className="w-full px-3 py-2 text-sm font-mono bg-white dark:bg-abyss border border-line dark:border-edge rounded-lg outline-none focus:border-accent dark:focus:border-ember text-ink dark:text-bone"
                  />
                )}
                <div className="mt-4 flex justify-end gap-2">
                  <button
                    onClick={() => setPrompt(null)}
                    className="px-4 py-2 text-sm border border-line dark:border-edge rounded-lg cursor-pointer"
                  >
                    Cancel
                  </button>
                  <button
                    onClick={() => void submitPrompt()}
                    disabled={busy}
                    className="px-4 py-2 text-sm bg-accent dark:bg-ember text-white dark:text-black font-semibold rounded-lg cursor-pointer disabled:opacity-50"
                  >
                    Confirm
                  </button>
                </div>
              </div>
            </div>
          )}
        </>
      )}
    </main>
  );
}
