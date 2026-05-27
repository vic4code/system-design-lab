"use client";

import { createContext, useContext, useEffect, useState } from "react";

export type User = {
  id: string;
  email: string;
  name: string;
  created_at: string;
};

type AuthState =
  | { status: "loading" }
  | { status: "guest" }
  | { status: "authenticated"; user: User; token: string };

type AuthContextValue = {
  state: AuthState;
  login: (token: string, user: User) => void;
  logout: () => void;
  token: string | null;
  user: User | null;
};

const AuthContext = createContext<AuthContextValue | null>(null);

const TOKEN_KEY = "bs_token";
const USER_KEY  = "bs_user";

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<AuthState>({ status: "loading" });

  useEffect(() => {
    const token = localStorage.getItem(TOKEN_KEY);
    const raw   = localStorage.getItem(USER_KEY);
    if (token && raw) {
      try {
        const user = JSON.parse(raw) as User;
        setState({ status: "authenticated", user, token });
        return;
      } catch {/* fall through */}
    }
    setState({ status: "guest" });
  }, []);

  const login = (token: string, user: User) => {
    localStorage.setItem(TOKEN_KEY, token);
    localStorage.setItem(USER_KEY, JSON.stringify(user));
    setState({ status: "authenticated", user, token });
  };

  const logout = () => {
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(USER_KEY);
    setState({ status: "guest" });
  };

  const token = state.status === "authenticated" ? state.token : null;
  const user  = state.status === "authenticated" ? state.user  : null;

  return (
    <AuthContext.Provider value={{ state, login, logout, token, user }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used inside AuthProvider");
  return ctx;
}
