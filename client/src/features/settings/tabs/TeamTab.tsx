import React, { useCallback, useEffect, useState } from "react";
import { UserPlus, Trash2, Users } from "lucide-react";
import { api } from "../../../lib/api";
import { useAuth, useUi } from "../../../stores/store";
import { Field, ConfirmModal } from "../../../components/ui";
import { GlassCard } from "../components/GlassCard";
import { CyberButton } from "../components/CyberButton";

interface UserItem {
  username: string;
  role: string;
  createdAt: string;
}

export function TeamTab() {
  const { user: currentUser } = useAuth();
  const pushToast = useUi((s) => s.pushToast);

  const [users, setUsers] = useState<UserItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [showAddModal, setShowAddModal] = useState(false);
  const [userToDelete, setUserToDelete] = useState<string | null>(null);

  // New user form state
  const [newUsername, setNewUsername] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [newRole, setNewRole] = useState("operator");
  const [addBusy, setAddBusy] = useState(false);

  // Shared fetcher for event handlers (create/role-change/delete refresh).
  const loadUsers = useCallback(async () => {
    try {
      const data = await api.users();
      setUsers(data);
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Failed to load team members", true);
    } finally {
      setLoading(false);
    }
  }, [pushToast]);

  // Initial load fetches inline so the effect performs no synchronous
  // set-state on setup; the cancelled flag guards unmount races.
  useEffect(() => {
    let cancelled = false;
    const run = async () => {
      try {
        const data = await api.users();
        if (!cancelled) setUsers(data);
      } catch (e) {
        if (!cancelled) pushToast(e instanceof Error ? e.message : "Failed to load team members", true);
      } finally {
        if (!cancelled) setLoading(false);
      }
    };
    void run();
    return () => {
      cancelled = true;
    };
  }, [pushToast]);

  const handleCreateUser = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newUsername.trim() || !newPassword) return;
    setAddBusy(true);
    try {
      await api.createUser(newUsername.trim(), newPassword, newRole);
      pushToast(`Team member "${newUsername}" added`);
      setNewUsername("");
      setNewPassword("");
      setNewRole("operator");
      setShowAddModal(false);
      void loadUsers();
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Failed to create user", true);
    } finally {
      setAddBusy(false);
    }
  };

  const handleRoleChange = async (username: string, nextRole: string) => {
    try {
      await api.setUserRole(username, nextRole);
      pushToast(`Role for "${username}" updated to ${nextRole}`);
      void loadUsers();
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Failed to update role", true);
    }
  };

  const handleDeleteUser = async (username: string) => {
    try {
      await api.deleteUser(username);
      pushToast(`User "${username}" removed`);
      setUserToDelete(null);
      void loadUsers();
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Failed to remove user", true);
    }
  };

  return (
    <div className="flex flex-col gap-6">
      <GlassCard>
        <div className="flex items-start justify-between gap-3 mb-6 flex-wrap">
          <div>
            <div className="flex items-center gap-2">
              <Users size={20} className="text-accent dark:text-ember" />
              <h2 className="text-xl font-semibold text-ink dark:text-bone">Team Members & Permissions</h2>
            </div>
            <p className="text-muted dark:text-fog text-sm mt-1">
              Manage accounts, technician roles, and system access levels for your team.
            </p>
          </div>
          {currentUser?.role === "admin" && (
            <button
              onClick={() => setShowAddModal(true)}
              className="flex items-center gap-1.5 px-3.5 py-2 bg-accent dark:bg-ember text-white dark:text-black text-sm font-semibold rounded-lg hover:bg-accent-hover dark:hover:bg-ember-hover shadow-sm cursor-pointer"
            >
              <UserPlus size={15} /> Add Team Member
            </button>
          )}
        </div>

        {/* Roles explanation card */}
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 mb-6 text-xs font-mono">
          <div className="p-4 rounded-2xl border border-outline-variant/20 bg-surface-container-low">
            <span className="font-semibold text-primary">Admin</span>
            <p className="text-on-surface-variant font-sans mt-1">Full control over servers, deployments, notifications, and team access.</p>
          </div>
          <div className="p-4 rounded-2xl border border-outline-variant/20 bg-surface-container-low">
            <span className="font-semibold text-tertiary">Technician (Operator)</span>
            <p className="text-on-surface-variant font-sans mt-1">Deploys projects, creates backups, restarts containers, and inspects metrics.</p>
          </div>
          <div className="p-4 rounded-2xl border border-outline-variant/20 bg-surface-container-low">
            <span className="font-semibold text-outline">Viewer</span>
            <p className="text-on-surface-variant font-sans mt-1">Read-only access to status dashboards, activity logs, and system health.</p>
          </div>
        </div>

        {/* Users list */}
        <div className="divide-y divide-outline-variant/10 border border-outline-variant/20 rounded-2xl overflow-hidden bg-surface-container-lowest shadow-sm">
          {loading && (
            <div className="p-6 text-center text-xs text-on-surface-variant font-mono animate-pulse">
              Loading team directory…
            </div>
          )}

          {!loading && users.length === 0 && (
            <div className="p-8 text-center text-on-surface-variant text-sm">
              No additional users configured.
            </div>
          )}

          {!loading &&
            users.map((u) => {
              const isSelf = u.username === currentUser?.username;
              return (
                <div key={u.username} className="p-4 flex items-center justify-between gap-4 flex-wrap hover:bg-surface-container-low/40 transition-colors">
                  <div className="flex items-center gap-3">
                    <span className="w-10 h-10 rounded-full bg-surface-container-low border border-outline-variant/30 flex items-center justify-center font-head font-bold text-base text-primary">
                      {u.username.slice(0, 1).toUpperCase()}
                    </span>
                    <div>
                      <div className="flex items-center gap-2">
                        <strong className="text-on-surface text-sm">{u.username}</strong>
                        {isSelf && (
                          <span className="px-2 py-0.5 rounded-full text-[10px] font-mono bg-surface-container-low border border-outline-variant/30 text-on-surface-variant">
                            You
                          </span>
                        )}
                      </div>
                      <div className="text-[11px] text-on-surface-variant font-mono mt-0.5">
                        {u.createdAt ? `Joined: ${new Date(u.createdAt).toLocaleDateString()}` : "Local Account"}
                      </div>
                    </div>
                  </div>

                  <div className="flex items-center gap-3">
                    {currentUser?.role === "admin" && !isSelf ? (
                      <select
                        value={u.role}
                        onChange={(e) => void handleRoleChange(u.username, e.target.value)}
                        className="bg-surface-container-low border border-outline-variant/30 rounded-xl px-3 py-1 text-xs font-mono text-on-surface outline-none focus:border-primary cursor-pointer transition-colors"
                      >
                        <option value="admin">Admin</option>
                        <option value="operator">Technician</option>
                        <option value="viewer">Viewer</option>
                      </select>
                    ) : (
                      <span className="inline-flex items-center gap-1 font-mono text-xs px-3 py-1 rounded-full border border-outline-variant/30 bg-surface-container-low text-on-surface uppercase">
                        {u.role === "operator" ? "Technician" : u.role}
                      </span>
                    )}

                    {currentUser?.role === "admin" && !isSelf && (
                      <button
                        onClick={() => setUserToDelete(u.username)}
                        className="p-1.5 text-on-surface-variant hover:text-error transition-colors cursor-pointer rounded-full border border-outline-variant/30 hover:border-error/40"
                        title="Delete user"
                      >
                        <Trash2 size={14} />
                      </button>
                    )}
                  </div>
                </div>
              );
            })}
        </div>
      </GlassCard>

      {/* Add User Modal */}
      {showAddModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-xs p-4" onClick={() => setShowAddModal(false)}>
          <div className="bg-surface-container-lowest border border-outline-variant/30 rounded-2xl p-6 w-full max-w-md shadow-2xl" onClick={(e) => e.stopPropagation()}>
            <h3 className="text-lg font-bold font-head text-on-surface mb-1">Add Team Member</h3>
            <p className="text-xs text-on-surface-variant mb-4">Grant dashboard or technician access to an operator.</p>

            <form onSubmit={(e) => void handleCreateUser(e)} className="space-y-4">
              <Field label="Username (2-64 characters)">
                <input
                  required
                  value={newUsername}
                  onChange={(e) => setNewUsername(e.target.value)}
                  placeholder="e.g. devops.lead, support_agent"
                  className="w-full bg-surface-container-low border border-outline-variant/30 rounded-xl px-3.5 py-2.5 text-on-surface text-sm outline-none font-mono focus:border-primary transition-colors"
                />
              </Field>

              <Field label="Initial Password (8+ characters)">
                <input
                  required
                  type="password"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  placeholder="••••••••••••"
                  className="w-full bg-surface-container-low border border-outline-variant/30 rounded-xl px-3.5 py-2.5 text-on-surface text-sm outline-none font-mono focus:border-primary transition-colors"
                />
              </Field>

              <Field label="Role & Access Level">
                <select
                  value={newRole}
                  onChange={(e) => setNewRole(e.target.value)}
                  className="w-full bg-surface-container-low border border-outline-variant/30 rounded-xl px-3.5 py-2.5 text-on-surface text-sm outline-none font-mono focus:border-primary transition-colors cursor-pointer"
                >
                  <option value="operator">Technician (Deploy, Backup, Restart)</option>
                  <option value="viewer">Viewer (Read-only)</option>
                  <option value="admin">Administrator (Full Control)</option>
                </select>
              </Field>

              <div className="flex justify-end gap-2.5 pt-4 border-t border-outline-variant/20">
                <button
                  type="button"
                  onClick={() => setShowAddModal(false)}
                  className="px-5 py-2 text-sm text-on-surface-variant hover:text-on-surface border border-outline-variant/30 rounded-full cursor-pointer bg-surface-container-low hover:bg-surface-container transition-colors"
                >
                  Cancel
                </button>
                <CyberButton type="submit" disabled={addBusy || !newUsername.trim() || newPassword.length < 8}>
                  {addBusy ? "Creating…" : "Create Member"}
                </CyberButton>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Delete User Confirm */}
      {userToDelete && (
        <ConfirmModal
          title={`Remove user "${userToDelete}"?`}
          body="This user will immediately lose access to ServerHub. All their active sessions will be terminated."
          confirmLabel="Remove User"
          requireText={userToDelete}
          onClose={() => setUserToDelete(null)}
          onConfirm={() => void handleDeleteUser(userToDelete)}
        />
      )}
    </div>
  );
}
