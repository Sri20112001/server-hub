import { useEffect, useState } from "react";
import { Download, FolderSearch, Radar } from "lucide-react";
import { api } from "../../lib/api";
import type { DiscoveredProject, FoundProject } from "../../lib/types";
import { useUi } from "../../stores/store";
import { EmptyState, Kicker, Modal, StatusPill } from "../../components/ui";

export function DiscoveryModal({
  onClose,
  onImported,
}: {
  onClose: () => void;
  onImported: () => void;
}) {
  const pushToast = useUi((s) => s.pushToast);
  const [projects, setProjects] = useState<DiscoveredProject[] | null>(null);
  const [filesystem, setFilesystem] = useState<FoundProject[] | null>(null);
  const [dockerAvailable, setDockerAvailable] = useState(true);
  const [busy, setBusy] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    api
      .discovery()
      .then((r) => {
        setProjects(r.projects);
        setFilesystem(r.filesystem ?? []);
        setDockerAvailable(r.dockerAvailable);
      })
      .catch((e) => setErr(e instanceof Error ? e.message : "Scan failed"));
  }, []);

  const importOne = async (name: string) => {
    setBusy(name);
    try {
      const p = await api.importDiscovered(name);
      pushToast(`“${p.name}” joined the fleet with ${p.servicesAdded} station${p.servicesAdded === 1 ? "" : "s"}.`);
      onImported();
      onClose();
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Import failed", true);
      setBusy(null);
    }
  };

  return (
    <Modal onClose={onClose}>
      <div className="flex items-center gap-2 flex-wrap">
        <Radar size={16} />
        <h3 className="text-[18px]">Scan shipyard</h3>
      </div>
      <p className="text-muted dark:text-fog text-[13px] mt-1 mb-4">
        Detected from Docker compose labels and server directories. Importing registers the ship and its stations — no typing.
      </p>

      {err && (
        <div className="bg-red-50 dark:bg-red-950 border border-red-200 dark:border-red-900 text-red-800 dark:text-red-200 rounded-input px-3 py-2.5 text-[13px]">
          {err}
        </div>
      )}

      {!err && projects === null && (
        <div className="text-muted dark:text-fog text-[13px]">Sweeping the shipyard…</div>
      )}

      {!err && projects !== null && !dockerAvailable && (
        <EmptyState
          title="Shipyard unreachable"
          hint="No Docker socket here — register ships by hand with New Dispatch."
        />
      )}

      {!err && projects !== null && dockerAvailable && projects.length === 0 && (filesystem === null || filesystem.length === 0) && (
        <EmptyState
          title="Empty waters"
          hint="No compose projects, containers, or project directories detected."
        />
      )}

      {projects !== null && projects.length > 0 && (
        <div className="flex flex-col gap-3 max-h-[50vh] overflow-y-auto pr-1">
          {projects.map((p) => (
            <div
              key={p.name}
              className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-3.5"
            >
              <div className="flex items-center justify-between gap-4">
                <div>
                  <div className="font-head font-bold text-[15px]">{p.name}</div>
                  {p.deploymentPath && (
                    <div className="font-mono text-muted dark:text-fog text-[11px] mt-0.5 truncate max-w-[280px]">
                      {p.deploymentPath}
                    </div>
                  )}
                </div>
                {p.registered ? (
                  <StatusPill status="healthy" label="Registered" />
                ) : (
                  <button
                    className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-accent dark:bg-ember text-white dark:text-black hover:bg-accent-hover dark:hover:bg-ember-hover px-3 py-1.5 text-xs"
                    disabled={busy !== null}
                    onClick={() => void importOne(p.name)}
                  >
                    <Download size={12} /> {busy === p.name ? "Importing…" : "Import"}
                  </button>
                )}
              </div>
              <div className="flex items-center gap-2 flex-wrap mt-2.5">
                {p.services.map((s) => (
                  <span
                    key={`${s.name}-${s.container}`}
                    className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap"
                    title={`${s.container} · ${s.image} · ${s.state}`}
                  >
                    {s.name} · {s.type}
                  </span>
                ))}
              </div>
              <div className="mt-2">
                <Kicker>
                  {p.services.length} station{p.services.length === 1 ? "" : "s"} · {p.composeFile || "docker-compose.yml"}
                </Kicker>
              </div>
            </div>
          ))}
        </div>
      )}

      {filesystem !== null && filesystem.length > 0 && (
        <div className="mt-5">
          <div className="flex items-center gap-2 flex-wrap mb-3">
            <FolderSearch size={14} />
            <Kicker>Server directories</Kicker>
          </div>
          <div className="flex flex-col gap-3 max-h-[40vh] overflow-y-auto pr-1">
            {filesystem.map((p) => (
              <div
                key={p.path}
                className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-xl px-4 py-3.5"
              >
                <div className="flex items-center justify-between gap-4">
                  <div>
                    <div className="font-head font-bold text-[15px]">{p.name}</div>
                    <div className="font-mono text-muted dark:text-fog text-[11px] mt-0.5 truncate max-w-[280px]">
                      {p.path}
                    </div>
                  </div>
                  {p.registered ? (
                    <StatusPill status="healthy" label="Registered" />
                  ) : (
                    <button
                      className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-accent dark:bg-ember text-white dark:text-black hover:bg-accent-hover dark:hover:bg-ember-hover px-3 py-1.5 text-xs"
                      disabled={busy !== null}
                      onClick={() => void importOne(p.name)}
                    >
                      <Download size={12} /> {busy === p.name ? "Importing…" : "Import"}
                    </button>
                  )}
                </div>
                <div className="flex items-center gap-2 flex-wrap mt-2.5">
                  {p.stack.map((s) => (
                    <span
                      key={s}
                      className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-tint dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-md px-2 py-[3px] whitespace-nowrap"
                    >
                      {s}
                    </span>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="flex items-center gap-2 flex-wrap justify-end mt-4">
        <button
          className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-white dark:bg-panel border-line dark:border-edge text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss"
          onClick={onClose}
        >
          Close
        </button>
      </div>
    </Modal>
  );
}
