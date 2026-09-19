import { Link, useLocation } from "react-router";
import { Activity, Terminal, ExternalLink, ShieldAlert, Route } from "lucide-react";

export function NotFoundPage() {
  const location = useLocation();

  return (
    <div className="flex flex-col w-full min-h-screen items-center py-12 px-4 bg-paper dark:bg-abyss text-ink dark:text-bone font-body">
      
      {/* System Status Banner */}
      <div className="w-full max-w-2xl flex items-center justify-between mb-4 px-2">
        <div className="flex items-center gap-2">
          <span className="font-mono text-xs text-muted dark:text-fog uppercase tracking-wider">SYSTEM / RECOVERY CONSOLE</span>
          <span className="text-line dark:text-edge font-mono text-xs">::</span>
          <span className="font-mono text-xs text-accent dark:text-ember font-medium">DISPATCH_OFFLINE</span>
        </div>
        <div className="flex items-center gap-2 bg-white dark:bg-panel px-3 py-1 rounded-full border border-line dark:border-edge shadow-sm">
          <span className="relative flex h-2 w-2">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-brick opacity-75"></span>
            <span className="relative inline-flex rounded-full h-2 w-2 bg-brick"></span>
          </span>
          <span className="font-mono text-xs text-brick font-medium">SIGNAL SEVERED (404)</span>
        </div>
      </div>

      {/* Primary Mission Control Workbench Card */}
      <div className="w-full max-w-2xl bg-white dark:bg-panel rounded-card shadow-chrome border border-line dark:border-edge p-6 md:p-8 flex flex-col gap-6 relative overflow-hidden">
        {/* Subtle Top Technical Index Strip */}
        <div className="absolute top-0 inset-x-0 h-1 bg-gradient-to-r from-brick via-accent to-tint dark:to-emboss"></div>

        {/* Header & Kicker */}
        <div className="flex flex-col gap-2 pt-2">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2 bg-tint dark:bg-emboss px-2.5 py-1 rounded-full">
              <ShieldAlert className="w-3.5 h-3.5 text-brick" />
              <span className="font-body text-[11px] text-muted dark:text-fog tracking-wider uppercase font-semibold">DIAGNOSTIC / #ERR_404</span>
            </div>
            <span className="font-mono text-xs text-muted dark:text-fog/70">REF: SVR-09X</span>
          </div>
          <div className="flex flex-col sm:flex-row sm:items-baseline justify-between gap-1 mt-1">
            <h1 className="font-head text-3xl md:text-4xl tracking-tight font-bold">Sector Uncharted</h1>
            <span className="font-mono text-sm text-brick font-semibold tracking-normal">[404_NOT_FOUND]</span>
          </div>
          <p className="text-sm text-muted dark:text-fog leading-relaxed">
            The requested routing vector, daemon socket, or telemetry endpoint does not exist on this active cluster node or has been cleanly decommissioned.
          </p>
        </div>

        {/* Technical Radar Visualizer */}
        <div className="w-full bg-tint/50 dark:bg-emboss/50 rounded-lg p-4 flex flex-col sm:flex-row items-center gap-6">
          <div className="relative w-32 h-32 shrink-0 flex items-center justify-center bg-white dark:bg-panel rounded-full border border-line dark:border-edge shadow-inner overflow-hidden">
            <div className="absolute inset-2 rounded-full border border-dashed border-line/60 dark:border-edge/60"></div>
            <div className="absolute inset-6 rounded-full border border-line/40 dark:border-edge/40"></div>
            <div className="absolute inset-11 rounded-full border border-line/30 dark:border-edge/30"></div>
            <div className="absolute inset-x-2 top-1/2 h-[1px] bg-line/60 dark:bg-edge/60"></div>
            <div className="absolute inset-y-2 left-1/2 w-[1px] bg-line/60 dark:bg-edge/60"></div>
            {/* Rotating Sweep Beam */}
            <div className="absolute inset-0 rounded-full animate-spin [animation-duration:4s] [animation-timing-function:linear] pointer-events-none flex items-center justify-center">
              <div className="w-full h-[1px] bg-gradient-to-r from-transparent via-brick to-transparent opacity-40"></div>
            </div>
            {/* Disconnected Target Indicator */}
            <div className="relative z-10 flex flex-col items-center">
              <div className="w-3.5 h-3.5 rounded-full bg-brick/20 border border-brick flex items-center justify-center">
                <div className="w-1.5 h-1.5 rounded-full bg-brick"></div>
              </div>
              <span className="font-mono text-[9px] text-brick font-bold mt-1 tracking-tighter">LOST_PKT</span>
            </div>
          </div>

          <div className="flex-1 flex flex-col justify-between h-full w-full gap-2.5">
            <div className="flex items-center justify-between pb-1.5 border-b border-line dark:border-edge">
              <span className="font-mono text-xs text-muted dark:text-fog uppercase">BEACON FREQUENCY</span>
              <span className="font-mono text-xs font-medium">1420.405 MHz (SILENT)</span>
            </div>
            <div className="flex items-center justify-between pb-1.5 border-b border-line dark:border-edge">
              <span className="font-mono text-xs text-muted dark:text-fog uppercase">INGRESS GATEWAY</span>
              <span className="font-mono text-xs font-medium">Apache Proxy / Nginx</span>
            </div>
            <div className="flex items-center justify-between pb-1.5 border-b border-line dark:border-edge">
              <span className="font-mono text-xs text-muted dark:text-fog uppercase">PROBE STATUS</span>
              <span className="font-mono text-xs text-brick font-medium flex items-center gap-1.5">
                <Activity className="w-3.5 h-3.5" />
                HOST_UNREACHABLE
              </span>
            </div>
          </div>
        </div>

        {/* Technical Incident Details Monospace Tray */}
        <div className="bg-paper dark:bg-abyss rounded-lg border border-line dark:border-edge p-3.5 flex flex-col gap-2">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Terminal className="w-4 h-4 text-muted dark:text-fog" />
              <span className="font-mono text-xs font-medium text-ink dark:text-bone">TRACE LOG // INGRESS_DISPATCH_FAILURE</span>
            </div>
          </div>
          <div className="bg-white dark:bg-panel border border-line dark:border-edge rounded p-3 font-mono text-[11px] text-ink dark:text-bone flex flex-col gap-1 overflow-x-auto shadow-inner">
            <div className="flex gap-2">
              <span className="text-muted/50 select-none">01</span>
              <span className="text-muted dark:text-fog">REQUEST_PATH:</span>
              <span className="text-accent dark:text-ember font-medium truncate">{location.pathname}</span>
            </div>
            <div className="flex gap-2">
              <span className="text-muted/50 select-none">02</span>
              <span className="text-muted dark:text-fog">DAEMON_STATUS:</span>
              <span className="text-brick font-medium">ERR_ROUTE_NOT_REGISTERED (0x7F)</span>
            </div>
            <div className="flex gap-2">
              <span className="text-muted/50 select-none">03</span>
              <span className="text-muted dark:text-fog">TIMESTAMP:</span>
              <span className="font-mono">{new Date().toISOString()}</span>
            </div>
          </div>
        </div>

        {/* Primary Control Actions */}
        <div className="flex flex-wrap items-center justify-between gap-3 pt-2">
          <Link to="/" className="inline-flex items-center gap-2 bg-accent hover:bg-accent-hover dark:bg-ember dark:hover:bg-ember-hover dark:text-ink text-white font-medium text-sm px-5 py-2.5 rounded-input shadow-sm transition-all duration-150 active:scale-[0.98]">
            <Route className="w-4 h-4" />
            <span>Return to Bridge</span>
          </Link>
          <a href="#" className="font-body text-sm text-muted hover:text-accent dark:text-fog dark:hover:text-ember transition-colors flex items-center gap-1.5 px-2 py-1">
            <span>Inspect Webhook Routing</span>
            <ExternalLink className="w-4 h-4" />
          </a>
        </div>
      </div>
    </div>
  );
}
