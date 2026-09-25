import { useCallback, useEffect, useMemo, useState } from "react";
import { Outlet, useLocation, useNavigate, Link } from "react-router";
import {
  Activity,
  Database,
  FolderGit2,
  LayoutGrid,
  LogOut,
  Moon,
  Radar,
  Rocket,
  Search,
  Ship,
  SlidersHorizontal,
  Sun,
  User,
  X,
} from "lucide-react";
import { api } from "../lib/api";
import type { Project } from "../lib/types";
import { useAuth, useUi } from "../stores/store";
import { useTheme } from "../lib/useTheme";

/* ---------- floating brand (top-left) ---------- */

function Brand() {
  return (
    <Link
      className="fixed top-5 left-6 z-40 flex items-center gap-2.5 no-underline max-md:left-3"
      to="/"
      aria-label="ServerHub home"
    >
      <span className="w-[34px] h-[34px] rounded-full bg-accent-deep dark:bg-ember text-white dark:text-black flex items-center justify-center">
        <User size={17} strokeWidth={2.2} />
      </span>
      <span className="font-head font-bold text-[17px] max-md:hidden">
        ServerHub
      </span>
    </Link>
  );
}

/* ---------- command palette ---------- */

function CommandPalette({ onClose }: { onClose: () => void }) {
  const nav = useNavigate();
  const [q, setQ] = useState("");
  const [projects, setProjects] = useState<Project[]>([]);
  const [sel, setSel] = useState(0);

  useEffect(() => {
    api
      .projects()
      .then(setProjects)
      .catch(() => setProjects([]));
  }, []);

  const items = useMemo(() => {
    const needle = q.trim().toLowerCase();
    const pages = [
      {
        icon: <LayoutGrid size={15} />,
        title: "Go to Dashboard",
        sub: "page",
        run: () => nav("/"),
      },
      {
        icon: <Ship size={15} />,
        title: "View all ships",
        sub: "page",
        run: () => nav("/fleet"),
      },
      {
        icon: <Database size={15} />,
        title: "Browse databases",
        sub: "page",
        run: () => nav("/databases"),
      },
      {
        icon: <Activity size={15} />,
        title: "View Host Telemetry",
        sub: "page",
        run: () => nav("/telemetry"),
      },
      {
        icon: <SlidersHorizontal size={15} />,
        title: "Go to Settings",
        sub: "page",
        run: () => nav("/settings"),
      },
      {
        icon: <Radar size={15} />,
        title: "Scan shipyard",
        sub: "action",
        run: () => nav("/?scan=1"),
      },
    ];
    const ships = projects.map((p) => ({
      icon: <FolderGit2 size={15} />,
      title: p.name,
      sub: "open ship",
      run: () => nav(`/?project=${p.id}`),
    }));
    const all = [...pages, ...ships];
    if (!needle) return all;
    return all.filter((i) => i.title.toLowerCase().includes(needle));
  }, [q, projects, nav]);

  const choose = useCallback(
    (i: number) => {
      const item = items[i];
      if (!item) return;
      onClose();
      item.run();
    },
    [items, onClose],
  );

  return (
    <div
      className="fixed inset-0 z-[60] flex items-start justify-center px-4 pt-[12vh] bg-[rgba(28,25,23,0.28)] dark:bg-[rgba(0,0,0,0.55)]"
      onClick={onClose}
    >
      <div
        className="w-full max-w-[560px] bg-white dark:bg-panel border border-line dark:border-edge rounded-card overflow-hidden shadow-chrome"
        onClick={(e) => e.stopPropagation()}
      >
        <input
          autoFocus
          className="w-full outline-none text-[15px] px-5 py-4 border-x-0 border-t-0 border-b border-line dark:border-edge bg-white dark:bg-panel text-ink dark:text-bone"
          placeholder="Search ships, pages, actions…"
          value={q}
          onChange={(e) => {
            setQ(e.target.value);
            setSel(0);
          }}
          onKeyDown={(e) => {
            if (e.key === "Escape") onClose();
            if (e.key === "ArrowDown") {
              e.preventDefault();
              setSel((s) => Math.min(s + 1, items.length - 1));
            }
            if (e.key === "ArrowUp") {
              e.preventDefault();
              setSel((s) => Math.max(s - 1, 0));
            }
            if (e.key === "Enter") choose(sel);
          }}
        />
        <div className="max-h-[320px] overflow-y-auto p-2">
          {items.length === 0 && (
            <div className="p-4 text-muted dark:text-fog text-[13px]">
              Nothing on this heading.
            </div>
          )}
          {items.map((item, i) => (
            <button
              key={`${item.sub}-${item.title}`}
              className={`flex items-center gap-3 w-full text-left bg-transparent border-0 cursor-pointer px-3 py-2.5 rounded-[10px] text-[13px] text-ink dark:text-bone hover:bg-paper dark:hover:bg-emboss ${
                i === sel ? "bg-paper dark:bg-emboss" : ""
              }`}
              onMouseEnter={() => setSel(i)}
              onClick={() => choose(i)}
            >
              <span className="text-muted dark:text-fog flex">{item.icon}</span>
              {item.title}
              <span className="text-muted dark:text-fog text-xs ml-auto">
                {item.sub}
              </span>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}

/* ---------- app shell ---------- */

export function AppShell({ online }: { online: boolean }) {
  const nav = useNavigate();
  const loc = useLocation();
  const { user, logout } = useAuth();
  const { theme, setTheme } = useTheme();
  const { toasts, dismissToast, paletteOpen, setPaletteOpen, pushToast } =
    useUi();

  useEffect(() => {
    const fn = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPaletteOpen(!paletteOpen);
      }
    };
    window.addEventListener("keydown", fn);
    return () => window.removeEventListener("keydown", fn);
  }, [paletteOpen, setPaletteOpen]);

  const onDock = (path: string) => {
    if (loc.pathname !== path) nav(path);
  };

  const dockBtn = (active: boolean) =>
    `relative w-11 h-11 rounded-full flex items-center justify-center bg-transparent border-0 cursor-pointer hover:bg-paper dark:hover:bg-emboss hover:text-ink dark:hover:text-bone ${
      active ? "text-ink dark:text-bone" : "text-muted dark:text-fog"
    }`;

  return (
    <>
      <Brand />
      <button
        className="bg-white dark:bg-panel border border-line dark:border-edge rounded-full shadow-chrome fixed top-5 left-1/2 -translate-x-1/2 z-40 flex items-center justify-between gap-2.5 h-[42px] pl-4 pr-2 w-[calc(100vw-190px)] min-w-0 sm:w-auto sm:min-w-[480px] text-muted dark:text-fog text-[13px] cursor-pointer hover:border-accent dark:hover:border-ember"
        onClick={() => setPaletteOpen(true)}
      >
        <div className="flex items-center justify-center gap-3">
          <Search size={15} />
          <span className="truncate">Search ships, actions, or press</span>
        </div>

        <span className="flex items-center justify-center font-mono text-[11px] bg-paper dark:bg-abyss border border-line dark:border-edge rounded-md px-2.5 py-1.5 text-muted dark:text-fog">
          ⌘ K
        </span>
      </button>
      <div className="fixed top-5 right-6 z-40 flex items-center gap-2 max-md:right-3">
        <button
          className="bg-white dark:bg-panel border border-line dark:border-edge rounded-full shadow-chrome w-[42px] h-[42px] flex items-center justify-center text-muted dark:text-fog hover:text-ink dark:hover:text-bone cursor-pointer"
          title={
            theme === "cyberdeck"
              ? "Switch to light mode"
              : "Switch to dark mode"
          }
          aria-label="Toggle color mode"
          onClick={() =>
            setTheme(theme === "cyberdeck" ? "workbench" : "cyberdeck")
          }
        >
          {theme === "cyberdeck" ? <Sun size={16} /> : <Moon size={16} />}
        </button>
        <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded-full shadow-chrome flex items-center gap-2 h-[42px] px-4 font-mono text-xs">
          <span
            className={
              online
                ? "w-2 h-2 rounded-full bg-moss shadow-[0_0_0_3px_rgba(22,163,74,0.18)]"
                : "w-2 h-2 rounded-full bg-brick shadow-[0_0_0_3px_rgba(220,38,38,0.15)]"
            }
          />
          {online ? "Online" : "Offline"}
        </div>
      </div>

      <Outlet />

      <nav
        className="bg-white dark:bg-panel border border-line dark:border-edge rounded-full shadow-chrome fixed bottom-6 left-1/2 -translate-x-1/2 z-40 flex items-center gap-1 px-3 py-2"
        aria-label="Primary"
      >
        <button
          className={dockBtn(loc.pathname === "/")}
          title="Dashboard"
          aria-label="Dashboard"
          onClick={() => onDock("/")}
        >
          <LayoutGrid size={19} />
          {loc.pathname === "/" && (
            <span className="absolute bottom-0.5 left-1/2 -translate-x-1/2 w-1 h-1 rounded-full bg-accent dark:bg-ember" />
          )}
        </button>
        <button
          className={dockBtn(loc.pathname === "/fleet")}
          title="Fleet"
          aria-label="Fleet"
          onClick={() => onDock("/fleet")}
        >
          <Ship size={19} />
          {loc.pathname === "/fleet" && (
            <span className="absolute bottom-0.5 left-1/2 -translate-x-1/2 w-1 h-1 rounded-full bg-accent dark:bg-ember" />
          )}
        </button>
        <button
          className={dockBtn(loc.pathname === "/databases")}
          title="Databases"
          aria-label="Databases"
          onClick={() => onDock("/databases")}
        >
          <Database size={19} />
          {loc.pathname === "/databases" && (
            <span className="absolute bottom-0.5 left-1/2 -translate-x-1/2 w-1 h-1 rounded-full bg-accent dark:bg-ember" />
          )}
        </button>
        <button
          className={dockBtn(loc.pathname === "/telemetry")}
          title="Telemetry"
          aria-label="Telemetry"
          onClick={() => onDock("/telemetry")}
        >
          <Activity size={19} />
          {loc.pathname === "/telemetry" && (
            <span className="absolute bottom-0.5 left-1/2 -translate-x-1/2 w-1 h-1 rounded-full bg-accent dark:bg-ember" />
          )}
        </button>
        <button
          className={dockBtn(loc.pathname === "/settings")}
          title="Settings"
          aria-label="Settings"
          onClick={() => onDock("/settings")}
        >
          <SlidersHorizontal size={19} />
          {loc.pathname === "/settings" && (
            <span className="absolute bottom-0.5 left-1/2 -translate-x-1/2 w-1 h-1 rounded-full bg-accent dark:bg-ember" />
          )}
        </button>
        <span className="w-px h-6 bg-line dark:bg-edge mx-1.5" />
        <button
          className="relative w-11 h-11 rounded-full flex items-center justify-center bg-transparent border-0 cursor-pointer text-muted dark:text-fog hover:bg-paper dark:hover:bg-emboss hover:text-ink dark:hover:text-bone"
          title={user ? `Sign out ${user.username}` : "Sign out"}
          aria-label="Sign out"
          onClick={async () => {
            await logout();
            pushToast("Signed out. Fair winds.");
            nav("/login");
          }}
        >
          <LogOut size={19} />
        </button>
      </nav>

      {paletteOpen && <CommandPalette onClose={() => setPaletteOpen(false)} />}

      <div className="fixed bottom-6 right-6 z-[80] flex flex-col gap-2 max-w-[min(360px,calc(100vw-2rem))]">
        {toasts.map((t) => (
          <div
            key={t.id}
            className={`rounded-xl px-4 py-2.5 text-[13px] flex gap-2.5 items-center shadow-chrome ${
              t.bad
                ? "bg-red-900 dark:bg-red-900 text-white dark:text-red-100"
                : "bg-ink dark:bg-bone text-paper dark:text-ink"
            }`}
          >
            {t.bad ? <X size={14} /> : <Rocket size={14} />}
            <span>{t.text}</span>
            <button
              onClick={() => dismissToast(t.id)}
              className="bg-transparent border-0 text-inherit cursor-pointer"
              aria-label="Dismiss"
            >
              <X size={13} />
            </button>
          </div>
        ))}
      </div>
    </>
  );
}
