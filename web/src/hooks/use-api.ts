import { useMemo } from "react";
import { Api } from "../lib/openapi";
import { Env } from "@/lib/env";
import { auth } from "@/lib/auth";

export const useApi = () => {
  const api = useMemo(() => {
    const token = auth.getToken();
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
    };

    // Add Authorization header if token exists
    if (token) {
      headers["Authorization"] = `Bearer ${token}`;
    }

    return new Api({
      baseUrl: Env.VITE_API_URL,
      baseApiParams: {
        headers,
        // Include cookies in requests (for HTTPOnly session cookies)
        credentials: "include",
        // Parse JSON responses
        format: "json",
      },
    });
  }, []); // Keep empty deps - token is read on each request via headers

  return api;
};
