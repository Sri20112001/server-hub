import React, { useEffect, useState } from "react";
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

  const loadUsers = async () => {
    try {
      const data = await api.users();
      setUsers(data);
    } catch (e) {
      pushToast(e instanceof Error ? e.message : "Failed to load team members", true);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadUsers();
  }, []);

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
          <div className="p-3 rounded-xl border border-line dark:border-edge bg-paper/50 dark:bg-abyss/50">
            <span className="font-semibold text-accent dark:text-ember">Admin</span>
            <p className="text-muted dark:text-fog font-sans mt-1">Full control over servers, deployments, notifications, and team access.</p>
          </div>
          <div className="p-3 rounded-xl border border-line dark:border-edge bg-paper/50 dark:bg-abyss/50">
            <span className="font-semibold text-moss">Technician (Operator)</span>
            <p className="text-muted dark:text-fog font-sans mt-1">Deploys projects, creates backups, restarts containers, and inspects metrics.</p>
          </div>
          <div className="p-3 rounded-xl border border-line dark:border-edge bg-paper/50 dark:bg-abyss/50">
            <span className="font-semibold text-stone">Viewer</span>
            <p className="text-muted dark:text-fog font-sans mt-1">Read-only access to status dashboards, activity logs, and system health.</p>
          </div>
        </div>

        {/* Users list */}
        <div className="divide-y divide-line dark:divide-edge border border-line dark:border-edge rounded-xl overflow-hidden bg-white dark:bg-panel shadow-sm">
          {loading && (
            <div className="p-6 text-center text-xs text-muted dark:text-fog font-mono animate-pulse">
              Loading team directory…
            </div>
          )}

          {!loading && users.length === 0 && (
            <div className="p-8 text-center text-muted dark:text-fog text-sm">
              No additional users configured.
            </div>
          )}

          {!loading &&
            users.map((u) => {
              const isSelf = u.username === currentUser?.username;
              return (
                <div key={u.username} className="p-4 flex items-center justify-between gap-4 flex-wrap hover:bg-paper/40 dark:hover:bg-emboss/30 transition-colors">
                  <div className="flex items-center gap-3">
                    <span className="w-10 h-10 rounded-full bg-paper dark:bg-abyss border border-line dark:border-edge flex items-center justify-center font-head font-bold text-base text-accent dark:text-ember">
                      {u.username.slice(0, 1).toUpperCase()}
                    </span>
                    <div>
                      <div className="flex items-center gap-2">
                        <strong className="text-ink dark:text-bone text-sm">{u.username}</strong>
                        {isSelf && (
                          <span className="px-1.5 py-0.5 rounded text-[10px] font-mono bg-paper dark:bg-abyss border border-line dark:border-edge text-muted dark:text-fog">
                            You
                          </span>
                        )}
                      </div>
                      <div className="text-[11px] text-muted dark:text-fog font-mono mt-0.5">
                        {u.createdAt ? `Joined: ${new Date(u.createdAt).toLocaleDateString()}` : "Local Account"}
                      </div>
                    </div>
                  </div>

                  <div className="flex items-center gap-3">
                    {currentUser?.role === "admin" && !isSelf ? (
                      <select
                        value={u.role}
                        onChange={(e) => void handleRoleChange(u.username, e.target.value)}
                        className="bg-paper dark:bg-abyss border border-line dark:border-edge rounded-lg px-2.5 py-1 text-xs font-mono text-ink dark:text-bone outline-none focus:border-accent dark:focus:border-ember cursor-pointer"
                      >
                        <option value="admin">Admin</option>
                        <option value="operator">Technician</option>
                        <option value="viewer">Viewer</option>
                      </select>
                    ) : (
                      <span className="inline-flex items-center gap-1 font-mono text-xs px-2.5 py-1 rounded border border-line dark:border-edge bg-paper dark:bg-abyss text-ink dark:text-bone uppercase">
                        {u.role === "operator" ? "Technician" : u.role}
                      </span>
                    )}

                    {currentUser?.role === "admin" && !isSelf && (
                      <button
                        onClick={() => setUserToDelete(u.username)}
                        className="p-1.5 text-muted dark:text-fog hover:text-brick transition-colors cursor-pointer rounded border border-line dark:border-edge hover:border-brick/40"
                        title="Delete user"
                      >
                        <Trash2 size={15} />
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
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-[rgba(28,25,23,0.35)] dark:bg-[rgba(0,0,0,0.65)] p-4 backdrop-blur-xs" onClick={() => setShowAddModal(false)}>
          <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-6 w-full max-w-md shadow-chrome" onClick={(e) => e.stopPropagation()}>
            <h3 className="text-lg font-bold font-head text-ink dark:text-bone mb-1">Add Team Member</h3>
            <p className="text-xs text-muted dark:text-fog mb-4">Grant dashboard or technician access to an operator.</p>

            <form onSubmit={(e) => void handleCreateUser(e)} className="space-y-3">
              <Field label="Username (2-64 characters)">
                <input
                  required
                  value={newUsername}
                  onChange={(e) => setNewUsername(e.target.value)}
                  placeholder="e.g. devops.lead, support_agent"
                  className="w-full bg-paper dark:bg-abyss border border-line dark:border-edge rounded-lg px-3 py-2 text-ink dark:text-bone text-sm outline-none font-mono focus:border-accent dark:focus:border-ember"
                />
              </Field>

              <Field label="Initial Password (8+ characters)">
                <input
                  required
                  type="password"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  placeholder="••••••••••••"
                  className="w-full bg-paper dark:bg-abyss border border-line dark:border-edge rounded-lg px-3 py-2 text-ink dark:text-bone text-sm outline-none font-mono focus:border-accent dark:focus:border-ember"
                />
              </Field>

              <Field label="Role & Access Level">
                <select
                  value={newRole}
                  onChange={(e) => setNewRole(e.target.value)}
                  className="w-full bg-paper dark:bg-abyss border border-line dark:border-edge rounded-lg px-3 py-2 text-ink dark:text-bone text-sm outline-none font-mono focus:border-accent dark:focus:border-ember"
                >
                  <option value="operator">Technician (Deploy, Backup, Restart)</option>
                  <option value="viewer">Viewer (Read-only)</option>
                  <option value="admin">Administrator (Full Control)</option>
                </select>
              </Field>

              <div className="flex justify-end gap-2 pt-4 border-t border-line dark:border-edge">
                <button
                  type="button"
                  onClick={() => setShowAddModal(false)}
                  className="px-4 py-2 text-sm text-muted dark:text-fog hover:text-ink dark:hover:text-bone border border-line dark:border-edge rounded-lg cursor-pointer"
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
