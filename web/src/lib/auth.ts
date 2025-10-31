// JWT Authentication utilities for local token storage
const TOKEN_KEY = "branchd_token";

export interface AuthUser {
  id: string;
  email: string;
  name: string;
  is_admin: boolean;
  created_at: string;
}

export interface LoginResponse {
  token: string;
  user: AuthUser;
}

export const auth = {
  // Get stored token
  getToken(): string | null {
    return localStorage.getItem(TOKEN_KEY);
  },

  // Store token
  setToken(token: string): void {
    localStorage.setItem(TOKEN_KEY, token);
  },

  // Remove token
  clearToken(): void {
    localStorage.removeItem(TOKEN_KEY);
  },

  // Check if user is authenticated
  isAuthenticated(): boolean {
    return !!this.getToken();
  },

  // Logout (clear token)
  logout(): void {
    this.clearToken();
    window.location.href = "/login";
  },
};
