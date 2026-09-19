import { useEffect, useState } from "react";
import { createBrowserRouter, Navigate, RouterProvider } from "react-router";
import { AppShell } from "../components/chrome";
import { DashboardPage } from "../features/dashboard/DashboardPage";
import { SettingsPage } from "../features/settings/SettingsPage";
import { LoginPage } from "../features/auth/LoginPage";
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
        element: (
          <div className="flex flex-col items-center justify-center min-h-screen bg-paper dark:bg-abyss text-ink dark:text-bone">
            <h1 className="text-4xl font-bold mb-4">404 - Not Found</h1>
            <p className="text-muted dark:text-fog mb-6">The page you are looking for does not exist.</p>
            <a href="/server-hub/" className="px-4 py-2 bg-accent text-white rounded-input">Go Home</a>
            <p className="mt-8 text-xs text-muted">React Application Route (If you see this, Nginx/Apache is working!)</p>
          </div>
        )
      },
    ],
    { basename: "/server-hub" },
  );
  return <RouterProvider router={router} />;
}
