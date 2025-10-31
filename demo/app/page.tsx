"use client";

import { useState } from "react";

interface User {
  id: number;
  name: string;
  email: string;
  phone: string;
  address: string;
  ssn: string;
}

interface DatabaseState {
  connectionString: string;
  users: User[];
  loading: boolean;
  error: string | null;
  editingUser: User | null;
  fetched: boolean;
}

export default function Home() {
  const branchNames = [
    "schema-only-branch",
    "schema-and-data-branch",
    "anon-rules-branch",
  ];

  const [databases, setDatabases] = useState<DatabaseState[]>([
    {
      connectionString: "",
      users: [],
      loading: false,
      error: null,
      editingUser: null,
      fetched: false,
    },
    {
      connectionString: "",
      users: [],
      loading: false,
      error: null,
      editingUser: null,
      fetched: false,
    },
    {
      connectionString: "",
      users: [],
      loading: false,
      error: null,
      editingUser: null,
      fetched: false,
    },
  ]);

  const [newUser, setNewUser] = useState<{
    [key: number]: {
      name: string;
      email: string;
      phone: string;
      address: string;
      ssn: string;
    };
  }>({
    0: { name: "", email: "", phone: "", address: "", ssn: "" },
    1: { name: "", email: "", phone: "", address: "", ssn: "" },
    2: { name: "", email: "", phone: "", address: "", ssn: "" },
  });

  // Fetch users for a specific database
  const fetchUsers = async (index: number) => {
    const db = databases[index];
    if (!db.connectionString.trim()) {
      return;
    }

    setDatabases((prev) => {
      const newDbs = [...prev];
      newDbs[index] = { ...newDbs[index], loading: true, error: null };
      return newDbs;
    });

    try {
      const response = await fetch(
        `/api/db?connectionString=${encodeURIComponent(db.connectionString)}`,
      );
      const data = await response.json();

      if (data.success) {
        setDatabases((prev) => {
          const newDbs = [...prev];
          newDbs[index] = {
            ...newDbs[index],
            users: data.users,
            loading: false,
            error: null,
            fetched: true,
          };
          return newDbs;
        });
      } else {
        setDatabases((prev) => {
          const newDbs = [...prev];
          newDbs[index] = {
            ...newDbs[index],
            loading: false,
            error: data.error || "Failed to fetch users",
            users: [],
            fetched: true,
          };
          return newDbs;
        });
      }
    } catch (error: any) {
      setDatabases((prev) => {
        const newDbs = [...prev];
        newDbs[index] = {
          ...newDbs[index],
          loading: false,
          error: error.message || "Network error",
          users: [],
          fetched: true,
        };
        return newDbs;
      });
    }
  };

  // Refresh all databases
  const refreshAll = () => {
    databases.forEach((_, index) => {
      fetchUsers(index);
    });
  };

  // Add a new user
  const addUser = async (index: number) => {
    const db = databases[index];
    const userData = newUser[index];

    if (!db.connectionString.trim() || !userData.name || !userData.email) {
      return;
    }

    try {
      const response = await fetch("/api/db", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          connectionString: db.connectionString,
          name: userData.name,
          email: userData.email,
          phone: userData.phone,
          address: userData.address,
          ssn: userData.ssn,
        }),
      });

      const data = await response.json();

      if (data.success) {
        // Clear the form
        setNewUser((prev) => ({
          ...prev,
          [index]: { name: "", email: "", phone: "", address: "", ssn: "" },
        }));
        // Refresh the users list
        await fetchUsers(index);
      } else {
        alert(`Error: ${data.error}`);
      }
    } catch (error: any) {
      alert(`Error: ${error.message}`);
    }
  };

  // Update a user
  const updateUser = async (index: number) => {
    const db = databases[index];
    const user = db.editingUser;

    if (!user) return;

    try {
      const response = await fetch("/api/db", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          connectionString: db.connectionString,
          id: user.id,
          name: user.name,
          email: user.email,
          phone: user.phone,
          address: user.address,
          ssn: user.ssn,
        }),
      });

      const data = await response.json();

      if (data.success) {
        // Clear editing state
        setDatabases((prev) => {
          const newDbs = [...prev];
          newDbs[index] = { ...newDbs[index], editingUser: null };
          return newDbs;
        });
        // Refresh the users list
        await fetchUsers(index);
      } else {
        alert(`Error: ${data.error}`);
      }
    } catch (error: any) {
      alert(`Error: ${error.message}`);
    }
  };

  // Delete a user
  const deleteUser = async (index: number, userId: number) => {
    const db = databases[index];

    if (!confirm("Are you sure you want to delete this user?")) {
      return;
    }

    try {
      const response = await fetch(
        `/api/db?connectionString=${encodeURIComponent(db.connectionString)}&id=${userId}`,
        { method: "DELETE" },
      );

      const data = await response.json();

      if (data.success) {
        // Refresh the users list
        await fetchUsers(index);
      } else {
        alert(`Error: ${data.error}`);
      }
    } catch (error: any) {
      alert(`Error: ${error.message}`);
    }
  };

  return (
    <div className="min-h-screen bg-gray-50 p-8">
      <div className="max-w-7xl mx-auto">
        <div className="mb-8 flex items-center">
          <h1 className="text-3xl font-bold text-gray-900">Branchd Demo</h1>
          <button
            onClick={refreshAll}
            className="bg-gray-800 ml-auto hover:bg-gray-700 text-white font-medium py-2 px-6 rounded-lg transition-colors"
          >
            Refresh All
          </button>
        </div>

        <div className="space-y-8">
          {databases.map((db, index) => (
            <div key={index} className="bg-white rounded-lg shadow-md p-6">
              <h2 className="text-2xl font-bold text-gray-900 mb-4">
                {branchNames[index]}
              </h2>

              {/* Connection String Input */}
              <div className="mb-4">
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  Connection String
                </label>
                <div className="flex gap-2">
                  <input
                    type="text"
                    value={db.connectionString}
                    onChange={(e) => {
                      setDatabases((prev) => {
                        const newDbs = [...prev];
                        newDbs[index] = {
                          ...newDbs[index],
                          connectionString: e.target.value,
                        };
                        return newDbs;
                      });
                    }}
                    placeholder="postgresql://user:password@host:port/database"
                    className="flex-1 px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                  />
                  <button
                    onClick={() => fetchUsers(index)}
                    disabled={db.loading || !db.connectionString.trim()}
                    className="bg-green-600 hover:bg-green-700 disabled:bg-gray-400 text-white font-medium py-2 px-4 rounded-md transition-colors"
                  >
                    {db.loading ? "Loading..." : "Connect"}
                  </button>
                </div>
              </div>

              {/* Error Display */}
              {db.error && (
                <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded-md">
                  <p className="text-red-700 text-sm">Error: {db.error}</p>
                </div>
              )}

              {/* Users Table */}
              {db.fetched && !db.error && (
                <div className="mb-4">
                  <h3 className="text-lg font-medium text-gray-800 mb-2">
                    Users
                  </h3>
                  <div className="overflow-x-auto">
                    <table className="min-w-full divide-y divide-gray-200">
                      <thead className="bg-gray-50">
                        <tr>
                          <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                            ID
                          </th>
                          <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                            Name
                          </th>
                          <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                            Email
                          </th>
                          <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                            Phone
                          </th>
                          <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                            Address
                          </th>
                          <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                            SSN
                          </th>
                          <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                            Actions
                          </th>
                        </tr>
                      </thead>
                      <tbody className="bg-white divide-y divide-gray-200">
                        {db.users.length > 0 ? (
                          db.users.map((user) => (
                          <tr key={user.id}>
                            {db.editingUser?.id === user.id ? (
                              <>
                                <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
                                  {user.id}
                                </td>
                                <td className="px-6 py-4 whitespace-nowrap">
                                  <input
                                    type="text"
                                    value={db.editingUser.name}
                                    onChange={(e) => {
                                      setDatabases((prev) => {
                                        const newDbs = [...prev];
                                        if (newDbs[index].editingUser) {
                                          newDbs[index].editingUser!.name =
                                            e.target.value;
                                        }
                                        return newDbs;
                                      });
                                    }}
                                    className="px-2 py-1 border border-gray-300 rounded text-sm"
                                  />
                                </td>
                                <td className="px-6 py-4 whitespace-nowrap">
                                  <input
                                    type="email"
                                    value={db.editingUser.email}
                                    onChange={(e) => {
                                      setDatabases((prev) => {
                                        const newDbs = [...prev];
                                        if (newDbs[index].editingUser) {
                                          newDbs[index].editingUser!.email =
                                            e.target.value;
                                        }
                                        return newDbs;
                                      });
                                    }}
                                    className="px-2 py-1 border border-gray-300 rounded text-sm"
                                  />
                                </td>
                                <td className="px-6 py-4 whitespace-nowrap">
                                  <input
                                    type="text"
                                    value={db.editingUser.phone || ""}
                                    onChange={(e) => {
                                      setDatabases((prev) => {
                                        const newDbs = [...prev];
                                        if (newDbs[index].editingUser) {
                                          newDbs[index].editingUser!.phone =
                                            e.target.value;
                                        }
                                        return newDbs;
                                      });
                                    }}
                                    className="px-2 py-1 border border-gray-300 rounded text-sm"
                                  />
                                </td>
                                <td className="px-6 py-4 whitespace-nowrap">
                                  <input
                                    type="text"
                                    value={db.editingUser.address || ""}
                                    onChange={(e) => {
                                      setDatabases((prev) => {
                                        const newDbs = [...prev];
                                        if (newDbs[index].editingUser) {
                                          newDbs[index].editingUser!.address =
                                            e.target.value;
                                        }
                                        return newDbs;
                                      });
                                    }}
                                    className="px-2 py-1 border border-gray-300 rounded text-sm"
                                  />
                                </td>
                                <td className="px-6 py-4 whitespace-nowrap">
                                  <input
                                    type="text"
                                    value={db.editingUser.ssn || ""}
                                    onChange={(e) => {
                                      setDatabases((prev) => {
                                        const newDbs = [...prev];
                                        if (newDbs[index].editingUser) {
                                          newDbs[index].editingUser!.ssn =
                                            e.target.value;
                                        }
                                        return newDbs;
                                      });
                                    }}
                                    className="px-2 py-1 border border-gray-300 rounded text-sm"
                                  />
                                </td>
                                <td className="px-6 py-4 whitespace-nowrap text-sm">
                                  <button
                                    onClick={() => updateUser(index)}
                                    className="text-green-600 hover:text-green-900 mr-3"
                                  >
                                    Save
                                  </button>
                                  <button
                                    onClick={() => {
                                      setDatabases((prev) => {
                                        const newDbs = [...prev];
                                        newDbs[index].editingUser = null;
                                        return newDbs;
                                      });
                                    }}
                                    className="text-gray-600 hover:text-gray-900"
                                  >
                                    Cancel
                                  </button>
                                </td>
                              </>
                            ) : (
                              <>
                                <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
                                  {user.id}
                                </td>
                                <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
                                  {user.name}
                                </td>
                                <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
                                  {user.email}
                                </td>
                                <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
                                  {user.phone}
                                </td>
                                <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
                                  {user.address}
                                </td>
                                <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
                                  {user.ssn}
                                </td>
                                <td className="px-6 py-4 whitespace-nowrap text-sm">
                                  <button
                                    onClick={() => {
                                      setDatabases((prev) => {
                                        const newDbs = [...prev];
                                        newDbs[index].editingUser = { ...user };
                                        return newDbs;
                                      });
                                    }}
                                    className="text-blue-600 hover:text-blue-900 mr-3"
                                  >
                                    Edit
                                  </button>
                                  <button
                                    onClick={() => deleteUser(index, user.id)}
                                    className="text-red-600 hover:text-red-900"
                                  >
                                    Delete
                                  </button>
                                </td>
                              </>
                            )}
                          </tr>
                        ))
                        ) : (
                          <tr>
                            <td
                              colSpan={7}
                              className="px-6 py-4 text-center text-sm text-gray-500"
                            >
                              No users found
                            </td>
                          </tr>
                        )}
                      </tbody>
                    </table>
                  </div>
                </div>
              )}

              {/* Add User Form */}
              {db.connectionString && !db.error && (
                <div className="border-t pt-4">
                  <h3 className="text-lg font-medium text-gray-800 mb-2">
                    New User
                  </h3>
                  <div className="grid grid-cols-6 gap-2 mb-2">
                    <input
                      type="text"
                      value={newUser[index].name}
                      onChange={(e) => {
                        setNewUser((prev) => ({
                          ...prev,
                          [index]: { ...prev[index], name: e.target.value },
                        }));
                      }}
                      placeholder="Name"
                      className="px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                    />
                    <input
                      type="email"
                      value={newUser[index].email}
                      onChange={(e) => {
                        setNewUser((prev) => ({
                          ...prev,
                          [index]: { ...prev[index], email: e.target.value },
                        }));
                      }}
                      placeholder="Email"
                      className="px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                    />
                    <input
                      type="text"
                      value={newUser[index].phone}
                      onChange={(e) => {
                        setNewUser((prev) => ({
                          ...prev,
                          [index]: { ...prev[index], phone: e.target.value },
                        }));
                      }}
                      placeholder="Phone"
                      className="px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                    />
                    <input
                      type="text"
                      value={newUser[index].address}
                      onChange={(e) => {
                        setNewUser((prev) => ({
                          ...prev,
                          [index]: { ...prev[index], address: e.target.value },
                        }));
                      }}
                      placeholder="Address"
                      className="px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                    />
                    <input
                      type="text"
                      value={newUser[index].ssn}
                      onChange={(e) => {
                        setNewUser((prev) => ({
                          ...prev,
                          [index]: { ...prev[index], ssn: e.target.value },
                        }));
                      }}
                      placeholder="SSN"
                      className="px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                    />
                    <button
                      onClick={() => addUser(index)}
                      disabled={!newUser[index].name || !newUser[index].email}
                      className="bg-blue-600 hover:bg-blue-700 disabled:bg-gray-400 text-white font-medium py-2 px-4 rounded-md transition-colors w-full"
                    >
                      Add
                    </button>
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
