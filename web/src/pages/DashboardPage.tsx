import { useEffect, useState } from "react";
import { useApi } from "../hooks/use-api";
import type {
  GithubComBranchdDevBranchdInternalModelsRestore,
  GithubComBranchdDevBranchdInternalModelsAnonRule,
  InternalServerConfigResponse,
  InternalServerCreateBranchResponse,
  InternalServerBranchListResponse,
  InternalServerSystemInfoResponse,
} from "../lib/openapi";
import { Button } from "../shadcn/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "../shadcn/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "../shadcn/components/ui/table";
import { Input } from "../shadcn/components/ui/input";
import { Label } from "../shadcn/components/ui/label";
import { Alert, AlertDescription } from "../shadcn/components/ui/alert";
import {
  Loader2,
  Trash2,
  GitBranch,
  Database,
  Cpu,
  HardDrive,
  Activity,
  Server,
  Settings2,
  ShieldAlert,
  Globe,
  Clock,
  RotateCw,
} from "lucide-react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "../shadcn/components/ui/card";
import { Badge } from "../shadcn/components/ui/badge";

export function DashboardPage() {
  const api = useApi();
  const [config, setConfig] = useState<InternalServerConfigResponse | null>(
    null,
  );
  const [systemInfo, setSystemInfo] =
    useState<InternalServerSystemInfoResponse | null>(null);
  const [restores, setRestores] = useState<
    GithubComBranchdDevBranchdInternalModelsRestore[]
  >([]);
  const [branches, setBranches] = useState<InternalServerBranchListResponse[]>(
    [],
  );
  const [anonRules, setAnonRules] = useState<
    GithubComBranchdDevBranchdInternalModelsAnonRule[]
  >([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Create branch dialog state
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [branchName, setBranchName] = useState("");
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);
  const [createdBranch, setCreatedBranch] =
    useState<InternalServerCreateBranchResponse | null>(null);

  // Delete branch dialog state
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [branchToDelete, setBranchToDelete] =
    useState<InternalServerBranchListResponse | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  // Trigger restore state
  const [triggering, setTriggering] = useState(false);
  const [triggerError, setTriggerError] = useState<string | null>(null);

  // Delete restore dialog state
  const [deleteRestoreDialogOpen, setDeleteRestoreDialogOpen] = useState(false);
  const [restoreToDelete, setRestoreToDelete] =
    useState<GithubComBranchdDevBranchdInternalModelsRestore | null>(null);
  const [deletingRestore, setDeletingRestore] = useState(false);
  const [deleteRestoreError, setDeleteRestoreError] = useState<string | null>(
    null,
  );

  const fetchData = async () => {
    try {
      setLoading(true);
      setError(null);

      // Fetch config first
      const configResponse = await api.api.configList();
      const configData = await configResponse.json();

      // If no config found, show error (user needs to complete setup first)
      if (!configData || configResponse.status === 404) {
        setError("Configuration not found. Please complete setup first.");
        setLoading(false);
        return;
      }

      setConfig(configData);

      // Fetch system info, restores, branches, and anon rules
      const [
        systemInfoResponse,
        restoresResponse,
        branchesResponse,
        anonRulesResponse,
      ] = await Promise.all([
        api.api.systemInfoList(),
        api.api.restoresList(),
        api.api.branchesList(),
        api.api.anonRulesList(),
      ]);

      const systemInfoData = await systemInfoResponse.json();
      const restoresData = await restoresResponse.json();
      const branchesData = await branchesResponse.json();
      const anonRulesData = await anonRulesResponse.json();

      setSystemInfo(systemInfoData || null);
      setRestores(restoresData || []);
      setBranches(branchesData || []);
      setAnonRules(anonRulesData || []);
    } catch (err: any) {
      if (err.status === 404) {
        setError("Configuration not found. Please complete setup first.");
      } else {
        setError(err.message || "Failed to load data");
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
    // Poll every 5 seconds for updates
    const interval = setInterval(fetchData, 5000);
    return () => clearInterval(interval);
  }, [api]);

  const handleCreateBranch = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!branchName) return;

    setCreating(true);
    setCreateError(null);

    try {
      const response = await api.api.branchesCreate({
        name: branchName,
      });

      const result = await response.json();
      setCreatedBranch(result);

      // Refresh the list
      await fetchData();

      // Close dialog after short delay
      setTimeout(() => {
        setCreateDialogOpen(false);
        setBranchName("");
        setCreatedBranch(null);
      }, 2000);
    } catch (err: any) {
      setCreateError(
        err.error?.error || err.message || "Failed to create branch",
      );
    } finally {
      setCreating(false);
    }
  };

  const handleDeleteBranch = async () => {
    if (!branchToDelete) return;

    setDeleting(true);
    setDeleteError(null);

    try {
      await api.api.branchesIdDelete(branchToDelete.id!);

      // Refresh the list
      await fetchData();

      setDeleteDialogOpen(false);
      setBranchToDelete(null);
    } catch (err: any) {
      setDeleteError(
        err.error?.error || err.message || "Failed to delete branch",
      );
    } finally {
      setDeleting(false);
    }
  };

  const handleTriggerRestore = async () => {
    setTriggering(true);
    setTriggerError(null);

    try {
      await api.api.restoresTriggerRestoreCreate();

      // Refresh to see the new database
      await fetchData();
      setTriggerError(null);
    } catch (err: any) {
      setTriggerError(
        err.error?.error || err.message || "Failed to trigger restore",
      );
    } finally {
      setTriggering(false);
    }
  };

  const handleDeleteRestore = async () => {
    if (!restoreToDelete) return;

    setDeletingRestore(true);
    setDeleteRestoreError(null);

    try {
      await api.api.restoresDelete(restoreToDelete.id!);

      // Refresh the list
      await fetchData();

      setDeleteRestoreDialogOpen(false);
      setRestoreToDelete(null);
    } catch (err: any) {
      setDeleteRestoreError(
        err.error?.error || err.message || "Failed to delete restore",
      );
    } finally {
      setDeletingRestore(false);
    }
  };

  if (loading && !config) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="h-8 w-8 animate-spin text-gray-400" />
      </div>
    );
  }

  if (error) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{error}</AlertDescription>
      </Alert>
    );
  }

  if (!config) {
    return null;
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-3xl font-bold">Dashboard</h1>
        <p className="text-gray-500 mt-1">
          Monitor your database branching system
        </p>
      </div>

      {/* System Overview */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        {/* VM Metrics */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Server className="h-5 w-5" />
              Virtual Machine
            </CardTitle>
            <CardDescription>Resource utilization and capacity</CardDescription>
          </CardHeader>
          <CardContent>
            {systemInfo ? (
              <div className="space-y-4">
                <div className="flex items-center gap-3">
                  <Cpu className="h-5 w-5" />
                  <div className="flex-1">
                    <p className="text-sm text-gray-500">CPU Cores</p>
                    <p className="text-2xl font-bold">
                      {systemInfo.vm?.cpu_count || 0}
                    </p>
                  </div>
                </div>
                <div className="flex items-center gap-3">
                  <Activity className="h-5 w-5" />
                  <div className="flex-1">
                    <p className="text-sm text-gray-500">Memory</p>
                    <div className="flex items-baseline gap-2">
                      <p className="text-2xl font-bold">
                        {systemInfo.vm?.memory_used_gb?.toFixed(1) || "0.0"}
                      </p>
                      <p className="text-sm text-gray-500">
                        / {systemInfo.vm?.memory_total_gb?.toFixed(1) || "0.0"}{" "}
                        GB
                      </p>
                    </div>
                    <div className="mt-2 w-full bg-gray-200 rounded-full h-2">
                      <div
                        className="bg-gray-900 h-2 rounded-full"
                        style={{
                          width: `${
                            systemInfo.vm?.memory_total_gb &&
                            systemInfo.vm?.memory_used_gb
                              ? (systemInfo.vm.memory_used_gb /
                                  systemInfo.vm.memory_total_gb) *
                                100
                              : 0
                          }%`,
                        }}
                      />
                    </div>
                  </div>
                </div>
                <div className="flex items-center gap-3">
                  <HardDrive className="h-5 w-5" />
                  <div className="flex-1">
                    <p className="text-sm text-gray-500">Storage</p>
                    <div className="flex items-baseline gap-2">
                      <p className="text-2xl font-bold">
                        {systemInfo.vm?.disk_used_gb?.toFixed(1) || "0.0"}
                      </p>
                      <p className="text-sm text-gray-500">
                        / {systemInfo.vm?.disk_total_gb?.toFixed(1) || "0.0"} GB
                      </p>
                    </div>
                    <div className="mt-2 w-full bg-gray-200 rounded-full h-2">
                      <div
                        className="bg-gray-900 h-2 rounded-full"
                        style={{
                          width: `${systemInfo.vm?.disk_used_percent?.toFixed(0) || 0}%`,
                        }}
                      />
                    </div>
                  </div>
                </div>
              </div>
            ) : (
              <div className="flex items-center justify-center py-8">
                <Loader2 className="h-6 w-6 animate-spin text-gray-400" />
              </div>
            )}
          </CardContent>
        </Card>

        {/* Branching Configuration */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Settings2 className="h-5 w-5" />
              Branching Configuration
            </CardTitle>
            <CardDescription>
              Current settings for database branching
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              {/* Database Name */}
              <div>
                <p className="text-sm text-gray-500 mb-1">Source Database</p>
                <div className="flex items-center gap-3">
                  <Database className="h-5 w-5" />
                  <div>
                    <p className="text-lg font-semibold">
                      {systemInfo?.source_database?.name ||
                        config.database_name ||
                        "Not configured"}
                    </p>
                    <p className="text-sm text-gray-500">
                      PostgreSQL{" "}
                      {systemInfo?.source_database?.version?.replace(
                        "PostgreSQL ",
                        "",
                      ) || config.postgres_version || "16"}
                      {systemInfo?.source_database?.size_gb && (
                        <> · {systemInfo.source_database.size_gb.toFixed(2)} GB</>
                      )}
                      {systemInfo?.source_database?.connected !== undefined && (
                        <>
                          {" · "}
                          <span
                            className={
                              systemInfo.source_database.connected
                                ? "text-green-600"
                                : "text-red-600"
                            }
                          >
                            {systemInfo.source_database.connected
                              ? "Connected"
                              : "Disconnected"}
                          </span>
                        </>
                      )}
                    </p>
                  </div>
                </div>
              </div>

              {/* Configuration Grid */}
              <div className="border-t pt-4">
                <div className="grid grid-cols-2 gap-4">
                  {/* Restore Mode */}
                  <div className="flex items-start gap-3">
                    <Database className="h-5 w-5 mt-0.5" />
                    <div>
                      <p className="text-sm font-medium">Restore Mode</p>
                      <p className="text-sm text-gray-600">
                        {config.schema_only ? "Schema only" : "Schema + data"}
                      </p>
                    </div>
                  </div>

                  {/* Refresh Schedule */}
                  <div className="flex items-start gap-3">
                    <Clock className="h-5 w-5 mt-0.5" />
                    <div>
                      <p className="text-sm font-medium">Refresh Schedule</p>
                      <p className="text-sm text-gray-600 font-mono">
                        {config.refresh_schedule || "Not configured"}
                      </p>
                    </div>
                  </div>

                  {/* Max Restores */}
                  <div className="flex items-start gap-3">
                    <RotateCw className="h-5 w-5 mt-0.5" />
                    <div>
                      <p className="text-sm font-medium">Maximum Restores</p>
                      <p className="text-sm text-gray-600">
                        {config.max_restores || 5} restores
                      </p>
                    </div>
                  </div>

                  {/* Anonymization Rules */}
                  <div className="flex items-start gap-3">
                    <ShieldAlert className="h-5 w-5 mt-0.5" />
                    <div>
                      <p className="text-sm font-medium">Anonymization Rules</p>
                      <p className="text-sm text-gray-600">
                        {anonRules.length === 0
                          ? "None configured"
                          : `${anonRules.length} rule${anonRules.length === 1 ? "" : "s"}`}
                      </p>
                    </div>
                  </div>

                  {/* Let's Encrypt Domain (if configured) */}
                  {config.domain && (
                    <div className="flex items-start gap-3 col-span-2">
                      <Globe className="h-5 w-5 mt-0.5" />
                      <div>
                        <p className="text-sm font-medium">Custom Domain</p>
                        <p className="text-sm text-gray-600 font-mono">
                          {config.domain}
                        </p>
                      </div>
                    </div>
                  )}
                </div>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Restores Card */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Restores</CardTitle>
              <CardDescription>
                Restored database snapshots available for branching
              </CardDescription>
            </div>
            <Button onClick={handleTriggerRestore} disabled={triggering}>
              {triggering && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
              Trigger Restore
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {triggerError && (
            <Alert variant="destructive" className="mb-4">
              <AlertDescription>{triggerError}</AlertDescription>
            </Alert>
          )}
          {restores.length === 0 ? (
            <div className="text-center py-8 text-gray-500">
              <Database className="h-12 w-12 mx-auto mb-2 opacity-50" />
              <p>No restores yet</p>
              {loading && (
                <Loader2 className="h-6 w-6 animate-spin mx-auto mt-2" />
              )}
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Type</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Started at</TableHead>
                  <TableHead>Ready at</TableHead>
                  <TableHead className="w-[100px]">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {restores.map((restore) => (
                  <TableRow key={restore.id}>
                    <TableCell className="font-mono text-sm">
                      {restore.name}
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={restore.schema_only ? "secondary" : "default"}
                      >
                        {restore.schema_only
                          ? "Schema-only"
                          : "Schema and data"}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      {restore.schema_ready ? (
                        <Badge variant="default" className="bg-green-600">
                          Ready
                        </Badge>
                      ) : (
                        <Badge
                          variant="secondary"
                          className="flex items-center gap-1"
                        >
                          <Loader2 className="h-3 w-3 animate-spin" />
                          Restoring
                        </Badge>
                      )}
                    </TableCell>
                    <TableCell>
                      {restore.created_at &&
                        new Date(restore.created_at).toLocaleString()}
                    </TableCell>
                    <TableCell>
                      {restore.ready_at
                        ? new Date(restore.ready_at).toLocaleString()
                        : "-"}
                    </TableCell>
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => {
                          setRestoreToDelete(restore);
                          setDeleteRestoreDialogOpen(true);
                        }}
                      >
                        <Trash2 className="h-4 w-4 text-red-600" />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      {/* Branches Card */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="flex items-center gap-2">
                <GitBranch className="h-5 w-5" />
                Branches
              </CardTitle>
              <CardDescription>
                Create isolated database branches for development
              </CardDescription>
            </div>
            <Button onClick={() => setCreateDialogOpen(true)}>
              <GitBranch className="h-4 w-4 mr-2" />
              Create Branch
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {branches.length === 0 ? (
            <div className="text-center py-8 text-gray-500">
              <GitBranch className="h-12 w-12 mx-auto mb-2 opacity-50" />
              <p>No branches yet</p>
              <p className="text-sm mt-1">
                Create your first branch to get started
              </p>
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Restore</TableHead>
                  <TableHead>Connection String</TableHead>
                  <TableHead>Created</TableHead>
                  <TableHead className="w-[100px]">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {branches.map((branch) => (
                  <TableRow key={branch.id}>
                    <TableCell className="font-semibold">
                      {branch.name}
                    </TableCell>
                    <TableCell className="font-mono text-xs">
                      {branch.restore_name || "N/A"}
                    </TableCell>
                    <TableCell className="font-mono text-xs max-w-md truncate">
                      {branch.connection_url || "N/A"}
                    </TableCell>
                    <TableCell>
                      {branch.created_at &&
                        new Date(branch.created_at).toLocaleString()}
                    </TableCell>
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => {
                          setBranchToDelete(branch);
                          setDeleteDialogOpen(true);
                        }}
                      >
                        <Trash2 className="h-4 w-4 text-red-600" />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      {/* Create Branch Dialog */}
      <Dialog open={createDialogOpen} onOpenChange={setCreateDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create New Branch</DialogTitle>
            <DialogDescription>
              Create an isolated database branch for development or testing
            </DialogDescription>
          </DialogHeader>

          {createdBranch ? (
            <div className="space-y-4">
              <Alert>
                <AlertDescription>
                  Branch created successfully! Connection details:
                </AlertDescription>
              </Alert>

              <div className="space-y-2">
                <Label>Connection String</Label>
                <Input
                  value={
                    createdBranch.user &&
                    createdBranch.password &&
                    createdBranch.database
                      ? `postgresql://${createdBranch.user}:${createdBranch.password}@${createdBranch.host || "localhost"}:${createdBranch.port}/${createdBranch.database}`
                      : ""
                  }
                  readOnly
                  className="font-mono text-xs"
                />
              </div>
            </div>
          ) : (
            <form onSubmit={handleCreateBranch} className="space-y-4">
              {createError && (
                <Alert variant="destructive">
                  <AlertDescription>{createError}</AlertDescription>
                </Alert>
              )}

              <div className="space-y-2">
                <Label htmlFor="branchName">Branch Name</Label>
                <Input
                  id="branchName"
                  placeholder="my-feature-branch"
                  value={branchName}
                  onChange={(e) => setBranchName(e.target.value)}
                  required
                  disabled={creating}
                />
              </div>

              <DialogFooter>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setCreateDialogOpen(false)}
                  disabled={creating}
                >
                  Cancel
                </Button>
                <Button type="submit" disabled={creating}>
                  {creating && (
                    <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                  )}
                  Create
                </Button>
              </DialogFooter>
            </form>
          )}
        </DialogContent>
      </Dialog>

      {/* Delete Branch Dialog */}
      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Branch</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete the branch "{branchToDelete?.name}
              "? This action cannot be undone.
            </DialogDescription>
          </DialogHeader>

          {deleteError && (
            <Alert variant="destructive">
              <AlertDescription>{deleteError}</AlertDescription>
            </Alert>
          )}

          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setDeleteDialogOpen(false)}
              disabled={deleting}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDeleteBranch}
              disabled={deleting}
            >
              {deleting && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Restore Dialog */}
      <Dialog
        open={deleteRestoreDialogOpen}
        onOpenChange={setDeleteRestoreDialogOpen}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Restore</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete the restore "
              {restoreToDelete?.name}
              "? This action cannot be undone and will fail if there are active
              branches.
            </DialogDescription>
          </DialogHeader>

          {deleteRestoreError && (
            <Alert variant="destructive">
              <AlertDescription>{deleteRestoreError}</AlertDescription>
            </Alert>
          )}

          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setDeleteRestoreDialogOpen(false)}
              disabled={deletingRestore}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDeleteRestore}
              disabled={deletingRestore}
            >
              {deletingRestore && (
                <Loader2 className="h-4 w-4 mr-2 animate-spin" />
              )}
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
