import React, { useState } from 'react';
import { Send, Check, Users, Plus, Trash2 } from 'lucide-react';
import { api } from '../../../lib/api';
import type { NotifySettings, NotificationGroup, NotificationGroupDetail } from '../../../lib/types';
import { useUi, useAuth } from '../../../stores/store';
import { Field, Toggle, ConfirmModal } from '../../../components/ui';
import { GlassCard } from '../components/GlassCard';
import { CyberButton } from '../components/CyberButton';

interface NotificationsTabProps {
  notify: NotifySettings | null;
  setNotify: React.Dispatch<React.SetStateAction<NotifySettings | null>>;
  groups: NotificationGroup[];
  refreshGroups: (selectId?: number) => Promise<void>;
}

export function NotificationsTab({ notify, setNotify, groups, refreshGroups }: NotificationsTabProps) {
  const pushToast = useUi((s) => s.pushToast);
  const [ntBusy, setNtBusy] = useState(false);
  const [ntTest, setNtTest] = useState<{ telegram: string; email: string } | null>(null);
  const [tgToken, setTgToken] = useState("");
  const [smtpPass, setSmtpPass] = useState("");

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
          emailGroupId: notify.email.emailGroupId ?? null,
        },
      });
      setTgToken("");
      setSmtpPass("");
      setNtTest(null);
      pushToast("Notification routing saved.");
      const fresh = await api.notifySettings();
      setNotify(fresh);
      await refreshGroups();
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

  if (!notify) {
    return <p className="text-gray-500 dark:text-gray-400 text-sm">Loading notification settings…</p>;
  }

  const inputClasses = "w-full bg-white dark:bg-black/50 border border-gray-300 dark:border-white/10 rounded-lg font-body text-sm text-gray-900 dark:text-white px-4 py-2.5 outline-none focus:border-cyan-500 dark:focus:border-cyan-400 focus:ring-1 focus:ring-cyan-500/50 placeholder:text-gray-400 dark:placeholder:text-gray-600 font-mono transition-all";

  return (
    <div className="flex flex-col gap-6">
      {/* Settings section */}
      <GlassCard>
        <div className="flex items-start justify-between gap-3 mb-6">
          <div>
            <h2 className="text-xl font-semibold text-gray-900 dark:text-white">Notifications</h2>
            <p className="text-gray-500 dark:text-gray-400 text-sm mt-1">Server-side signals to Telegram and email when things break.</p>
          </div>
          <Toggle checked={notify.enabled} onChange={() => patchNotify({ enabled: !notify.enabled })} label="Notifications master switch" />
        </div>

        <div className="flex flex-col gap-0 divide-y divide-gray-200 dark:divide-white/10 mb-8">
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
          />
        </div>

        {/* Telegram Config */}
        <div className="mb-8">
          <div className="flex items-center justify-between gap-4 py-2 mb-4 border-b border-gray-200 dark:border-white/10 pb-4">
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white flex items-center gap-2">
              <span className="text-cyan-500">#</span> Telegram
            </h3>
            <div className="flex items-center gap-3">
              <span className="text-xs text-gray-500 dark:text-gray-400">Enabled</span>
              <Toggle
                checked={notify.telegram.enabled}
                onChange={() => patchNotify({ telegram: { ...notify.telegram, enabled: !notify.telegram.enabled } })}
                label="Telegram channel"
              />
            </div>
          </div>
          <div className="grid grid-cols-1 gap-5 sm:grid-cols-2">
            <Field label="Bot token">
              <input
                className={inputClasses}
                type="password"
                value={tgToken}
                onChange={(e) => setTgToken(e.target.value)}
                placeholder={notify.telegram.hasToken ? "Stored — leave blank to keep" : "123456:ABC-DEF…"}
                autoComplete="new-password"
              />
            </Field>
            <Field label="Chat id">
              <input
                className={inputClasses}
                value={notify.telegram.chatId}
                onChange={(e) => patchNotify({ telegram: { ...notify.telegram, chatId: e.target.value } })}
                placeholder="123456789 (from @userinfobot)"
                autoComplete="off"
              />
            </Field>
          </div>
        </div>

        {/* SMTP Config */}
        <div className="mb-6">
          <div className="flex items-center justify-between gap-4 py-2 mb-4 border-b border-gray-200 dark:border-white/10 pb-4">
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white flex items-center gap-2">
              <span className="text-cyan-500">@</span> Email (SMTP)
            </h3>
            <div className="flex items-center gap-3">
              <span className="text-xs text-gray-500 dark:text-gray-400">Enabled</span>
              <Toggle
                checked={notify.email.enabled}
                onChange={() => patchNotify({ email: { ...notify.email, enabled: !notify.email.enabled } })}
                label="Email channel"
              />
            </div>
          </div>
          <div className="grid grid-cols-1 gap-5 sm:grid-cols-2">
            <Field label="SMTP host">
              <input
                className={inputClasses}
                value={notify.email.host}
                onChange={(e) => patchNotify({ email: { ...notify.email, host: e.target.value } })}
                placeholder="smtp.gmail.com"
                autoComplete="off"
              />
            </Field>
            <Field label="Port">
              <input
                className={inputClasses}
                value={notify.email.port}
                onChange={(e) => patchNotify({ email: { ...notify.email, port: e.target.value } })}
                placeholder="587"
                autoComplete="off"
              />
            </Field>
            <Field label="Username">
              <input
                className={inputClasses}
                value={notify.email.username}
                onChange={(e) => patchNotify({ email: { ...notify.email, username: e.target.value } })}
                placeholder="you@example.com"
                autoComplete="off"
              />
            </Field>
            <Field label="Password">
              <input
                className={inputClasses}
                type="password"
                value={smtpPass}
                onChange={(e) => setSmtpPass(e.target.value)}
                placeholder={notify.email.hasPassword ? "Stored — leave blank to keep" : "app password"}
                autoComplete="new-password"
              />
            </Field>
            <Field label="From">
              <input
                className={inputClasses}
                value={notify.email.from}
                onChange={(e) => patchNotify({ email: { ...notify.email, from: e.target.value } })}
                placeholder="serverhub@example.com"
                autoComplete="off"
              />
            </Field>
            <Field label="To">
              <input
                className={inputClasses}
                value={notify.email.to}
                onChange={(e) => patchNotify({ email: { ...notify.email, to: e.target.value } })}
                placeholder="you@example.com"
                autoComplete="off"
              />
            </Field>
          </div>
          <div className="flex items-center justify-between gap-4 py-4 mt-2">
            <span className="text-[13px] text-gray-500 dark:text-gray-400">Use TLS (off = plain, on = STARTTLS or :465 implicit)</span>
            <Toggle
              checked={notify.email.tls}
              onChange={() => patchNotify({ email: { ...notify.email, tls: !notify.email.tls } })}
              label="SMTP TLS"
            />
          </div>

          <div className="mt-4 pt-4 border-t border-gray-200 dark:border-white/10">
            <Field label="Default notification group (email recipients)">
              <select
                className="w-full bg-white dark:bg-black/50 border border-gray-300 dark:border-white/10 rounded-lg font-body text-sm text-gray-900 dark:text-white px-4 py-2.5 outline-none focus:border-cyan-500 dark:focus:border-cyan-400 appearance-none"
                value={notify.email.emailGroupId ?? ""}
                onChange={(e) =>
                  patchNotify({
                    email: {
                      ...notify.email,
                      emailGroupId: e.target.value === "" ? null : Number(e.target.value),
                    },
                  })
                }
              >
                <option value="">Legacy To address (single recipient)</option>
                {groups.map((g) => (
                  <option key={g.id} value={g.id}>
                    {g.name} ({g.memberCount} member{g.memberCount === 1 ? "" : "s"})
                  </option>
                ))}
              </select>
            </Field>
            <p className="text-gray-500 dark:text-gray-400 text-[12px] mt-2">
              {notify.email.emailGroupId
                ? "Alert mail goes to every member of the selected group."
                : "No group selected — alert mail goes to the To address above."}
            </p>
          </div>
        </div>

        <div className="flex items-center gap-3 flex-wrap mt-8 pt-6 border-t border-gray-200 dark:border-white/10">
          <CyberButton disabled={ntBusy} onClick={() => void saveNotify()}>
            <Check size={16} /> {ntBusy ? "Saving…" : "Save routing"}
          </CyberButton>
          <button
            className="inline-flex items-center gap-2 rounded-lg text-sm font-medium px-5 py-2.5 cursor-pointer border transition-colors duration-200 bg-white dark:bg-transparent border-gray-300 dark:border-white/20 text-gray-700 dark:text-white hover:bg-gray-50 dark:hover:bg-white/5"
            onClick={() => void probeNotify()}
          >
            <Send size={16} /> Send test
          </button>
        </div>
        {ntTest && (
          <div className="mt-4 p-3 bg-gray-50 dark:bg-black/30 border border-gray-200 dark:border-white/10 rounded-lg">
            <p className="font-mono text-xs text-gray-600 dark:text-cyan-400">
              telegram: {ntTest.telegram} · email: {ntTest.email}
            </p>
          </div>
        )}
      </GlassCard>

      <NotificationGroupManager groups={groups} refreshGroups={refreshGroups} setNotify={setNotify} />
    </div>
  );
}

