import { useEffect, useState } from "react";
import { BellRing, Check, Pencil, Plus, Trash2 } from "lucide-react";
import { api } from "../../lib/api";
import type {
  NotificationChannel,
  NotificationEventType,
  NotificationGroup,
  NotificationRule,
  NotificationRuleCondition,
} from "../../lib/types";
import { useAuth, useUi } from "../../stores/store";
import { Field, Toggle } from "../../components/ui";
import { GlassCard } from "./components/GlassCard";
import { CyberButton } from "./components/CyberButton";

const EVENT_TYPES: NotificationEventType[] = [
  "AGENT_OFFLINE",
  "AGENT_ONLINE",
  "SERVER_ALERT",
  "SERVER_ALERT_RESOLVED",
];

const EVENT_LABELS: Record<NotificationEventType, string> = {
  AGENT_OFFLINE: "Agent Offline",
  AGENT_ONLINE: "Agent Online (recovery)",
  SERVER_ALERT: "Server Alert",
  SERVER_ALERT_RESOLVED: "Alert Resolved (recovery)",
};

const CONDITION_FIELDS = ["severity", "condition", "value", "serverId"] as const;
type ConditionField = (typeof CONDITION_FIELDS)[number];

const OPERATORS_BY_FIELD: Record<ConditionField, string[]> = {
  severity: ["eq", "neq", "contains", "in"],
  condition: ["eq", "neq", "contains", "in"],
  value: ["eq", "neq", "gt", "gte", "lt", "lte", "in"],
  serverId: ["eq", "neq", "gt", "gte", "lt", "lte", "in"],
};

const NUMERIC_FIELDS: ConditionField[] = ["value", "serverId"];

interface FormState {
  id: number | null;
  name: string;
  description: string;
  eventType: NotificationEventType;
  severity: string;
  useCondition: boolean;
  condField: ConditionField;
  condOp: string;
  condValue: string;
  groupId: string;
  email: boolean;
  inApp: boolean;
  cooldown: string;
  cooldownUnit: "seconds" | "minutes" | "hours";
  notifyOnRecovery: boolean;
  enabled: boolean;
}

const EMPTY_FORM: FormState = {
  id: null,
  name: "",
  description: "",
  eventType: "AGENT_OFFLINE",
  severity: "",
  useCondition: false,
  condField: "severity",
  condOp: "eq",
  condValue: "",
  groupId: "",
  email: true,
  inApp: false,
  cooldown: "15",
  cooldownUnit: "minutes",
  notifyOnRecovery: true,
  enabled: true,
};

function toSeconds(amount: string, unit: FormState["cooldownUnit"]): number {
  const n = Math.max(0, Math.floor(Number(amount) || 0));
  if (unit === "hours") return n * 3600;
  if (unit === "minutes") return n * 60;
  return n;
}

function buildConditionJson(f: FormState): string | undefined {
  if (!f.useCondition || !f.condValue.trim()) return undefined;
  const numeric = NUMERIC_FIELDS.includes(f.condField);
  let value: unknown = f.condValue.trim();
  if (f.condOp === "in") {
    const parts = f.condValue.split(",").map((p) => p.trim()).filter(Boolean);
    value = numeric ? parts.map(Number) : parts;
  } else if (numeric) {
    value = Number(f.condValue.trim());
  }
  const cond: NotificationRuleCondition = {
    field: f.condField,
    operator: f.condOp as NotificationRuleCondition["operator"],
    value,
  };
  return JSON.stringify(cond);
}

const inputClasses = "w-full bg-white dark:bg-black/50 border border-gray-300 dark:border-white/10 rounded-lg font-body text-sm text-gray-900 dark:text-white px-4 py-2.5 outline-none focus:border-cyan-500 dark:focus:border-cyan-400 focus:ring-1 focus:ring-cyan-500/50 placeholder:text-gray-400 dark:placeholder:text-gray-600 transition-all";

