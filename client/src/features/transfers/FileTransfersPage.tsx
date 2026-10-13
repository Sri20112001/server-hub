import { useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  ArrowDownUp,
  Copy,
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
    <main className="w-full max-w-5xl mx-auto px-6 pt-12 pb-28">
      <div className="flex flex-col sm:flex-row sm:items-end justify-between gap-4 mb-8">
        <div>
          <div className="flex items-center gap-2 mb-1.5">
            <span className="inline-flex items-center justify-center w-6 h-6 rounded-md bg-primary/10 text-primary">
              <Upload size={15} />
            </span>
            <span className="text-[11px] font-semibold text-outline uppercase tracking-wider">Storage & Transfers</span>
          </div>
          <h1 className="text-[32px] sm:text-[36px] font-bold text-on-surface tracking-tight leading-tight">
            File Transfers
          </h1>
          <p className="text-[15px] text-on-surface-variant max-w-xl mt-1">
            Browse, upload, and organize artifacts securely on the host storage pool.
            {!canMutate && " (Read-only mode)"}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={refreshAll}
            className="w-10 h-10 rounded-full bg-surface-container-lowest border border-outline-variant/30 flex items-center justify-center text-on-surface-variant hover:text-on-surface shadow-sm cursor-pointer transition-colors"
            title="Refresh"
            type="button"
          >
            <RefreshCw size={15} />
          </button>
        </div>
      </div>

      {disabled ? (
        <div className="mt-6 rounded-2xl border border-outline-variant/30 bg-surface-container-low p-10 text-center">
          <p className="text-[15px] font-medium text-on-surface">File transfers are disabled.</p>
          <p className="text-[13px] text-on-surface-variant mt-1 font-mono">
            Set FILE_STORAGE_ROOT on the server and restart to enable /transfers.
          </p>
        </div>
      ) : (
        <>
          {/* Storage overview */}
          <div className="mt-6 rounded-2xl bg-surface-container-lowest p-6 shadow-sm">
            <div className="flex items-center gap-2 mb-3">
              <span className="text-[11px] font-semibold uppercase text-on-surface-variant tracking-wider">Storage Pool</span>
            </div>
            {storageQ.isLoading ? (
              <p className="text-[14px] text-on-surface-variant">Loading storage…</p>
            ) : storage ? (
              <>
                <p className="text-[28px] font-bold text-on-surface">
                  {formatBytes(storage.used_bytes)}{" "}
                  <span className="text-[14px] font-normal text-on-surface-variant">
                    of {formatBytes(storage.total_bytes)}
                  </span>
                </p>
                <div className="h-2 mt-3 rounded-full bg-surface-container overflow-hidden">
                  <div
                    className="h-full rounded-full bg-primary"
                    style={{ width: `${usedPct ?? 0}%` }}
                  />
                </div>
                <p className="text-[12px] text-on-surface-variant mt-2 font-mono">
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
                        className="text-[12px] font-mono px-2.5 py-1 rounded-full bg-surface-container-low text-on-surface-variant hover:text-on-surface hover:bg-surface-container transition-colors cursor-pointer border-0"
                        title={`${f.files} files`}
                      >
                        {f.name} · {formatBytes(f.bytes)}
                      </button>
                    ))}
                  </div>
                )}
              </>
            ) : (
              <p className="text-[14px] text-on-surface-variant">
                Storage figures unavailable — {errMsg(storageQ.error, "file service error")}.
              </p>
            )}
          </div>

          {/* Contextual Pill Tabs */}
          <div className="mt-6 flex items-center overflow-x-auto p-1 bg-surface-container rounded-full max-w-fit shadow-inner">
            {(["files", "search", "trash", "activity"] as Tab[]).map((t) => (
              <button
                key={t}
                onClick={() => setTab(t)}
                className={`px-4 py-1.5 text-[13px] font-medium rounded-full cursor-pointer border-0 capitalize transition-all whitespace-nowrap ${
                  tab === t
                    ? "bg-surface-container-lowest text-primary font-semibold shadow-sm"
                    : "text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high"
                }`}
              >
                {t}
              </button>
            ))}
          </div>

          {tab === "files" && (
            <div className="mt-6 rounded-2xl bg-surface-container-lowest border border-outline-variant/20 shadow-sm overflow-hidden">
              {/* Breadcrumb + toolbar */}
              <div className="p-4 border-b border-outline-variant/20 flex items-center justify-between gap-3 flex-wrap">
                <div className="flex items-center gap-1.5 text-sm font-mono min-w-0">
                  <button
                    onClick={() => setPath("/")}
                    className={`px-2 py-1 rounded-full cursor-pointer transition-colors ${path === "/" ? "font-bold text-on-surface bg-surface-container" : "text-on-surface-variant hover:text-on-surface hover:bg-surface-container-low"}`}
                  >
                    /
                  </button>
                  {crumbs.map((c, i) => {
                    const p = `/${crumbs.slice(0, i + 1).join("/")}`;
                    const last = i === crumbs.length - 1;
                    return (
                      <span key={p} className="flex items-center gap-1.5 min-w-0">
                        <span className="text-on-surface-variant/40">/</span>
                        <button
                          onClick={() => setPath(p)}
                          className={`px-2 py-1 rounded-full cursor-pointer truncate transition-colors ${last ? "font-bold text-on-surface bg-surface-container" : "text-on-surface-variant hover:text-on-surface hover:bg-surface-container-low"}`}
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
                      className="px-3.5 py-1.5 text-xs border border-outline-variant/30 rounded-full cursor-pointer inline-flex items-center gap-1 bg-surface-container-low text-on-surface hover:bg-surface-container transition-colors"
                    >
                      <ArrowLeft size={13} /> Up
                    </button>
                  )}
                  <button
                    disabled={!canMutate || busy}
                    onClick={() => setPrompt({ kind: "mkdir", value: "" })}
                    className="px-3.5 py-1.5 text-xs border border-outline-variant/30 rounded-full cursor-pointer inline-flex items-center gap-1 disabled:opacity-50 bg-surface-container-low text-on-surface hover:bg-surface-container transition-colors"
                    title={canMutate ? "New folder" : "Operator role required"}
                  >
                    <FolderPlus size={13} /> New folder
                  </button>
                  <button
                    disabled={!canMutate || busy}
                    onClick={() => fileRef.current?.click()}
                    className="px-4 py-1.5 text-xs bg-primary text-on-primary font-semibold rounded-full cursor-pointer inline-flex items-center gap-1.5 disabled:opacity-50 hover:bg-primary/90 transition-all shadow-sm"
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
                <p className="p-8 text-center text-sm text-on-surface-variant">Loading…</p>
              ) : listQ.isError ? (
                <p className="p-8 text-center text-sm text-error font-medium">
                  {errMsg(listQ.error, "Could not list directory")}
                </p>
              ) : (listQ.data?.items.length ?? 0) === 0 ? (
                <p className="p-8 text-center text-sm text-on-surface-variant">
                  Empty folder. Upload a file or create a folder.
                </p>
              ) : (
                <table className="w-full text-sm border-collapse">
                  <thead>
                    <tr className="bg-surface-container-low/50 border-b border-outline-variant/20">
                      {["Name", "Size", "Modified", "Actions"].map((h) => (
                        <th key={h} className="text-left text-on-surface-variant text-[11px] px-4 py-3 font-semibold uppercase tracking-wider">
                          {h}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {listQ.data!.items.map((it) => (
                      <tr key={it.path} className="border-t border-outline-variant/10 hover:bg-surface-container-low/40 transition-colors">
                        <td className="px-4 py-3 min-w-0">
                          <button
                            onClick={() => it.type === "directory" && setPath(it.path)}
                            disabled={it.type !== "directory"}
                            className={`flex items-center gap-2 min-w-0 ${it.type === "directory" ? "cursor-pointer hover:underline" : "cursor-default"}`}
                            title={it.path}
                          >
                            {it.type === "directory" ? <Folder size={15} className="shrink-0 text-primary" /> : <FileIcon size={15} className="shrink-0 text-on-surface-variant" />}
                            <span className="truncate font-mono text-xs text-on-surface font-medium">{it.name}</span>
                          </button>
                        </td>
                        <td className="px-4 py-3 font-mono text-xs text-on-surface-variant whitespace-nowrap">
                          {it.type === "directory" ? "—" : formatBytes(it.size)}
                        </td>
                        <td className="px-4 py-3 font-mono text-xs text-on-surface-variant whitespace-nowrap">
                          {it.modified_at ? new Date(it.modified_at).toLocaleString() : "—"}
                        </td>
                        <td className="px-4 py-3 whitespace-nowrap">
                          <div className="flex items-center gap-1">
                            {it.type === "file" && (
                              <a
                                href={transfersApi.downloadUrl(it.path)}
                                className="p-1.5 rounded-full hover:bg-surface-container text-on-surface-variant hover:text-on-surface transition-colors"
                                title="Download"
                              >
                                <Download size={14} />
                              </a>
                            )}
                            <button
                              disabled={!canMutate}
                              onClick={() => setPrompt({ kind: "rename", item: it, value: it.name })}
                              className="p-1.5 rounded-full hover:bg-surface-container text-on-surface-variant hover:text-on-surface disabled:opacity-40 cursor-pointer transition-colors"
                              title="Rename"
                            >
                              <Pencil size={14} />
                            </button>
                            <button
                              disabled={!canMutate}
                              onClick={() => setPrompt({ kind: "move", item: it, value: it.name, destValue: parentPath(it.path) ?? "/" })}
                              className="p-1.5 rounded-full hover:bg-surface-container text-on-surface-variant hover:text-on-surface disabled:opacity-40 cursor-pointer transition-colors"
                              title="Move"
                            >
                              <Move size={14} />
                            </button>
                            <button
                              disabled={!canMutate}
                              onClick={() => setPrompt({ kind: "copy", item: it, value: it.name, destValue: parentPath(it.path) ?? "/" })}
                              className="p-1.5 rounded-full hover:bg-surface-container text-on-surface-variant hover:text-on-surface disabled:opacity-40 cursor-pointer transition-colors"
                              title="Copy"
                            >
                              <Copy size={14} />
                            </button>
                            <button
                              disabled={!canMutate}
                              onClick={() => void run(() => transfersApi.trashMove(it.path), `Moved ${it.name} to trash`)}
                              className="p-1.5 rounded-full hover:bg-surface-container text-on-surface-variant hover:text-error disabled:opacity-40 cursor-pointer transition-colors"
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
            <div className="mt-6 rounded-2xl bg-surface-container-lowest border border-outline-variant/20 p-6 shadow-sm">
              <div className="flex items-center gap-2">
                <div className="relative flex-1">
                  <Search size={15} className="absolute left-3.5 top-1/2 -translate-y-1/2 text-on-surface-variant" />
                  <input
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                    placeholder="Search by file name (min 2 chars)…"
                    className="w-full pl-10 pr-4 py-2.5 text-sm bg-surface-container-low border border-outline-variant/30 rounded-xl outline-none focus:border-primary text-on-surface placeholder:text-on-surface-variant/40 transition-colors shadow-sm"
                  />
                </div>
              </div>
              {searchQ.isFetching ? (
                <p className="text-sm text-on-surface-variant py-8 text-center">Searching…</p>
              ) : searchQ.data ? (
                searchQ.data.results.length === 0 ? (
                  <p className="text-sm text-on-surface-variant py-8 text-center">No matches for “{searchQ.data.query}”.</p>
                ) : (
                  <div className="mt-4 divide-y divide-outline-variant/10">
                    {searchQ.data.results.map((r) => (
                      <div key={r.path} className="py-3 flex items-center justify-between gap-3">
                        <span className="font-mono text-xs text-on-surface font-medium break-all">{r.path}</span>
                        <button
                          onClick={() => {
                            setPath(parentPath(r.path) ?? "/");
                            setTab("files");
                          }}
                          className="shrink-0 text-xs px-3.5 py-1.5 border border-outline-variant/30 rounded-full cursor-pointer bg-surface-container-low text-on-surface hover:bg-surface-container transition-colors"
                        >
                          Open folder
                        </button>
                      </div>
                    ))}
                  </div>
                )
              ) : (
                <p className="text-sm text-on-surface-variant py-8 text-center">Type at least 2 characters to search the whole library.</p>
              )}
            </div>
          )}

          {tab === "trash" && (
            <div className="mt-6 rounded-2xl bg-surface-container-lowest border border-outline-variant/20 p-6 shadow-sm">
              <div className="flex items-center justify-between mb-4">
                <h2 className="text-base font-bold text-on-surface">
                  Trash ({trashQ.data?.items.length ?? 0})
                </h2>
                <button
                  disabled={!canMutate || (trashQ.data?.items.length ?? 0) === 0}
                  onClick={() => void run(() => transfersApi.trashPurge(), "Trash purged")}
                  className="text-xs px-4 py-1.5 border border-error/30 text-error rounded-full cursor-pointer disabled:opacity-50 hover:bg-error-container/20 transition-colors"
                >
                  Empty trash
                </button>
              </div>
              {trashQ.isLoading ? (
                <p className="text-sm text-on-surface-variant py-6 text-center">Loading trash…</p>
              ) : (trashQ.data?.items.length ?? 0) === 0 ? (
                <p className="text-sm text-on-surface-variant py-6 text-center">Trash is empty.</p>
              ) : (
                trashQ.data!.items.map((t) => (
                  <div key={t.id} className="flex items-center justify-between py-3 border-b border-outline-variant/10 last:border-0 gap-3">
                    <div className="min-w-0">
                      <p className="text-sm font-mono text-on-surface font-medium break-all">{t.original_path}</p>
                      <p className="text-xs text-on-surface-variant font-mono mt-0.5">
                        {t.name} · {formatBytes(t.size)} · {new Date(t.deleted_at).toLocaleString()}
                      </p>
                    </div>
                    <button
                      disabled={!canMutate}
                      onClick={() => void run(() => transfersApi.trashRestore(t.id), `Restored ${t.name}`)}
                      className="shrink-0 text-xs px-3.5 py-1.5 border border-outline-variant/30 rounded-full cursor-pointer inline-flex items-center gap-1.5 disabled:opacity-50 bg-surface-container-low text-on-surface hover:bg-surface-container transition-colors"
                    >
                      <Undo2 size={13} /> Restore
                    </button>
                  </div>
                ))
              )}
            </div>
          )}

          {tab === "activity" && (
            <div className="mt-6 rounded-2xl bg-surface-container-lowest border border-outline-variant/20 p-6 shadow-sm">
              <div className="flex items-center gap-2 mb-4">
                <ArrowDownUp size={16} className="text-primary" />
                <h2 className="text-base font-bold text-on-surface">Recent transfers</h2>
              </div>
              {activityQ.isLoading ? (
                <p className="text-sm text-on-surface-variant py-6 text-center">Loading activity…</p>
              ) : (activityQ.data?.length ?? 0) === 0 ? (
                <p className="text-sm text-on-surface-variant py-6 text-center">
                  No transfer activity yet. Uploads, downloads, moves and deletes appear here.
                </p>
              ) : (
                activityQ.data!.map((a, i) => (
                  <div key={`${a.time}-${i}`} className="flex items-start justify-between py-3 border-b border-outline-variant/10 last:border-0 gap-3">
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium text-on-surface flex items-center gap-2 flex-wrap">
                        <span className="font-mono text-xs bg-surface-container-low text-on-surface-variant px-2 py-0.5 rounded-full border border-outline-variant/30">
                          {a.action}
                        </span>
                        <span className="break-all">{a.path || a.detail || "—"}</span>
                      </p>
                      <p className="text-xs text-on-surface-variant mt-1 font-mono">
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
            <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-xs p-4" onClick={() => setPrompt(null)}>
              <div
                className="w-full max-w-md bg-surface-container-lowest border border-outline-variant/30 rounded-2xl p-6 shadow-2xl"
                onClick={(e) => e.stopPropagation()}
              >
                <h3 className="font-head font-bold text-[18px] text-on-surface capitalize">
                  {prompt.kind === "mkdir" ? "New folder" : `${prompt.kind} ${prompt.item?.name ?? ""}`}
                </h3>
                <label className="block text-[11px] font-semibold uppercase tracking-wider text-on-surface-variant mt-4 mb-1.5">
                  {prompt.kind === "mkdir" ? "Folder name" : prompt.kind === "rename" ? "New name" : "Destination folder"}
                </label>
                {prompt.kind === "move" || prompt.kind === "copy" ? (
                  <input
                    value={prompt.destValue ?? ""}
                    onChange={(e) => setPrompt({ ...prompt, destValue: e.target.value })}
                    placeholder="/docs"
                    className="w-full px-3.5 py-2.5 text-sm font-mono bg-surface-container-low border border-outline-variant/30 rounded-xl outline-none focus:border-primary text-on-surface transition-colors"
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
                    className="w-full px-3.5 py-2.5 text-sm font-mono bg-surface-container-low border border-outline-variant/30 rounded-xl outline-none focus:border-primary text-on-surface transition-colors"
                  />
                )}
                <div className="mt-6 flex justify-end gap-2.5">
                  <button
                    onClick={() => setPrompt(null)}
                    className="px-5 py-2 text-sm border border-outline-variant/30 rounded-full cursor-pointer bg-surface-container-low text-on-surface hover:bg-surface-container transition-colors"
                  >
                    Cancel
                  </button>
                  <button
                    onClick={() => void submitPrompt()}
                    disabled={busy}
                    className="px-5 py-2 text-sm bg-primary text-on-primary font-semibold rounded-full cursor-pointer disabled:opacity-50 hover:bg-primary/90 transition-all shadow-sm"
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
