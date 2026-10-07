import { useEffect, useState } from "react";
import { createBrowserRouter, Navigate, RouterProvider } from "react-router";
import { AppShell } from "../components/chrome";
import { DashboardPage } from "../features/dashboard/DashboardPage";
import { FleetPage } from "../features/dashboard/FleetPage";
import { DatabasesPage } from "../features/dashboard/DatabasesPage";
import { ProjectDetailsPage } from "../features/dashboard/ProjectDetailsPage";
import { TelemetryPage } from "../features/dashboard/TelemetryPage";
import { ServersPage } from "../features/dashboard/ServersPage";
import { ServerDetailPage } from "../features/dashboard/ServerDetailPage";
import { AlertsPage } from "../features/dashboard/AlertsPage";
import { HealthChecksPage } from "../features/dashboard/HealthChecksPage";
import { MonitoringPage } from "../features/dashboard/MonitoringPage";
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
                path: "fleet",
                element: (
                  <Protected>
                    <FleetPage setOnline={setOnline} />
                  </Protected>
                ),
              },
              {
                path: "projects/:id",
                element: (
                  <Protected>
                    <ProjectDetailsPage setOnline={setOnline} />
                  </Protected>
                ),
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
