import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { KeyRound, Plus, RefreshCw } from "lucide-react";
import { dbBackupsApi } from "../../lib/api";
import type { VgAgent } from "../../lib/types";
import { ConfirmModal, Modal } from "../../components/ui";
import { useUi } from "../../stores/store";
import { InlineError } from "./shared";
import { primaryBtn, statusDot, timeStr, useDbBackupsRole } from "./helpers";

// Shown once at creation: the plaintext secret is never stored
// server-side and cannot be retrieved again.
function OneTimeSecret({ title, secret, warning, onClose }: {
  title: string;
  secret: string;
  warning: string;
  onClose: () => void;
}) {
  return (
    <Modal onClose={onClose}>
      <h3 className="font-head font-bold text-[18px] text-ink dark:text-bone mb-1">{title}</h3>
      <p className="text-brick text-[13px] mb-3">{warning}</p>
      <p className="font-mono text-xs bg-paper dark:bg-abyss border border-line dark:border-edge rounded-lg px-3 py-2.5 break-all select-all">
        {secret}
      </p>
      <div className="flex justify-end mt-4">
        <button onClick={onClose} className={primaryBtn}>Done</button>
      </div>
    </Modal>
  );
}

function AgentCard({ agent, onChanged }: { agent: VgAgent; onChanged: () => void }) {
  const { pushToast } = useUi();
  const { isAdmin } = useDbBackupsRole();
  const [busy, setBusy] = useState(false);
  const [confirmRevoke, setConfirmRevoke] = useState(false);
  const [rotated, setRotated] = useState<string | null>(null);

  const act = async (label: string, fn: () => Promise<unknown>) => {
    setBusy(true);
    try {
      await fn();
      pushToast(label);
      onChanged();
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Failed", true);
    } finally {
      setBusy(false);
    }
  };

  const revoked = !!agent.revoked_at;
  // Presence uses the server-derived lifecycle (heartbeat recency), not
  // the stored status flag: nothing resets the flag when an agent goes
  // quiet, so the flag alone would show dead agents as online forever.
  const presence = agent.lifecycle || agent.status;
  return (
    <div className="bg-surface-container-lowest border border-outline-variant/20 rounded-2xl p-4 shadow-sm">
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-center gap-2.5 min-w-0">
          <span className={`w-2.5 h-2.5 rounded-full shrink-0 ${revoked ? "bg-outline" : statusDot(presence)}`} />
          <div className="min-w-0">
            <p className="font-semibold text-sm text-on-surface truncate">{agent.name}</p>
            <p className="text-xs text-on-surface-variant font-mono mt-0.5 truncate">
              {revoked ? "revoked" : presence.toLowerCase()}
              {agent.version ? ` · v${agent.version}` : ""}
              {agent.last_seen_at ? ` · seen ${timeStr(agent.last_seen_at)}` : " · never seen"}
            </p>
          </div>
        </div>
        {isAdmin && !revoked && (
          <span className="flex items-center gap-1.5 shrink-0">
            <button
              title="Rotate token (invalidates the current one)"
              disabled={busy}
              onClick={() => void (async () => {
                setBusy(true);
                try {
                  const res = await dbBackupsApi.rotateAgentToken(agent.id);
                  setRotated(res.agent_token);
                  pushToast("New token issued; old token invalidated");
                  onChanged();
                } catch (e) {
                  pushToast(e instanceof Error ? e.message : "Failed", true);
                } finally {
                  setBusy(false);
                }
              })()}
              className="text-on-surface-variant hover:text-on-surface p-1.5 rounded-full cursor-pointer disabled:opacity-50 transition-colors"
            >
              <KeyRound size={14} />
            </button>
            <button
              title="Revoke agent"
              onClick={() => setConfirmRevoke(true)}
              className="text-on-surface-variant hover:text-error p-1.5 rounded-full cursor-pointer text-xs font-semibold px-2.5 transition-colors"
            >
              Revoke
            </button>
          </span>
        )}
      </div>

      {rotated && (
        <OneTimeSecret
          title="New agent token"
          secret={rotated}
          warning="Copy it now — the old token no longer works and this one will never be shown again."
          onClose={() => setRotated(null)}
        />
      )}
      {confirmRevoke && (
        <ConfirmModal
          title={`Revoke agent "${agent.name}"?`}
          body="The agent will be rejected everywhere: heartbeats, job claims, and status reports. Past backups are kept. This cannot be undone."
          confirmLabel="Revoke agent"
          busy={busy}
          onClose={() => setConfirmRevoke(false)}
          onConfirm={() => void act("Agent revoked", async () => {
            await dbBackupsApi.revokeAgent(agent.id);
            setConfirmRevoke(false);
          })}
        />
      )}
    </div>
  );
}

