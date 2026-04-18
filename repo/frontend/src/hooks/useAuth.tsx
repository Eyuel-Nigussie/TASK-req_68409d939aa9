import React, { createContext, useContext, useEffect, useMemo, useState } from "react";
import { api, getToken, setToken } from "../api/client";

export interface AuthUser {
  id: string;
  username: string;
  role: string;
  // Set when the account was provisioned with a shared/demo password.
  // The backend rejects every API call except /api/auth/rotate-password,
  // /api/auth/logout, and /api/auth/whoami until this flag clears, so
  // the SPA should route the user to the rotation form when it is true.
  mustRotatePassword?: boolean;
}

interface AuthContextValue {
  user: AuthUser | null;
  loading: boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  rotatePassword: (oldPassword: string, newPassword: string) => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | undefined>(undefined);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<AuthUser | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const tok = getToken();
    if (!tok) {
      setLoading(false);
      return;
    }
    api
      .whoami()
      .then((u) =>
        setUser({
          id: u.id,
          username: u.username,
          role: u.role,
          mustRotatePassword: u.must_rotate_password,
        }),
      )
      .catch(() => setToken(null))
      .finally(() => setLoading(false));
  }, []);

  const value = useMemo<AuthContextValue>(
    () => ({
      user,
      loading,
      login: async (username, password) => {
        const res = await api.login(username, password);
        setToken(res.token);
        setUser({
          id: res.user.id,
          username: res.user.username,
          role: res.user.role,
          mustRotatePassword: res.must_rotate_password ?? res.user.must_rotate_password,
        });
      },
      logout: async () => {
        try {
          await api.logout();
        } catch {
          /* still clear local state below */
        }
        setToken(null);
        setUser(null);
      },
      rotatePassword: async (oldPassword, newPassword) => {
        await api.rotatePassword(oldPassword, newPassword);
        setUser((prev) => (prev ? { ...prev, mustRotatePassword: false } : prev));
      },
    }),
    [user, loading],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used inside AuthProvider");
  return ctx;
}