function AlertRow({ title, hint, on, onFlip }: { title: string; hint: string; on: boolean; onFlip: () => void }) {
  return (
    <div className="flex items-center justify-between gap-4 py-4">
      <div>
        <div className="font-medium text-sm text-gray-900 dark:text-white">{title}</div>
        <div className="text-gray-500 dark:text-gray-400 text-[13px] mt-0.5">{hint}</div>
      </div>
      <Toggle checked={on} onChange={onFlip} label={title} />
    </div>
  );
}

function NotificationGroupManager({
  groups,
  refreshGroups,
  setNotify,
}: {
  groups: NotificationGroup[];
  refreshGroups: (id?: number) => Promise<void>;
  setNotify: React.Dispatch<React.SetStateAction<NotifySettings | null>>;
}) {
  const { user } = useAuth();
  const pushToast = useUi((s) => s.pushToast);
  
  const [groupDetail, setGroupDetail] = useState<NotificationGroupDetail | null>(null);
  const [newGroupName, setNewGroupName] = useState("");
  const [newGroupDesc, setNewGroupDesc] = useState("");
  const [newMemberEmail, setNewMemberEmail] = useState("");
  const [groupBusy, setGroupBusy] = useState(false);
  const [confirmDeleteGroup, setConfirmDeleteGroup] = useState<number | null>(null);

  const isAdmin = user?.role === "admin";
  const canEditGroups = user?.role === "operator" || isAdmin;

  const inputClasses = "w-full bg-white dark:bg-black/50 border border-gray-300 dark:border-white/10 rounded-lg font-body text-sm text-gray-900 dark:text-white px-4 py-2 outline-none focus:border-cyan-500 dark:focus:border-cyan-400 focus:ring-1 focus:ring-cyan-500/50 placeholder:text-gray-400 dark:placeholder:text-gray-600 transition-all";

  const selectGroup = async (id: number) => {
    try {
      setGroupDetail(await api.notificationGroup(id));
      setNewMemberEmail("");
    } catch (err) {
      pushToast(err instanceof Error ? err.message : "Load failed", true);
    }
  };

  const createGroup = async () => {
    if (!newGroupName.trim()) return;
    setGroupBusy(true);
    try {
      const g = await api.createNotificationGroup(newGroupName.trim(), newGroupDesc.trim());
      setNewGroupName("");
      setNewGroupDesc("");
      pushToast(`Group "${g.name}" created.`);
      await refreshGroups(g.id);
      await selectGroup(g.id);
    } catch (err) {
      pushToast(err instanceof Error ? err.message : "Create failed", true);
    } finally {
      setGroupBusy(false);
    }
  };

  const addMember = async () => {
    if (!groupDetail || !newMemberEmail.trim()) return;
    setGroupBusy(true);
    try {
      await api.addGroupMember(groupDetail.id, newMemberEmail.trim());
      setNewMemberEmail("");
      pushToast("Member added.");
      await refreshGroups(groupDetail.id);
      await selectGroup(groupDetail.id);
    } catch (err) {
      pushToast(err instanceof Error ? err.message : "Add failed", true);
    } finally {
      setGroupBusy(false);
    }
  };

  const removeMember = async (memberId: number) => {
    if (!groupDetail) return;
    try {
      await api.removeGroupMember(groupDetail.id, memberId);
      pushToast("Member removed.");
      await refreshGroups(groupDetail.id);
      await selectGroup(groupDetail.id);
    } catch (err) {
      pushToast(err instanceof Error ? err.message : "Remove failed", true);
    }
  };

  const deleteGroup = async () => {
    if (confirmDeleteGroup == null) return;
    const doomed = confirmDeleteGroup;
    try {
      await api.deleteNotificationGroup(doomed);
      pushToast("Group deleted. Email falls back to the To address.");
      setConfirmDeleteGroup(null);
      if (groupDetail?.id === doomed) setGroupDetail(null);
      const fresh = await api.notifySettings();
      setNotify(fresh);
      await refreshGroups();
    } catch (err) {
      pushToast(err instanceof Error ? err.message : "Delete failed", true);
    }
  };

  return (
    <GlassCard>
      <div className="flex items-start justify-between gap-3 mb-6">
        <div>
          <h2 className="text-xl font-semibold text-gray-900 dark:text-white">Notification Groups</h2>
          <p className="text-gray-500 dark:text-gray-400 text-sm mt-1">Recipient lists for alert emails. Select one as the default group above.</p>
        </div>
        <span className="text-gray-500 dark:text-gray-400 flex items-center justify-center w-10 h-10 rounded-full bg-gray-100 dark:bg-white/5 border border-gray-200 dark:border-white/10">
          <Users size={18} />
        </span>
      </div>

      {groups.length === 0 ? (
        <p className="text-gray-500 dark:text-gray-400 text-sm mb-6">No groups yet. Create one to send alert mail to multiple recipients.</p>
      ) : (
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-3 mb-6">
          {groups.map((g) => {
            const isActive = groupDetail?.id === g.id;
            return (
              <button
                key={g.id}
                onClick={() => void selectGroup(g.id)}
                className={`flex flex-col items-start p-3 rounded-xl border text-left transition-all ${
                  isActive
                    ? "bg-cyan-50 dark:bg-cyan-900/20 border-cyan-500 shadow-[0_0_10px_rgba(6,182,212,0.15)]"
                    : "bg-white dark:bg-black/30 border-gray-200 dark:border-white/10 hover:border-cyan-300 dark:hover:border-white/30"
                }`}
              >
                <span className={`font-semibold text-sm truncate w-full ${isActive ? 'text-cyan-700 dark:text-cyan-400' : 'text-gray-900 dark:text-white'}`}>{g.name}</span>
                <span className={`text-xs mt-1 font-mono ${isActive ? 'text-cyan-600/80 dark:text-cyan-400/80' : 'text-gray-500 dark:text-gray-400'}`}>{g.memberCount}/10 members</span>
              </button>
            );
          })}
        </div>
      )}

      {canEditGroups && (
        <div className="bg-gray-50 dark:bg-black/20 rounded-xl p-4 border border-gray-200 dark:border-white/10 mb-6">
          <h4 className="text-sm font-semibold text-gray-900 dark:text-white mb-3">Create New Group</h4>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 mb-4">
            <input
              className={inputClasses}
              value={newGroupName}
              onChange={(e) => setNewGroupName(e.target.value)}
              placeholder="Group name (e.g. Infrastructure)"
              autoComplete="off"
            />
            <input
              className={inputClasses}
              value={newGroupDesc}
              onChange={(e) => setNewGroupDesc(e.target.value)}
              placeholder="Description (optional)"
              autoComplete="off"
            />
          </div>
          <CyberButton disabled={groupBusy || !newGroupName.trim()} onClick={() => void createGroup()}>
            <Plus size={16} /> {groupBusy ? "Creating…" : "Create group"}
          </CyberButton>
        </div>
      )}

      {groupDetail && (
        <div className="bg-gray-100/50 dark:bg-black/40 border border-gray-200 dark:border-cyan-500/30 rounded-xl px-5 py-5 relative overflow-hidden shadow-inner">
          <div className="absolute top-0 left-0 w-1 h-full bg-cyan-500"></div>
          
          <div className="flex items-start justify-between gap-3 flex-wrap">
            <div>
              <div className="font-bold text-lg text-gray-900 dark:text-white flex items-center gap-2">
                {groupDetail.name}
              </div>
              {groupDetail.description && (
                <div className="text-gray-500 dark:text-gray-400 text-sm mt-1">{groupDetail.description}</div>
              )}
            </div>
            {isAdmin && (
              <button
                className="inline-flex items-center gap-1.5 text-xs text-red-600 dark:text-red-400 hover:text-red-700 dark:hover:text-red-300 font-medium px-3 py-1.5 rounded-lg hover:bg-red-50 dark:hover:bg-red-950/30 transition-colors"
                onClick={() => setConfirmDeleteGroup(groupDetail.id)}
              >
                <Trash2 size={14} /> Delete group
              </button>
            )}
          </div>
          
          <div className="mt-5 flex flex-col gap-2">
            <h4 className="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 mb-1">Members</h4>
            {groupDetail.members.length === 0 && (
              <p className="text-gray-500 dark:text-gray-400 text-sm italic">No members yet — an empty group falls back to the To address.</p>
            )}
            {groupDetail.members.map((m) => (
              <div key={m.id} className="flex items-center justify-between gap-3 py-2 px-3 bg-white dark:bg-white/5 rounded-lg border border-gray-200 dark:border-white/10 group">
                <span className="font-mono text-sm text-gray-800 dark:text-gray-200 break-all">{m.email}</span>
                {canEditGroups && (
                  <button
                    className="text-gray-400 hover:text-red-500 dark:text-gray-500 dark:hover:text-red-400 text-xs font-medium opacity-0 group-hover:opacity-100 transition-opacity flex items-center gap-1"
                    onClick={() => void removeMember(m.id)}
                  >
                    <Trash2 size={12} /> Remove
                  </button>
                )}
              </div>
            ))}
          </div>
          
          {canEditGroups && groupDetail.members.length < 10 && (
            <div className="flex gap-3 mt-5">
              <input
                className={`${inputClasses} flex-1`}
                value={newMemberEmail}
                onChange={(e) => setNewMemberEmail(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") void addMember();
                }}
                placeholder="teammate@example.com"
                autoComplete="off"
              />
              <CyberButton disabled={groupBusy || !newMemberEmail.trim()} onClick={() => void addMember()}>
                <Plus size={16} /> Add
              </CyberButton>
            </div>
          )}
        </div>
      )}

      {confirmDeleteGroup != null && (
        <ConfirmModal
          title="Delete notification group?"
          body="Its members are removed and alert email falls back to the To address. This cannot be undone."
          confirmLabel="Delete group"
          onClose={() => setConfirmDeleteGroup(null)}
          onConfirm={() => void deleteGroup()}
        />
      )}
    </GlassCard>
  );
}
