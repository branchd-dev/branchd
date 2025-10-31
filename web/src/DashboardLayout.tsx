import { useState, useEffect } from "react";
import { Button } from "./shadcn/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "./shadcn/components/ui/dropdown-menu";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "./shadcn/components/ui/alert-dialog";
import { Link } from "react-router";
import { ThemeToggle } from "./components/theme-toggle";
import { auth } from "./lib/auth";
import { useApi } from "./hooks/use-api";

interface DashboardLayoutProps {
  children: React.ReactNode;
}

interface User {
  id: string;
  email: string;
  name: string;
  is_admin: boolean;
}

export function DashboardLayout({ children }: DashboardLayoutProps) {
  const api = useApi();
  const [currentUser, setCurrentUser] = useState<User | null>(null);
  const [currentVersion, setCurrentVersion] = useState<string>("");
  const [latestVersion, setLatestVersion] = useState<string>("");
  const [updateAvailable, setUpdateAvailable] = useState<boolean>(false);
  const [showUpdateDialog, setShowUpdateDialog] = useState<boolean>(false);
  const [isUpdating, setIsUpdating] = useState<boolean>(false);

  useEffect(() => {
    // Fetch current user info to determine admin status
    const fetchCurrentUser = async () => {
      try {
        const response = await api.api.authMeList();
        setCurrentUser(response.data as User);
      } catch (err) {
        console.error("Failed to fetch current user:", err);
      }
    };

    fetchCurrentUser();
  }, [api.api]);

  useEffect(() => {
    // Check for updates
    const checkForUpdates = async () => {
      try {
        const response = await api.api.systemLatestVersionList();
        const data = response.data;
        setCurrentVersion(data.current_version || "dev");
        setLatestVersion(data.latest_version || "");
        setUpdateAvailable(data.update_available || false);
      } catch (err) {
        console.error("Failed to check for updates:", err);
      }
    };

    checkForUpdates();
    // Check for updates every 5 minutes
    const interval = setInterval(checkForUpdates, 5 * 60 * 1000);
    return () => clearInterval(interval);
  }, [api.api]);

  const handleLogout = () => {
    auth.logout();
  };

  const handleUpdateClick = () => {
    if (updateAvailable) {
      setShowUpdateDialog(true);
    }
  };

  const handleConfirmUpdate = async () => {
    setIsUpdating(true);
    const targetVersion = latestVersion;

    try {
      await api.api.systemUpdateCreate();

      // Poll for version change every 2 seconds
      const pollInterval = setInterval(async () => {
        try {
          const response = await api.api.systemLatestVersionList();
          const currentVer = response.data.current_version;

          // Check if version changed to target version
          if (currentVer === targetVersion) {
            clearInterval(pollInterval);
            setShowUpdateDialog(false);

            // Reload page to get new frontend code
            window.location.reload();
          }
        } catch (err) {
          // Server might be restarting, continue polling
          console.log("Polling for version update...");
        }
      }, 2000);

      // Safety timeout: stop polling after 2 minutes
      setTimeout(() => {
        clearInterval(pollInterval);
        setIsUpdating(false);
        alert("Update may have completed. Please refresh the page manually.");
      }, 120000);
    } catch (err) {
      console.error("Failed to trigger update:", err);
      alert("Failed to trigger update. Please try again.");
      setIsUpdating(false);
      setShowUpdateDialog(false);
    }
  };

  return (
    <div className="min-h-screen bg-background">
      {/* Header */}
      <header className="border-b">
        <div className="flex items-center justify-between px-6 py-3">
          <div className="flex items-center space-x-6">
            <div className="text-xl font-semibold">branchd</div>
            <nav className="flex space-x-6">
              <Button variant="ghost" size="sm" asChild>
                <Link to="/">Dashboard</Link>
              </Button>
              <Button variant="ghost" size="sm" asChild>
                <Link to="/settings">Settings</Link>
              </Button>
              <Button variant="ghost" size="sm" asChild>
                <Link to="/anon-rules">Anonymization Rules</Link>
              </Button>
              {currentUser?.is_admin && (
                <Button variant="ghost" size="sm" asChild>
                  <Link to="/users">Users</Link>
                </Button>
              )}
            </nav>
          </div>

          <div className="flex items-center gap-2">
            {/* Version button with update indicator */}
            {currentVersion && (
              <Button
                variant={updateAvailable ? "default" : "ghost"}
                size="sm"
                onClick={handleUpdateClick}
                disabled={!updateAvailable}
                className={updateAvailable ? "animate-pulse" : ""}
              >
                {updateAvailable
                  ? `Update to ${latestVersion}`
                  : currentVersion}
              </Button>
            )}
            <Button variant="ghost" size="sm" asChild>
              <a
                href="mailto:rafael@branchd.dev?subject=Branchd%20Support"
                target="_blank"
                rel="noopener noreferrer"
              >
                Need Support?
              </a>
            </Button>
            <ThemeToggle />
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="sm">
                  {currentUser?.email || "Account"}
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onClick={handleLogout}>
                  Log out
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>
      </header>

      {/* Update confirmation dialog */}
      <AlertDialog open={showUpdateDialog} onOpenChange={setShowUpdateDialog}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Update Branchd Server?</AlertDialogTitle>
            <AlertDialogDescription>
              {!isUpdating ? (
                <>
                  This will update your server from{" "}
                  <strong>{currentVersion}</strong> to{" "}
                  <strong>{latestVersion}</strong>.
                  <br />
                  <br />
                  The server will be briefly unavailable during the update.
                  This usually takes just a few seconds.
                </>
              ) : (
                <>
                  <strong>Update in progress...</strong>
                  <br />
                  <br />
                  The server is being updated to{" "}
                  <strong>{latestVersion}</strong>. The page will automatically
                  reload when the update is complete.
                  <br />
                  <br />
                  This usually takes just a few seconds.
                </>
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isUpdating}>
              {isUpdating ? "Please wait..." : "Cancel"}
            </AlertDialogCancel>
            {!isUpdating && (
              <Button
                onClick={(e) => {
                  e.preventDefault();
                  handleConfirmUpdate();
                }}
              >
                Update Now
              </Button>
            )}
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Main Content */}
      <main className="p-6">{children}</main>
    </div>
  );
}
