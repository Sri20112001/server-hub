# ServerHub — Bridge Console (React client)

Physical-workbench UI for the ServerHub control plane.
Design source of truth: `src/design/stitch_serverhub_mission_control_ui/`
(`physical_workbench/DESIGN.md` + dashboard/settings screens).
No header bar, no sidebar — floating command pill, status pill, and bottom dock.

Styling is **Tailwind CSS v4** (`@tailwindcss/vite`): `src/index.css` holds only
the `@theme` tokens (`bg-paper`, `text-muted`, `border-line`, `bg-accent`,
`font-head`, `rounded-card`, `shadow-chrome`, …) plus base element styles.
Every component composes utilities directly in `className` — no `@apply`,
no CSS modules, no inline styles (except two runtime theme-preview dots in
Settings, whose hex values come from theme data and can't be static classes).
No SCSS — Tailwind v4 covers variables, nesting, and variants, so a
preprocessor would only add build weight.

## Run

```bash
cp .env.example .env   # VITE_API_URL=http://localhost:4000
npm install
npm run dev            # http://localhost:5173 (allowed by API CORS)
```

Serve the production build from the `/server-hub` subpath (router basename and
asset base are set). Example Caddy (paths preserved, no stripping):

```caddy
handle /server-hub/api/* {
    reverse_proxy localhost:4000
}
handle /server-hub/* {
    root * /srv/serverhub/client/dist
    try_files {path} /server-hub/index.html
    file_server
}
```

## Pages

- `/login` — operator sign-in (cookie session)
- `/` — bridge dashboard: server load (click for telemetry timeline), fleet, launches, health, ship cards,
  launch table, audit feed, ship detail drawer (helm actions incl. terminal, stations,
  **live logs**, launch log with **rollback**, secret vault, **snapshots**), new-dispatch modal,
  staged **deploy progress** modal, **scan-shipyard modal** (Docker + directory auto-discovery),
  `⌘K` command palette (pages, ships, scan action), **live SSE signals** (auto-refresh + toasts
  for deploys, lifecycle, gateway, backups, thresholds), browser toasts for health/deploy/resource alerts
  (Settings → Alerts toggles)
- `/settings` — profile & password rotation, host telemetry, alert toggles
  (local), danger zone (typed-confirm wipe, restart runbook)

Backend must be running (`../server`, default `http://localhost:4000`).
