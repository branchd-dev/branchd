import { useEffect, useState } from "react";
import { useApi } from "../hooks/use-api";
import type {
  InternalServerConfigResponse,
  InternalServerSystemInfoResponse,
} from "../lib/openapi";
import { Button } from "../shadcn/components/ui/button";
import { Input } from "../shadcn/components/ui/input";
import { Label } from "../shadcn/components/ui/label";
import { Alert, AlertDescription } from "../shadcn/components/ui/alert";
import { Progress } from "../shadcn/components/ui/progress";
import { Loader2, ArrowLeft, Info, TriangleAlert } from "lucide-react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../shadcn/components/ui/select";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "../shadcn/components/ui/card";
import { useNavigate } from "react-router";

export function SettingsPage() {
  const api = useApi();
  const navigate = useNavigate();
  const [config, setConfig] = useState<InternalServerConfigResponse | null>(
    null,
  );
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Config form state
  const [connectionString, setConnectionString] = useState("");
  const [originalConnectionString, setOriginalConnectionString] = useState(""); // Track original redacted value
  const [postgresVersion, setPostgresVersion] = useState("16");
  const [schemaOnly, setSchemaOnly] = useState<"schema" | "full">("schema");
  const [refreshSchedule, setRefreshSchedule] = useState("");
  const [maxRestores, setMaxRestores] = useState(1);
  const [domain, setDomain] = useState("");
  const [letsEncryptEmail, setLetsEncryptEmail] = useState("");
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saveSuccess, setSaveSuccess] = useState(false);

  // System info state
  const [systemInfo, setSystemInfo] =
    useState<InternalServerSystemInfoResponse | null>(null);

  const fetchData = async () => {
    try {
      setLoading(true);
      setError(null);

      const configResponse = await api.api.configList();
      const configData = await configResponse.json();

      if (!configData || configResponse.status === 404) {
        setError("Configuration not found. Please complete setup first.");
        setLoading(false);
        return;
      }

      setConfig(configData);
      const redactedConnStr = configData.connection_string || "";
      setConnectionString(redactedConnStr);
      setOriginalConnectionString(redactedConnStr); // Save original redacted value
      setPostgresVersion(configData.postgres_version || "16");
      setSchemaOnly(configData.schema_only ? "schema" : "full");
      setRefreshSchedule(configData.refresh_schedule || "");
      setMaxRestores(configData.max_restores || 1);
      setDomain(configData.domain || "");
      setLetsEncryptEmail(configData.lets_encrypt_email || "");

      // Fetch system info
      try {
        const systemInfoResponse = await api.api.systemInfoList();
        const systemInfoData = await systemInfoResponse.json();
        setSystemInfo(systemInfoData);
      } catch (systemErr) {
        // Don't fail the whole page if system info fails
        console.error("Failed to fetch system info:", systemErr);
      }
    } catch (err: any) {
      if (err.status === 404) {
        setError("Configuration not found. Please complete setup first.");
      } else {
        setError(err.message || "Failed to load configuration");
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, [api]);

  const handleSaveConfig = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setSaveError(null);
    setSaveSuccess(false);

    try {
      // Validate: if domain is set, email must be set too
      if (domain && !letsEncryptEmail) {
        setSaveError("Email is required when using a custom domain");
        setSaving(false);
        return;
      }

      // Only send connection string if it was actually modified (not the redacted version)
      const connectionStringChanged =
        connectionString !== originalConnectionString;

      await api.api.configPartialUpdate({
        connectionString: connectionStringChanged
          ? connectionString
          : undefined,
        postgresVersion: postgresVersion || undefined,
        schemaOnly: schemaOnly === "schema",
        refreshSchedule: refreshSchedule, // Send empty string to clear
        maxRestores: maxRestores,
        domain: domain || undefined,
        letsEncryptEmail: letsEncryptEmail || undefined,
      });

      // Refresh to get updated config
      await fetchData();
      setSaveError(null);
      setSaveSuccess(true);

      // Clear success message after 3 seconds
      setTimeout(() => {
        setSaveSuccess(false);
      }, 3000);
    } catch (err: any) {
      setSaveError(
        err.error?.error || err.message || "Failed to save configuration",
      );
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
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
    <div className="max-w-4xl mx-auto space-y-6 p-6">
      {/* Header */}
      <div className="flex items-center gap-4">
        <Button variant="ghost" onClick={() => navigate("/")} className="-ml-2">
          <ArrowLeft className="h-4 w-4 mr-2" />
          Back to Dashboard
        </Button>
      </div>

      <div>
        <h1 className="text-3xl font-bold tracking-tight">Configuration</h1>
        <p className="text-gray-500 mt-1">
          Manage your database source and branching settings
        </p>
      </div>

      <form onSubmit={handleSaveConfig} className="space-y-6">
        {/* Alerts */}
        {saveSuccess && (
          <Alert className="bg-green-50 border-green-200">
            <AlertDescription className="text-green-800">
              Configuration saved successfully!
            </AlertDescription>
          </Alert>
        )}
        {saveError && (
          <Alert variant="destructive">
            <AlertDescription>{saveError}</AlertDescription>
          </Alert>
        )}

        {/* Source Database Section */}
        <Card>
          <CardHeader>
            <CardTitle>Source Database</CardTitle>
            <CardDescription>
              Connection details for your production PostgreSQL database
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <Alert className="bg-blue-50 border-blue-200 dark:bg-blue-950/50 dark:border-blue-800">
              <Info className="h-4 w-4 dark:text-blue-400" />
              <AlertDescription className="text-sm dark:text-blue-200">
                Your connection string is stored in the VM's SQLite and is
                secured by the EBS volume encryption.
              </AlertDescription>
            </Alert>

            {/* Display detected database info */}
            {systemInfo?.source_database?.connected && (
              <div className="bg-gray-50 border border-gray-200 rounded-lg p-3 space-y-1 dark:bg-gray-900 dark:border-gray-700">
                <p className="text-sm dark:text-gray-300">
                  <span className="font-medium dark:text-gray-200">Detected version:</span>{" "}
                  {systemInfo.source_database.version || "Unknown"}
                </p>
                <p className="text-sm dark:text-gray-300">
                  <span className="font-medium dark:text-gray-200">Detected database size:</span>{" "}
                  {systemInfo.source_database.size_gb?.toFixed(2) || "0"} GB
                </p>
              </div>
            )}

            <div className="space-y-2">
              <Label htmlFor="connectionString">
                PostgreSQL Connection String
              </Label>
              <Input
                id="connectionString"
                type="text"
                placeholder="postgresql://user:password@host:5432/database"
                value={connectionString}
                onChange={(e) => setConnectionString(e.target.value)}
                disabled={saving}
                className="font-mono text-sm"
              />
              <p className="text-xs text-gray-500">
                {config.connection_string ? (
                  <>Leave unchanged to keep current connection.</>
                ) : (
                  "Example: postgresql://myuser:mypass@db.example.com:5432/mydb"
                )}
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="postgresVersion">PostgreSQL Version</Label>
              <Select
                value={postgresVersion}
                onValueChange={setPostgresVersion}
                disabled={saving}
              >
                <SelectTrigger id="postgresVersion">
                  <SelectValue placeholder="Select version" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="14">PostgreSQL 14</SelectItem>
                  <SelectItem value="15">PostgreSQL 15</SelectItem>
                  <SelectItem value="16">PostgreSQL 16</SelectItem>
                  <SelectItem value="17">PostgreSQL 17</SelectItem>
                </SelectContent>
              </Select>
              <p className="text-xs text-gray-500">
                Must match your source database version
              </p>
            </div>
          </CardContent>
        </Card>

        {/* Branching Configuration Section */}
        <Card>
          <CardHeader>
            <CardTitle>Branching Configuration</CardTitle>
            <CardDescription>
              Control how database branches are created and maintained
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-6">
            <div className="space-y-2">
              <Label htmlFor="schemaOnly">Restore Mode</Label>
              <Select
                value={schemaOnly}
                onValueChange={(value: "schema" | "full") =>
                  setSchemaOnly(value)
                }
                disabled={saving}
              >
                <SelectTrigger id="schemaOnly">
                  <SelectValue placeholder="Select restore mode" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="schema">Schema only</SelectItem>
                  <SelectItem value="full">Schema and data</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="border-t pt-4 space-y-4">
              <div>
                <Label htmlFor="refreshSchedule" className="text-base">
                  Automatic Refresh Schedule
                </Label>
                <p className="text-sm text-gray-500 mt-1 mb-3">
                  Periodically sync your database for fresh branching
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="refreshSchedule" className="text-sm">
                  Cron Expression
                </Label>
                <Input
                  id="refreshSchedule"
                  placeholder="0 2 * * *"
                  value={refreshSchedule}
                  onChange={(e) => setRefreshSchedule(e.target.value)}
                  className="font-mono text-sm"
                  disabled={saving}
                />
                <p className="text-xs text-gray-500">
                  Leave empty to disable automatic refresh. Need help with cron?
                  Visit{" "}
                  <a
                    href="https://crontab.guru"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-blue-600 hover:underline"
                  >
                    https://crontab.guru
                  </a>
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="maxRestores">Maximum Restores to Keep</Label>
                <Input
                  id="maxRestores"
                  type="number"
                  min="1"
                  value={maxRestores}
                  onChange={(e) =>
                    setMaxRestores(Math.max(1, parseInt(e.target.value) || 1))
                  }
                  disabled={saving}
                />
                {/* Dynamic storage calculation with progress bar */}
                {systemInfo?.source_database?.size_gb &&
                  systemInfo?.vm?.disk_available_gb &&
                  (() => {
                    const requiredGB =
                      systemInfo.source_database.size_gb * maxRestores;
                    const availableGB = systemInfo.vm.disk_available_gb;
                    const usagePercent = (requiredGB / availableGB) * 100;
                    const isWarning = usagePercent > 80;
                    const isDanger = usagePercent > 100;

                    return (
                      <div className="space-y-3 mt-3 p-3 bg-gray-50 border border-gray-200 rounded-lg dark:bg-gray-900 dark:border-gray-700">
                        <div className="space-y-2">
                          <div className="flex justify-between items-center text-sm">
                            <span className="font-medium text-gray-700 dark:text-gray-300">
                              Estimated Storage Usage
                            </span>
                            <span
                              className={`font-semibold ${
                                isDanger
                                  ? "text-red-600 dark:text-red-400"
                                  : isWarning
                                    ? "text-yellow-600 dark:text-yellow-400"
                                    : "text-green-600 dark:text-green-400"
                              }`}
                            >
                              {requiredGB.toFixed(2)} / {availableGB.toFixed(2)}{" "}
                              GB
                            </span>
                          </div>
                          <Progress
                            value={Math.min(usagePercent, 100)}
                            className={`h-3 ${
                              isDanger
                                ? "[&>div]:bg-red-500"
                                : isWarning
                                  ? "[&>div]:bg-yellow-500"
                                  : "[&>div]:bg-green-500"
                            }`}
                          />
                          <p className="text-xs text-gray-500 dark:text-gray-400">
                            {systemInfo.source_database.size_gb.toFixed(2)} GB
                            per restore × {maxRestores} restore
                            {maxRestores === 1 ? "" : "s"}
                          </p>
                        </div>

                        {isDanger && (
                          <Alert className="py-2">
                            <TriangleAlert className="h-4 w-4" />
                            <AlertDescription className="text-xs">
                              Required storage exceeds available space. Consider
                              increasing your EBS volume size.
                            </AlertDescription>
                          </Alert>
                        )}

                        <p className="text-xs text-gray-500 dark:text-gray-400 italic">
                          Note: This is an estimate. Actual storage usage may be
                          lower due to compression.
                        </p>
                      </div>
                    );
                  })()}
              </div>

              <Alert>
                <Info className="h-4 w-4" />
                <AlertDescription className="text-sm space-y-2">
                  <p className="font-medium">How it works</p>
                  <ul className="space-y-1 list-disc list-inside">
                    <li>
                      Restores your source database on the configured schedule
                    </li>
                    <li>
                      Applies anonymization rules to protect sensitive data
                    </li>
                    <li>
                      Keeps up to {maxRestores} restore
                      {maxRestores === 1 ? "" : "s"}, automatically cleaning up
                      older ones
                    </li>
                    <li>
                      Restores with active branches are never deleted,
                      regardless of age
                    </li>
                    <li>New branches automatically use the latest restore</li>
                  </ul>
                </AlertDescription>
              </Alert>
            </div>
          </CardContent>
        </Card>

        {/* Let's Encrypt Section */}
        <Card>
          <CardHeader>
            <CardTitle>Let's Encrypt Certificate</CardTitle>
            <CardDescription>
              Optional: Use a custom domain with automatic HTTPS
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="domain">Custom Domain</Label>
              <Input
                id="domain"
                type="text"
                placeholder="branches-us-east-1.company.com"
                value={domain}
                onChange={(e) => setDomain(e.target.value)}
                disabled={saving}
              />
              <p className="text-xs text-gray-500">
                Leave empty to use self-signed certificate. If you provide a
                domain, make sure it points to this server's IP address.
              </p>
            </div>

            {domain && (
              <div className="space-y-2">
                <Label htmlFor="letsEncryptEmail">
                  Email for Let's Encrypt{" "}
                  <span className="text-red-500">*</span>
                </Label>
                <Input
                  id="letsEncryptEmail"
                  type="email"
                  placeholder="admin@company.com"
                  value={letsEncryptEmail}
                  onChange={(e) => setLetsEncryptEmail(e.target.value)}
                  required={!!domain}
                  disabled={saving}
                />
                <p className="text-xs text-gray-500">
                  Required for certificate issuance and renewal notifications
                </p>
              </div>
            )}

            {domain && (
              <div className="bg-amber-50 border border-amber-200 rounded-lg p-4 dark:bg-amber-950/50 dark:border-amber-800">
                <p className="text-sm text-amber-800 dark:text-amber-200">
                  <strong>Note:</strong> Ensure your domain's DNS A record
                  points to this server's IP address before saving. Let's
                  Encrypt will verify domain ownership before issuing the
                  certificate.
                </p>
              </div>
            )}
          </CardContent>
        </Card>

        {/* Save Button */}
        <div className="flex justify-end">
          <Button type="submit" disabled={saving} size="lg">
            {saving && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
            Save Configuration
          </Button>
        </div>
      </form>
    </div>
  );
}
