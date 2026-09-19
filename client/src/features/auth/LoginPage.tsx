import { useState } from "react";
import { useNavigate } from "react-router";
import { ShipWheel } from "lucide-react";
import { useAuth, useUi } from "../../stores/store";
import { Field } from "../../components/ui";

export function LoginPage() {
  const nav = useNavigate();
  const { login, busy, error } = useAuth();
  const pushToast = useUi((s) => s.pushToast);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await login(username.trim(), password);
      pushToast("Welcome aboard, Captain.");
      nav("/", { replace: true });
    } catch {
      /* error shown inline via store */
    }
  };

  return (
    <div className="min-h-[100svh] flex items-center justify-center p-4">
      <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-5 w-[400px] max-w-full">
        <div className="flex items-center gap-2 flex-wrap mb-1">
          <span className="w-[38px] h-[38px] rounded-full bg-accent-deep dark:bg-ember text-white dark:text-black flex items-center justify-center">
            <ShipWheel size={20} />
          </span>
          <div>
            <div className="font-head font-bold text-[20px]">ServerHub</div>
            <div className="text-[11px] font-semibold tracking-[0.08em] uppercase text-muted dark:text-fog">
              Bridge console · sign in
            </div>
          </div>
        </div>
        <p className="text-muted dark:text-fog text-[13px] my-3 mb-4">
          One server. One captain. Identify yourself to take the helm.
        </p>
        <form onSubmit={submit} className="flex flex-col gap-4">
          <Field label="Username">
            <input
              className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-input font-body text-[13px] text-ink dark:text-bone px-3 py-2.5 outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              autoComplete="username"
              autoFocus
            />
          </Field>
          <Field label="Password">
            <input
              className="w-full bg-white dark:bg-panel border border-line dark:border-edge rounded-input font-body text-[13px] text-ink dark:text-bone px-3 py-2.5 outline-none focus:border-accent dark:focus:border-ember placeholder:text-stone dark:placeholder:text-fog"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
            />
          </Field>
          {error && (
            <div className="bg-red-50 dark:bg-red-950 border border-red-200 dark:border-red-900 text-red-800 dark:text-red-200 rounded-input px-3 py-2.5 text-[13px]">
              {error}
            </div>
          )}
          <button
            className="inline-flex items-center gap-2 rounded-input text-[13px] font-medium px-4 py-2 cursor-pointer border border-transparent whitespace-nowrap transition-colors duration-150 disabled:opacity-55 disabled:cursor-not-allowed bg-accent dark:bg-ember text-white dark:text-black hover:bg-accent-hover dark:hover:bg-ember-hover"
            type="submit"
            disabled={busy || !username || !password}
          >
            {busy ? "Checking credentials…" : "Take the helm"}
          </button>
        </form>
      </div>
    </div>
  );
}
