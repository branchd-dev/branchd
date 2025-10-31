import { useEffect, useState } from "react";
import { useApi } from "../hooks/use-api";
import type { GithubComBranchdDevBranchdInternalModelsAnonRule } from "../lib/openapi";
import { Button } from "../shadcn/components/ui/button";
import { Input } from "../shadcn/components/ui/input";
import { Label } from "../shadcn/components/ui/label";
import { Trash2, Plus, ShieldAlert, Info } from "lucide-react";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "../shadcn/components/ui/table";
import {
  Alert,
  AlertDescription,
} from "../shadcn/components/ui/alert";

export function AnonRulesPage() {
  const api = useApi();

  const [rules, setRules] = useState<
    GithubComBranchdDevBranchdInternalModelsAnonRule[]
  >([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // New rule form state
  const [tableName, setTableName] = useState<string>("");
  const [columnName, setColumnName] = useState<string>("");
  const [template, setTemplate] = useState<string>("");
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    loadRules();
  }, []);

  const loadRules = async () => {
    try {
      setLoading(true);
      setError(null);

      const response = await api.api.anonRulesList();
      const data = await response.json();

      setRules(data || []);
    } catch (err: any) {
      setError(err.message || "Failed to load anonymization rules");
    } finally {
      setLoading(false);
    }
  };

  const handleCreateRule = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!tableName || !columnName || !template) {
      return;
    }

    try {
      setSubmitting(true);
      await api.api.anonRulesCreate({
        table: tableName,
        column: columnName,
        template: template,
      });

      // Reload rules
      await loadRules();

      // Clear form
      setTableName("");
      setColumnName("");
      setTemplate("");
    } catch (err: any) {
      setError(err.error?.error || err.message || "Failed to create rule");
    } finally {
      setSubmitting(false);
    }
  };

  const handleDeleteRule = async (ruleId: string) => {
    try {
      await api.api.anonRulesDelete(ruleId);
      await loadRules();
    } catch (err: any) {
      setError(err.error?.error || err.message || "Failed to delete rule");
    }
  };

  return (
    <div className="space-y-8">
      {/* Header */}
      <div>
        <h1 className="text-3xl font-bold tracking-tight">
          Anonymization Rules
        </h1>
        <p className="text-gray-500 mt-1">
          Define rules to protect sensitive data during restores
        </p>
      </div>

      {/* How it works */}
      <Alert>
        <Info className="h-4 w-4" />
        <AlertDescription className="text-sm space-y-3">
          <p className="font-medium">How Anonymization Rules Work</p>
          <p className="text-gray-600">
            Rules are applied automatically during database restores. Define
            simple templates that replace actual values with consistent fake
            data.
          </p>
          <div>
            <p className="text-gray-600 mb-3">
              Use the special{" "}
              <code className="bg-gray-100 px-1.5 py-0.5 rounded text-xs font-mono">
                {"${index}"}
              </code>{" "}
              variable to generate unique values based on row number:
            </p>
            <div className="overflow-hidden rounded border">
              <table className="w-full text-sm">
                <thead className="bg-gray-50">
                  <tr>
                    <th className="text-left px-4 py-2 font-medium text-gray-900">
                      Table
                    </th>
                    <th className="text-left px-4 py-2 font-medium text-gray-900">
                      Column
                    </th>
                    <th className="text-left px-4 py-2 font-medium text-gray-900">
                      Template
                    </th>
                    <th className="text-left px-4 py-2 font-medium text-gray-900">
                      Result
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y bg-white">
                  <tr>
                    <td className="px-4 py-2 font-mono text-xs text-gray-900">
                      users
                    </td>
                    <td className="px-4 py-2 font-mono text-xs text-gray-900">
                      first_name
                    </td>
                    <td className="px-4 py-2 font-mono text-xs text-gray-900">
                      FirstName {"${index}"}
                    </td>
                    <td className="px-4 py-2 text-xs text-gray-600">
                      FirstName 1, FirstName 2, ...
                    </td>
                  </tr>
                  <tr>
                    <td className="px-4 py-2 font-mono text-xs text-gray-900">
                      users
                    </td>
                    <td className="px-4 py-2 font-mono text-xs text-gray-900">
                      email
                    </td>
                    <td className="px-4 py-2 font-mono text-xs text-gray-900">
                      user_{"${index}"}@example.com
                    </td>
                    <td className="px-4 py-2 text-xs text-gray-600">
                      user_1@example.com, user_2@example.com, ...
                    </td>
                  </tr>
                  <tr>
                    <td className="px-4 py-2 font-mono text-xs text-gray-900">
                      users
                    </td>
                    <td className="px-4 py-2 font-mono text-xs text-gray-900">
                      phone
                    </td>
                    <td className="px-4 py-2 font-mono text-xs text-gray-900">
                      +12223334444
                    </td>
                    <td className="px-4 py-2 text-xs text-gray-600">
                      +12223334444 (same for all rows)
                    </td>
                  </tr>
                  <tr>
                    <td className="px-4 py-2 font-mono text-xs text-gray-900">
                      users
                    </td>
                    <td className="px-4 py-2 font-mono text-xs text-gray-900">
                      ssn
                    </td>
                    <td className="px-4 py-2 font-mono text-xs text-gray-900">
                      123-45-6789
                    </td>
                    <td className="px-4 py-2 text-xs text-gray-600">
                      123-45-6789 (same for all rows)
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </AlertDescription>
      </Alert>

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {/* Add New Rule Section */}
      <div className="space-y-4">
        <h2 className="text-lg font-semibold">Add New Rule</h2>
        <form onSubmit={handleCreateRule} className="space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div className="space-y-2">
              <Label htmlFor="table">Table</Label>
              <Input
                id="table"
                placeholder="users"
                value={tableName}
                onChange={(e) => setTableName(e.target.value)}
                required
                disabled={submitting}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="column">Column</Label>
              <Input
                id="column"
                placeholder="email"
                value={columnName}
                onChange={(e) => setColumnName(e.target.value)}
                required
                disabled={submitting}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="template">Template</Label>
              <Input
                id="template"
                placeholder="user_${index}@example.com"
                value={template}
                onChange={(e) => setTemplate(e.target.value)}
                required
                disabled={submitting}
              />
            </div>
          </div>

          <Button type="submit" disabled={submitting}>
            <Plus className="h-4 w-4 mr-2" />
            Add Rule
          </Button>
        </form>
      </div>

      {/* Rules List */}
      <div className="space-y-4">
        <h2 className="text-lg font-semibold">Active Rules</h2>
        {loading ? (
          <div className="text-center py-12 text-gray-500">
            <p>Loading rules...</p>
          </div>
        ) : rules.length === 0 ? (
          <div className="text-center py-12 text-gray-500">
            <ShieldAlert className="h-12 w-12 mx-auto mb-3 opacity-30" />
            <p className="font-medium">No anonymization rules defined</p>
            <p className="text-sm mt-1">
              Add rules above to automatically anonymize sensitive data
            </p>
          </div>
        ) : (
          <div className="border rounded-lg overflow-hidden">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Table</TableHead>
                  <TableHead>Column</TableHead>
                  <TableHead>Template</TableHead>
                  <TableHead className="w-[100px]">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rules.map((rule) => (
                  <TableRow key={rule.id}>
                    <TableCell className="font-mono text-sm">
                      {rule.table}
                    </TableCell>
                    <TableCell className="font-mono text-sm">
                      {rule.column}
                    </TableCell>
                    <TableCell className="font-mono text-sm">
                      {rule.template}
                    </TableCell>
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => rule.id && handleDeleteRule(rule.id)}
                      >
                        <Trash2 className="h-4 w-4 text-gray-500 hover:text-red-600" />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>
    </div>
  );
}
