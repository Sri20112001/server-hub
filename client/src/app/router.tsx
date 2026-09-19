import { useEffect, useState } from "react";
import { createBrowserRouter, Navigate, RouterProvider } from "react-router";
import { AppShell } from "../components/chrome";
import { DashboardPage } from "../features/dashboard/DashboardPage";
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
      { path: "/login", element: <LoginPage /> },
      {
        element: <AppShell online={online} />,
        children: [
          {
            path: "/",
            element: (
              <Protected>
                <DashboardPage setOnline={setOnline} />
              </Protected>
            ),
          },
          {
            path: "/settings",
            element: (
              <Protected>
                <SettingsPage setOnline={setOnline} />
              </Protected>
            ),
          },
        ],
      },
      {
        path: "*",
        element: <NotFoundPage />
      },
    ],
    { basename: "/server-hub" },
  );
  return <RouterProvider router={router} />;
}
