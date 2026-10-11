import { useEffect, useState } from "react";
import { createBrowserRouter, Navigate, RouterProvider } from "react-router";
import { AppShell } from "../components/chrome";
import { DashboardPage } from "../features/dashboard/DashboardPage";
import { DatabasesPage } from "../features/dashboard/DatabasesPage";
import { TelemetryPage } from "../features/dashboard/TelemetryPage";
import { ServersPage } from "../features/dashboard/ServersPage";
import { ServerDetailPage } from "../features/dashboard/ServerDetailPage";
import { AlertsPage } from "../features/dashboard/AlertsPage";
import { HealthChecksPage } from "../features/dashboard/HealthChecksPage";
import { MonitoringPage } from "../features/dashboard/MonitoringPage";
import { FileTransfersPage } from "../features/transfers/FileTransfersPage";
import { AuditLogPage } from "../features/dashboard/AuditLogPage";
import { SettingsPage } from "../features/settings/SettingsPage";
import { LoginPage } from "../features/auth/LoginPage";
import { NotFoundPage } from "../features/core/NotFoundPage";
import { useAuth } from "../stores/store";

function Protected({ children }: { children: React.ReactNode }) {
  const { user, checked, check } = useAuth();
  useEffect(() => {
    if (!checked) void check();
  }, [checked, check]);
  if (!checked) return null;
  if (!user) return <Navigate to="/login" replace />;
  return <>{children}</>;
}

export function AppRouter() {
  const [online, setOnline] = useState(true);
  const router = createBrowserRouter(
    [
      {
        path: "/",
        errorElement: <NotFoundPage />,
        children: [
          {
            path: "login",
            element: <LoginPage />,
          },
          {
            path: "",
            element: <AppShell online={online} />,
            children: [
              {
                index: true,
                element: (
                  <Protected>
                    <DashboardPage setOnline={setOnline} />
                  </Protected>
                ),
              },
              {
                // Projects section temporarily hidden from the UI (see
                // FleetPage / ProjectDetailsPage, preserved on disk with
                // their backend APIs intact). Direct visits bounce home
                // instead of landing on an unlinked page.
                path: "fleet",
                element: <Navigate to="/" replace />,
              },
              {
                path: "projects/:id",
                element: <Navigate to="/" replace />,
              },
              {
                path: "databases",
                element: (
                  <Protected>
                    <DatabasesPage setOnline={setOnline} />
                  </Protected>
                ),
              },
              {
                path: "settings",
                element: (
                  <Protected>
                    <SettingsPage setOnline={setOnline} />
                  </Protected>
                ),
              },
              {
                path: "telemetry",
                element: (
                  <Protected>
                    <TelemetryPage />
                  </Protected>
                ),
              },
              {
                path: "servers",
                element: (
                  <Protected>
                    <ServersPage setOnline={setOnline} />
                  </Protected>
                ),
              },
              {
                path: "servers/:id",
                element: (
                  <Protected>
                    <ServerDetailPage setOnline={setOnline} />
                  </Protected>
                ),
              },
              {
                path: "alerts",
                element: (
                  <Protected>
                    <AlertsPage />
                  </Protected>
                ),
              },
              {
                path: "health-checks",
                element: (
                  <Protected>
                    <HealthChecksPage />
                  </Protected>
                ),
              },
              {
                path: "audit",
                element: (
                  <Protected>
                    <AuditLogPage />
                  </Protected>
                ),
              },
              {
                path: "monitoring",
                element: (
                  <Protected>
                    <MonitoringPage />
                  </Protected>
                ),
              },
              {
                path: "transfers",
                element: (
                  <Protected>
                    <FileTransfersPage />
                  </Protected>
                ),
              },
            ],
          },
          {
            path: "*",
            element: <NotFoundPage />,
          },
        ],
      },
    ],
    { basename: "/server-hub" },
  );
  return <RouterProvider router={router} />;
}
