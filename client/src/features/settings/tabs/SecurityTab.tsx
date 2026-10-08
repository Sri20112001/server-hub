import React, { useState } from 'react';
import { Eye, EyeOff, Check, TriangleAlert } from 'lucide-react';
import { useAuth, useUi } from '../../../stores/store';
import { api } from '../../../lib/api';
import { Field, ConfirmModal } from '../../../components/ui';
import { GlassCard } from '../components/GlassCard';
import { CyberButton } from '../components/CyberButton';
import { DangerButton } from '../components/DangerButton';

export function SecurityTab() {
  const { user } = useAuth();
  const pushToast = useUi((s) => s.pushToast);
  const [confirmRestart, setConfirmRestart] = useState(false);
  const [confirmWipe, setConfirmWipe] = useState(false);

  return (
    <div className="flex flex-col gap-6">
      {/* Profile & Access */}
      <GlassCard>
        <div className="flex items-start justify-between gap-3 mb-6">
          <div>
            <h2 className="text-xl font-semibold text-ink dark:text-bone">Profile & Access</h2>
            <p className="text-muted dark:text-fog text-sm mt-1">Manage operator session identity and root authentication vectors.</p>
          </div>
          <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-paper dark:bg-emboss border border-line dark:border-edge text-ink dark:text-bone rounded-input px-2 py-1 whitespace-nowrap shadow-sm">NODE-01</span>
        </div>

        <div className="bg-paper dark:bg-emboss/40 border border-line dark:border-edge rounded-xl px-5 py-4 flex items-center justify-between gap-3 flex-wrap">
          <div className="flex items-center gap-4 flex-wrap">
            <span className="w-12 h-12 rounded-full bg-paper dark:bg-emboss border border-line dark:border-edge flex items-center justify-center font-head font-bold text-xl text-accent dark:text-ember shadow-inner">
              {(user?.username ?? "C").slice(0, 1).toUpperCase()}
            </span>
            <div>
              <div className="flex items-center gap-2 flex-wrap">
                <strong className="text-ink dark:text-bone text-base">{user?.username ?? "—"}</strong>
                <span className="inline-flex items-center gap-1.5 font-mono text-[11px] bg-accent/10 dark:bg-ember/10 border border-accent/30 dark:border-ember/30 text-accent dark:text-ember rounded-input px-2 py-0.5 whitespace-nowrap">{user?.role ?? "admin"}</span>
              </div>
              <div className="font-mono text-muted dark:text-fog text-xs mt-0.5">single-operator station · cookie session</div>
            </div>
          </div>
          <span className="inline-flex items-center gap-1.5 h-7 px-3 rounded-full font-mono text-xs bg-white dark:bg-panel border border-line dark:border-edge text-ink dark:text-bone whitespace-nowrap shadow-sm">
            <span className="w-2 h-2 rounded-full bg-moss" /> Auth: Local
          </span>
        </div>

        <div className="mt-8">
          <h3 className="text-sm font-semibold text-ink dark:text-bone mb-4">Rotate Master Password</h3>
          <PasswordForm pushToast={pushToast} />
        </div>
      </GlassCard>

      {/* Danger Zone */}
      <GlassCard className="!border-brick/30 !bg-red-50/50 dark:!bg-red-950/20">
        <div className="flex items-center gap-2.5 flex-wrap text-brick mb-2">
          <TriangleAlert size={20} />
          <h2 className="text-xl font-bold text-ink dark:text-bone">Danger Zone</h2>
        </div>
        <p className="text-muted dark:text-red-200/70 text-sm mb-6">
          Irreversible operations. Proceed with operational clearance.
        </p>
        <div className="flex items-center gap-4 flex-wrap">
          <DangerButton onClick={() => setConfirmRestart(true)}>
            Fresh start (Restart ServerHub)
          </DangerButton>
          <DangerButton onClick={() => setConfirmWipe(true)}>
            Wipe deployment history
          </DangerButton>
        </div>
      </GlassCard>

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
    </div>
  );
}

function PasswordForm({ pushToast }: { pushToast: (m: string, err?: boolean) => void }) {
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [again, setAgain] = useState("");
  const [pwBusy, setPwBusy] = useState(false);
  const [showPw, setShowPw] = useState(false);

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

  const inputClasses = "w-full bg-paper dark:bg-emboss border border-line dark:border-edge rounded-input font-body text-sm text-ink dark:text-bone px-4 py-2.5 outline-none focus:border-accent dark:focus:border-ember placeholder:text-muted dark:placeholder:text-fog font-mono transition-colors";

  return (
    <form onSubmit={(e) => void changePassword(e)} className="flex flex-col gap-5">
      <div className="grid grid-cols-1 gap-5 sm:grid-cols-2">
        <Field label="Current master password">
          <div className="relative">
            <input
              className={`${inputClasses} pr-10`}
              type={showPw ? "text" : "password"}
              value={current}
              onChange={(e) => setCurrent(e.target.value)}
              autoComplete="current-password"
            />
            <button
              type="button"
              className="absolute right-3 top-1/2 -translate-y-1/2 text-muted dark:text-fog hover:text-ink dark:hover:text-bone bg-transparent border-0 cursor-pointer flex p-1 transition-colors"
              onClick={() => setShowPw((v) => !v)}
              aria-label="Toggle visibility"
            >
              {showPw ? <EyeOff size={16} /> : <Eye size={16} />}
            </button>
          </div>
        </Field>
        <Field label="New master password (8+ chars)">
          <input
            className={inputClasses}
            type={showPw ? "text" : "password"}
            value={next}
            onChange={(e) => setNext(e.target.value)}
            autoComplete="new-password"
          />
        </Field>
      </div>
      <Field label="Confirm new password">
        <input
          className={inputClasses}
          type={showPw ? "text" : "password"}
          value={again}
          onChange={(e) => setAgain(e.target.value)}
          autoComplete="new-password"
        />
      </Field>
      <div className="flex items-center justify-end gap-2 mt-2">
        <CyberButton
          type="submit"
          disabled={pwBusy || !current || next.length < 8 || next !== again}
        >
          <Check size={16} /> {pwBusy ? "Rotating…" : "Save changes"}
        </CyberButton>
      </div>
    </form>
  );
}
