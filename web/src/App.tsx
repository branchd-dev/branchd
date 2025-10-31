import { Routes, Route, Navigate } from "react-router";
import { Login } from "./pages/LoginPage";
import { SetupPage } from "./pages/SetupPage";
import { DashboardPage } from "./pages/DashboardPage";
import { SettingsPage } from "./pages/SettingsPage";
import { AnonRulesPage } from "./pages/AnonRulesPage";
import { UsersPage } from "./pages/UsersPage";
import { DashboardLayout } from "./DashboardLayout";
import { auth } from "@/lib/auth";

interface RouteGuardProps {
  children: React.ReactNode;
}

function ProtectedRoute({ children }: RouteGuardProps) {
  if (!auth.isAuthenticated()) {
    return <Navigate to="/login" replace />;
  }

  return <DashboardLayout>{children}</DashboardLayout>;
}

function PublicRoute({ children }: RouteGuardProps) {
  if (auth.isAuthenticated()) {
    return <Navigate to="/" replace />;
  }

  return <>{children}</>;
}

export function App() {
  return (
    <Routes>
      {/* Public Routes */}
      <Route
        path="/setup"
        element={
          <PublicRoute>
            <SetupPage />
          </PublicRoute>
        }
      />
      <Route
        path="/login"
        element={
          <PublicRoute>
            <Login />
          </PublicRoute>
        }
      />

      {/* Protected Routes */}
      <Route
        path="/"
        element={
          <ProtectedRoute>
            <DashboardPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/settings"
        element={
          <ProtectedRoute>
            <SettingsPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/anon-rules"
        element={
          <ProtectedRoute>
            <AnonRulesPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/users"
        element={
          <ProtectedRoute>
            <UsersPage />
          </ProtectedRoute>
        }
      />
    </Routes>
  );
}