export function NotificationRulesSection() {
  const { user } = useAuth();
  const pushToast = useUi((s) => s.pushToast);
  const [rules, setRules] = useState<NotificationRule[]>([]);
  const [groups, setGroups] = useState<NotificationGroup[]>([]);
  const [form, setForm] = useState<FormState>(EMPTY_FORM);
  const [editing, setEditing] = useState(false);
  const [busy, setBusy] = useState(false);

  const isAdmin = user?.role === "admin";
  const canEdit = user?.role === "operator" || isAdmin;

  const refresh = async () => {
    try {
      const [r, g] = await Promise.all([api.notificationRules(), api.notificationGroups()]);
      setRules(r);
      setGroups(g);
    } catch {
      /* engine unavailable — legacy notifications still work */
    }
  };

  useEffect(() => {
    // Intentional: initial load from the API on mount.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void refresh();
  }, []);

  const set = <K extends keyof FormState>(k: K, v: FormState[K]) =>
    setForm((f) => {
      const next = { ...f, [k]: v };
      if (k === "condField") {
        const ops = OPERATORS_BY_FIELD[v as ConditionField];
        if (!ops.includes(next.condOp)) next.condOp = ops[0];
      }
      return next;
    });

  const startCreate = () => {
    setForm({ ...EMPTY_FORM, groupId: groups[0] ? String(groups[0].id) : "" });
    setEditing(true);
  };

  const startEdit = (r: NotificationRule) => {
    let useCondition = false;
    let condField: ConditionField = "severity";
    let condOp = "eq";
    let condValue = "";
    if (r.conditionJson) {
      try {
        const c = JSON.parse(r.conditionJson) as { field: string; operator: string; value: unknown };
        if (CONDITION_FIELDS.includes(c.field as ConditionField)) {
          useCondition = true;
          condField = c.field as ConditionField;
          condOp = c.operator;
          condValue = Array.isArray(c.value) ? c.value.join(", ") : String(c.value ?? "");
        }
      } catch {
        /* leave condition off on unparseable rows */
      }
    }
    const channels = r.channels.split(",").map((s) => s.trim());
    const cd = r.cooldownSeconds;
    setForm({
      id: r.id,
      name: r.name,
      description: "",
      eventType: r.eventType,
      severity: r.severity,
      useCondition,
      condField,
      condOp,
      condValue,
      groupId: String(r.notificationGroupId),
      email: channels.includes("EMAIL"),
      inApp: channels.includes("IN_APP"),
      cooldown: String(cd >= 3600 && cd % 3600 === 0 ? cd / 3600 : cd >= 60 && cd % 60 === 0 ? cd / 60 : cd),
      cooldownUnit: cd >= 3600 && cd % 3600 === 0 ? "hours" : cd >= 60 && cd % 60 === 0 ? "minutes" : "seconds",
      notifyOnRecovery: r.notifyOnRecovery,
      enabled: r.enabled,
    });
    setEditing(true);
  };

  const submit = async () => {
    if (!form.name.trim() || !form.groupId) {
      pushToast("Name and recipient group are required", true);
      return;
    }
    const channels: NotificationChannel[] = [
      ...(form.email ? ["EMAIL" as const] : []),
      ...(form.inApp ? ["IN_APP" as const] : []),
    ];
    if (channels.length === 0) {
      pushToast("Select at least one channel", true);
      return;
    }
    setBusy(true);
    try {
      const payload = {
        name: form.name.trim(),
        description: form.description.trim(),
        enabled: form.enabled,
        eventType: form.eventType,
        severity: form.severity || undefined,
        conditionJson: buildConditionJson(form),
        notificationGroupId: Number(form.groupId),
        channels,
        cooldownSeconds: toSeconds(form.cooldown, form.cooldownUnit),
        notifyOnRecovery: form.notifyOnRecovery,
      };
      if (form.id == null) {
        await api.createNotificationRule(payload);
        pushToast("Rule created.");
      } else {
        await api.updateNotificationRule(form.id, payload);
        pushToast("Rule updated.");
      }
      setEditing(false);
      setForm(EMPTY_FORM);
      await refresh();
    } catch (err) {
      pushToast(err instanceof Error ? err.message : "Save failed", true);
    } finally {
      setBusy(false);
    }
  };

  const toggleEnabled = async (r: NotificationRule) => {
    try {
      await api.updateNotificationRule(r.id, { enabled: !r.enabled });
      await refresh();
    } catch (err) {
      pushToast(err instanceof Error ? err.message : "Update failed", true);
    }
  };

  const remove = async (r: NotificationRule) => {
    if (!confirm(`Delete rule "${r.name}"?`)) return;
    try {
      await api.deleteNotificationRule(r.id);
      pushToast("Rule deleted.");
      await refresh();
    } catch (err) {
      pushToast(err instanceof Error ? err.message : "Delete failed", true);
    }
  };

  return (
    <GlassCard>
      <div className="flex items-start justify-between gap-3 mb-6">
        <div>
          <h2 className="text-xl font-semibold text-gray-900 dark:text-white">Notification Rules</h2>
          <p className="text-gray-500 dark:text-gray-400 text-sm mt-1">
            Route matching events to recipient groups.{" "}
            {rules.length === 0
              ? "No rules configured — existing notifications keep using the default behavior."
              : `${rules.length} rule${rules.length === 1 ? "" : "s"} configured.`}
          </p>
        </div>
        <span className="text-gray-500 dark:text-gray-400 flex items-center justify-center w-10 h-10 rounded-full bg-gray-100 dark:bg-white/5 border border-gray-200 dark:border-white/10">
          <BellRing size={18} />
        </span>
      </div>

      {rules.length > 0 && (
        <div className="flex flex-col gap-2 mb-6">
          {rules.map((r) => (
            <div
              key={r.id}
              className="flex items-center justify-between gap-3 py-3 px-3 bg-white dark:bg-black/30 border border-gray-200 dark:border-white/10 rounded-xl"
            >
              <div className="min-w-0">
                <div className="flex items-center gap-2 flex-wrap">
                  <span className="font-semibold text-sm text-gray-900 dark:text-white">{r.name}</span>
                  {!r.enabled && (
                    <span className="font-mono text-[11px] text-gray-500 dark:text-gray-400">disabled</span>
                  )}
                </div>
                <div className="text-gray-500 dark:text-gray-400 text-xs font-mono mt-0.5">
                  {r.eventType}
                  {r.severity ? ` · ${r.severity}` : ""} → {r.groupName || `#${r.notificationGroupId}`} ·{" "}
                  {r.channels} · cooldown {r.cooldownSeconds}s{r.notifyOnRecovery ? " · recovery" : ""}
                </div>
              </div>
              <div className="flex items-center gap-2 shrink-0">
                {canEdit && (
                  <Toggle checked={r.enabled} onChange={() => void toggleEnabled(r)} label={`Enable ${r.name}`} />
                )}
                {canEdit && (
                  <button
                    className="text-gray-400 hover:text-cyan-500 dark:text-gray-500 dark:hover:text-cyan-400 bg-transparent border-0 cursor-pointer p-1.5"
                    onClick={() => startEdit(r)}
                    aria-label={`Edit ${r.name}`}
                  >
                    <Pencil size={15} />
                  </button>
                )}
                {isAdmin && (
                  <button
                    className="text-gray-400 hover:text-red-500 dark:text-gray-500 dark:hover:text-red-400 bg-transparent border-0 cursor-pointer p-1.5"
                    onClick={() => void remove(r)}
                    aria-label={`Delete ${r.name}`}
                  >
                    <Trash2 size={15} />
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {canEdit && !editing && (
        <CyberButton onClick={startCreate}>
          <Plus size={16} /> New rule
        </CyberButton>
      )}

      {canEdit && editing && (
        <div className="bg-gray-50 dark:bg-black/20 rounded-xl p-4 border border-gray-200 dark:border-white/10 flex flex-col gap-4">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Field label="Rule name *">
              <input className={inputClasses} value={form.name} onChange={(e) => set("name", e.target.value)} placeholder="Critical offline pages" autoComplete="off" />
            </Field>
            <Field label="Description">
              <input className={inputClasses} value={form.description} onChange={(e) => set("description", e.target.value)} placeholder="Who gets woken up" autoComplete="off" />
            </Field>
          </div>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Field label="Event">
              <select className={inputClasses} value={form.eventType} onChange={(e) => set("eventType", e.target.value as NotificationEventType)}>
                {EVENT_TYPES.map((t) => (
                  <option key={t} value={t}>{EVENT_LABELS[t]}</option>
                ))}
              </select>
            </Field>
            <Field label="Severity (blank = any)">
              <select className={inputClasses} value={form.severity} onChange={(e) => set("severity", e.target.value)}>
                <option value="">Any severity</option>
                <option value="INFO">INFO</option>
                <option value="WARNING">WARNING</option>
                <option value="CRITICAL">CRITICAL</option>
              </select>
            </Field>
          </div>
          <div className="flex items-center justify-between gap-4">
            <span className="text-sm text-gray-500 dark:text-gray-400">Extra condition (optional)</span>
            <Toggle checked={form.useCondition} onChange={() => set("useCondition", !form.useCondition)} label="Use condition" />
          </div>
          {form.useCondition && (
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
              <Field label="Field">
                <select className={inputClasses} value={form.condField} onChange={(e) => set("condField", e.target.value as ConditionField)}>
                  {CONDITION_FIELDS.map((f) => (
                    <option key={f} value={f}>{f}</option>
                  ))}
                </select>
              </Field>
              <Field label="Operator">
                <select className={inputClasses} value={form.condOp} onChange={(e) => set("condOp", e.target.value)}>
                  {OPERATORS_BY_FIELD[form.condField].map((o) => (
                    <option key={o} value={o}>{o}</option>
                  ))}
                </select>
              </Field>
              <Field label={form.condOp === "in" ? "Values (comma-separated)" : "Value"}>
                <input className={`${inputClasses} font-mono`} value={form.condValue} onChange={(e) => set("condValue", e.target.value)} placeholder={NUMERIC_FIELDS.includes(form.condField) ? "90" : "cpu_high"} autoComplete="off" />
              </Field>
            </div>
          )}
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Field label="Recipient group *">
              <select className={inputClasses} value={form.groupId} onChange={(e) => set("groupId", e.target.value)}>
                <option value="">Select a group…</option>
                {groups.map((g) => (
                  <option key={g.id} value={g.id}>{g.name} ({g.memberCount})</option>
                ))}
              </select>
            </Field>
            <div className="flex items-end gap-5 pb-3">
              <label className="flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
                <input type="checkbox" checked={form.email} onChange={(e) => set("email", e.target.checked)} /> Email
              </label>
              <label className="flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
                <input type="checkbox" checked={form.inApp} onChange={(e) => set("inApp", e.target.checked)} /> In-app
              </label>
            </div>
          </div>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Field label="Cooldown">
              <div className="flex gap-2">
                <input type="number" min={0} className={`${inputClasses} font-mono`} value={form.cooldown} onChange={(e) => set("cooldown", e.target.value)} />
                <select className={inputClasses} value={form.cooldownUnit} onChange={(e) => set("cooldownUnit", e.target.value as FormState["cooldownUnit"])}>
                  <option value="seconds">sec</option>
                  <option value="minutes">min</option>
                  <option value="hours">hr</option>
                </select>
              </div>
            </Field>
            <div className="flex flex-col gap-3 justify-end pb-1">
              <div className="flex items-center justify-between gap-4">
                <span className="text-sm text-gray-500 dark:text-gray-400">Notify on recovery</span>
                <Toggle checked={form.notifyOnRecovery} onChange={() => set("notifyOnRecovery", !form.notifyOnRecovery)} label="Notify on recovery" />
              </div>
              <div className="flex items-center justify-between gap-4">
                <span className="text-sm text-gray-500 dark:text-gray-400">Enabled</span>
                <Toggle checked={form.enabled} onChange={() => set("enabled", !form.enabled)} label="Rule enabled" />
              </div>
            </div>
          </div>
          <div className="flex items-center gap-3 flex-wrap">
            <CyberButton disabled={busy} onClick={() => void submit()}>
              <Check size={16} /> {busy ? "Saving…" : form.id == null ? "Create rule" : "Save rule"}
            </CyberButton>
            <button
              className="inline-flex items-center gap-2 rounded-lg text-sm font-medium px-5 py-2.5 cursor-pointer border transition-colors duration-200 bg-white dark:bg-transparent border-gray-300 dark:border-white/20 text-gray-700 dark:text-white hover:bg-gray-50 dark:hover:bg-white/5"
              onClick={() => { setEditing(false); setForm(EMPTY_FORM); }}
            >
              Cancel
            </button>
          </div>
        </div>
      )}
    </GlassCard>
  );
}
