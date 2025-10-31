import { useState, useEffect } from "react";
import { useApi } from "@/hooks/use-api";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/shadcn/components/ui/card";
import { Button } from "@/shadcn/components/ui/button";
import { Input } from "@/shadcn/components/ui/input";
import { Label } from "@/shadcn/components/ui/label";
import { Alert, AlertDescription } from "@/shadcn/components/ui/alert";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger, DialogDescription } from "@/shadcn/components/ui/dialog";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shadcn/components/ui/table";
import { Badge } from "@/shadcn/components/ui/badge";
import { CheckIcon, CopyIcon, Trash2Icon, UserPlusIcon } from "lucide-react";

interface User {
  id: string;
  email: string;
  name: string;
  is_admin: boolean;
  created_at: string;
}

interface CreateUserResponse {
  user: User;
  token: string;
}

export function UsersPage() {
  const api = useApi();
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  // Create user dialog state
  const [isCreateDialogOpen, setIsCreateDialogOpen] = useState(false);
  const [createName, setCreateName] = useState("");
  const [createEmail, setCreateEmail] = useState("");
  const [createIsAdmin, setCreateIsAdmin] = useState(false);
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState("");
  const [createdToken, setCreatedToken] = useState("");
  const [tokenCopied, setTokenCopied] = useState(false);

  useEffect(() => {
    loadUsers();
  }, []);

  const loadUsers = async () => {
    try {
      setLoading(true);
      const response = await api.api.usersList();
      setUsers(response.data as User[]);
    } catch (err: any) {
      setError(err.message || "Failed to load users");
    } finally {
      setLoading(false);
    }
  };

  const handleCreateUser = async (e: React.FormEvent) => {
    e.preventDefault();
    setCreateError("");
    setCreating(true);

    try {
      const response = await api.api.usersCreate({
        email: createEmail,
        name: createName,
        is_admin: createIsAdmin,
      });

      const data = response.data as CreateUserResponse;
      setCreatedToken(data.token);
      setUsers([data.user, ...users]);

      // Don't close dialog immediately - show the token first
    } catch (err: any) {
      setCreateError(err.message || "Failed to create user");
    } finally {
      setCreating(false);
    }
  };

  const handleDeleteUser = async (userId: string) => {
    if (!confirm("Are you sure you want to delete this user?")) {
      return;
    }

    try {
      await api.api.usersDelete(userId);
      setUsers(users.filter((u) => u.id !== userId));
    } catch (err: any) {
      alert(`Failed to delete user: ${err.message}`);
    }
  };

  const copyToken = () => {
    navigator.clipboard.writeText(createdToken);
    setTokenCopied(true);
    setTimeout(() => setTokenCopied(false), 2000);
  };

  const closeCreateDialog = () => {
    setIsCreateDialogOpen(false);
    setCreateName("");
    setCreateEmail("");
    setCreateIsAdmin(false);
    setCreatedToken("");
    setTokenCopied(false);
    setCreateError("");
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <p className="text-gray-500">Loading users...</p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <div>
          <h1 className="text-3xl font-bold">Users</h1>
          <p className="text-gray-500 mt-1">Manage user accounts and access</p>
        </div>

        <Dialog open={isCreateDialogOpen} onOpenChange={setIsCreateDialogOpen}>
          <DialogTrigger asChild>
            <Button>
              <UserPlusIcon className="mr-2 h-4 w-4" />
              Create User
            </Button>
          </DialogTrigger>
          <DialogContent className="max-w-2xl">
            <DialogHeader>
              <DialogTitle>
                {createdToken ? "User Created Successfully" : "Create New User"}
              </DialogTitle>
              <DialogDescription>
                {createdToken
                  ? "Save the token below - it won't be shown again"
                  : "Create a new user account and generate their authentication token"}
              </DialogDescription>
            </DialogHeader>

            {createdToken ? (
              <div className="space-y-4">
                <Alert>
                  <AlertDescription>
                    <strong>Important:</strong> Save this token now. The user will need it to authenticate via CLI or web.
                    It will not be shown again.
                  </AlertDescription>
                </Alert>

                <div className="space-y-2">
                  <Label>Authentication Token</Label>
                  <div className="flex gap-2">
                    <Input
                      value={createdToken}
                      readOnly
                      className="font-mono text-sm"
                    />
                    <Button
                      type="button"
                      variant="outline"
                      onClick={copyToken}
                      className="shrink-0"
                    >
                      {tokenCopied ? (
                        <>
                          <CheckIcon className="h-4 w-4 mr-2" />
                          Copied!
                        </>
                      ) : (
                        <>
                          <CopyIcon className="h-4 w-4 mr-2" />
                          Copy
                        </>
                      )}
                    </Button>
                  </div>
                  <p className="text-xs text-gray-500">
                    User can authenticate with: <code>branchd login --token &lt;token&gt;</code>
                  </p>
                </div>

                <Button onClick={closeCreateDialog} className="w-full">
                  Done
                </Button>
              </div>
            ) : (
              <form onSubmit={handleCreateUser} className="space-y-4">
                {createError && (
                  <Alert variant="destructive">
                    <AlertDescription>{createError}</AlertDescription>
                  </Alert>
                )}

                <div className="space-y-2">
                  <Label htmlFor="create-name">Name</Label>
                  <Input
                    id="create-name"
                    value={createName}
                    onChange={(e) => setCreateName(e.target.value)}
                    placeholder="John Doe"
                    required
                    disabled={creating}
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="create-email">Email</Label>
                  <Input
                    id="create-email"
                    type="email"
                    value={createEmail}
                    onChange={(e) => setCreateEmail(e.target.value)}
                    placeholder="john@example.com"
                    required
                    disabled={creating}
                  />
                </div>

                <div className="flex items-center space-x-2">
                  <input
                    id="create-admin"
                    type="checkbox"
                    checked={createIsAdmin}
                    onChange={(e) => setCreateIsAdmin(e.target.checked)}
                    disabled={creating}
                    className="h-4 w-4 rounded border-gray-300"
                  />
                  <Label htmlFor="create-admin" className="cursor-pointer">
                    Admin user (can create users and manage all resources)
                  </Label>
                </div>

                <div className="flex gap-2">
                  <Button type="submit" disabled={creating} className="flex-1">
                    {creating ? "Creating..." : "Create User"}
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    onClick={closeCreateDialog}
                    disabled={creating}
                  >
                    Cancel
                  </Button>
                </div>
              </form>
            )}
          </DialogContent>
        </Dialog>
      </div>

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <Card>
        <CardHeader>
          <CardTitle>All Users</CardTitle>
          <CardDescription>
            {users.length} {users.length === 1 ? "user" : "users"} total
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Email</TableHead>
                <TableHead>Role</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-[100px]">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {users.map((user) => (
                <TableRow key={user.id}>
                  <TableCell className="font-medium">{user.name}</TableCell>
                  <TableCell>{user.email}</TableCell>
                  <TableCell>
                    {user.is_admin ? (
                      <Badge>Admin</Badge>
                    ) : (
                      <Badge variant="secondary">User</Badge>
                    )}
                  </TableCell>
                  <TableCell>
                    {new Date(user.created_at).toLocaleDateString()}
                  </TableCell>
                  <TableCell>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleDeleteUser(user.id)}
                    >
                      <Trash2Icon className="h-4 w-4 text-red-500" />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