export function AgentsTab() {
  const qc = useQueryClient();
  const { pushToast } = useUi();
  const { isAdmin } = useDbBackupsRole();
  const [busy, setBusy] = useState(false);
  const [regKey, setRegKey] = useState<string | null>(null);
  const [enrollToken, setEnrollToken] = useState<string | null>(null);
  const [enrollName, setEnrollName] = useState("");

  const agentsQ = useQuery({ queryKey: ["vg-agents"], queryFn: dbBackupsApi.agents, refetchInterval: 30_000 });
  const machinesQ = useQuery({ queryKey: ["vg-machines"], queryFn: dbBackupsApi.machines, refetchInterval: 30_000 });
  const tokensQ = useQuery({ queryKey: ["vg-enroll-tokens"], queryFn: dbBackupsApi.enrollTokens, refetchInterval: 30_000 });

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["vg-agents"] });
    void qc.invalidateQueries({ queryKey: ["vg-machines"] });
    void qc.invalidateQueries({ queryKey: ["vg-enroll-tokens"] });
    void agentsQ.refetch();
  };

  const mint = async (fn: () => Promise<{ enrollment_token?: string; registration_key?: string }>, label: string, set: (s: string) => void) => {
    setBusy(true);
    try {
      const res = await fn();
      const secret = res.enrollment_token ?? res.registration_key ?? "";
      set(secret);
      pushToast(label);
      refresh();
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Failed", true);
    } finally {
      setBusy(false);
    }
  };

  const revokeToken = async (id: string) => {
    setBusy(true);
    try {
      await dbBackupsApi.revokeEnrollToken(id);
      pushToast("Token revoked");
      refresh();
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Failed", true);
    } finally {
      setBusy(false);
    }
  };

  const loading = agentsQ.isLoading || machinesQ.isLoading || tokensQ.isLoading;
  const failed = agentsQ.isError || machinesQ.isError || tokensQ.isError;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between flex-wrap gap-2.5">
        <p className="text-on-surface-variant text-xs">
          Agents run on your machines and perform the backups. Tokens are shown once, at creation.
        </p>
        <span className="flex items-center gap-2 flex-wrap">
          <button onClick={refresh} aria-label="Refresh"
            className="w-10 h-10 rounded-full bg-surface-container-lowest border border-outline-variant/30 flex items-center justify-center text-on-surface-variant hover:text-on-surface cursor-pointer shadow-sm transition-colors">
            <RefreshCw size={15} />
          </button>
          {isAdmin && (
            <>
              <button
                disabled={busy}
                onClick={() => void mint(() => dbBackupsApi.mintRegistrationToken(), "Registration key created", setRegKey)}
                className="px-4 py-2 text-xs border border-outline-variant/30 rounded-full cursor-pointer disabled:opacity-50 bg-surface-container-low text-on-surface hover:bg-surface-container transition-colors shadow-sm">
                New registration key
              </button>
              <span className="inline-flex items-center gap-2">
                <input
                  value={enrollName}
                  onChange={(e) => setEnrollName(e.target.value)}
                  placeholder="Agent name (optional)"
                  className="bg-surface-container-lowest border border-outline-variant/30 rounded-xl px-3.5 py-2 text-xs text-on-surface outline-none focus:border-primary placeholder:text-on-surface-variant/40 w-44 shadow-sm transition-colors"
                />
                <button
                  disabled={busy}
                  onClick={() => void mint(
                    () => dbBackupsApi.mintEnrollToken(enrollName.trim() ? { name: enrollName.trim() } : {}),
                    "Enrollment token created",
                    setEnrollToken,
                  )}
                  className="flex items-center gap-1.5 px-4 py-2 bg-primary text-on-primary text-xs font-semibold rounded-full cursor-pointer disabled:opacity-50 hover:bg-primary/90 transition-all shadow-sm">
                  <Plus size={13} /> Enroll token
                </button>
              </span>
            </>
          )}
        </span>
      </div>

      {loading && (
        <div className="space-y-3">
          {[1, 2].map((i) => <div key={i} className="h-20 rounded-2xl bg-surface-container-low border border-outline-variant/20 animate-pulse" />)}
        </div>
      )}
      {!loading && failed && (
        <InlineError message="Could not load agents." onRetry={refresh} />
      )}
      {!loading && !failed && (agentsQ.data ?? []).length === 0 && (
        <p className="text-sm text-on-surface-variant py-4 text-center">
          No agents yet. {isAdmin ? "Create an enrollment token above, then install the agent on a machine." : "Ask an admin to enroll an agent."}
        </p>
      )}
      {(agentsQ.data ?? []).map((a) => <AgentCard key={a.id} agent={a} onChanged={refresh} />)}

      {(machinesQ.data ?? []).length > 0 && (
        <div className="rounded-2xl border border-outline-variant/20 p-5 bg-surface-container-lowest shadow-sm">
          <h3 className="text-sm font-semibold text-on-surface mb-2">Machines</h3>
          {(machinesQ.data ?? []).map((m) => (
            <p key={m.id} className="text-xs font-mono text-on-surface-variant py-1">
              {m.hostname || m.id.slice(0, 8)}{m.os ? ` · ${m.os}` : ""}{m.ip_address ? ` · ${m.ip_address}` : ""}
            </p>
          ))}
        </div>
      )}

      {(tokensQ.data ?? []).length > 0 && (
        <div className="rounded-2xl border border-outline-variant/20 p-5 bg-surface-container-lowest shadow-sm">
          <h3 className="text-sm font-semibold text-ink dark:text-bone mb-2">Enrollment tokens</h3>
          {(tokensQ.data ?? []).map((t) => (
            <div key={t.id} className="flex items-center justify-between py-1.5 border-b border-line dark:border-edge last:border-0 gap-2">
              <span className="font-mono text-xs text-muted dark:text-fog">
                {t.id.slice(0, 8)} Â· expires {timeStr(t.expires_at)}
                {t.used_at ? " Â· used" : ""}
                {t.revoked_at ? " Â· revoked" : ""}
              </span>
              {isAdmin && !t.used_at && !t.revoked_at && (
                <button disabled={busy} onClick={() => void revokeToken(t.id)}
                  className="text-xs px-2 py-1 border border-line dark:border-edge rounded-lg cursor-pointer disabled:opacity-50">
                  Revoke
                </button>
              )}
            </div>
          ))}
        </div>
      )}

      {regKey && (
        <OneTimeSecret
          title="Registration key"
          secret={regKey}
          warning="Copy it now â€” only the fingerprint is stored and it will never be shown again."
          onClose={() => setRegKey(null)}
        />
      )}
      {enrollToken && (
        <OneTimeSecret
          title="Enrollment token"
          secret={enrollToken}
          warning="Single-use and short-lived. Copy it now â€” it will never be shown again."
          onClose={() => setEnrollToken(null)}
        />
      )}
    </div>
  );
}
