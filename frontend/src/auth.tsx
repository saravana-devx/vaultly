import {
  createContext,
  useContext,
  useEffect,
  useState,
  useCallback,
  type ReactNode,
} from "react";
import type { Org, User } from "./types";
import { authApi, refreshSession, api } from "./api";

export interface AuthValue {
  user: User | null;
  orgs: Org[];
  loading: boolean;
  orgId: string | null;
  org: Org | undefined;
  switchOrg: (id: string) => void;
  reloadOrgs: () => Promise<Org[]>;
  /** Resolves to a challenge string when a second factor is still needed. */
  login: (email: string, password: string) => Promise<string | null>;
  loginMFA: (challenge: string, code: string) => Promise<void>;
  register: (body: unknown) => Promise<void>;
  logout: () => Promise<void>;
}

const Ctx = createContext<AuthValue | null>(null);
export const useAuth = () => useContext(Ctx) as AuthValue;

export function AuthProvider({ children }: { children?: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [orgs, setOrgs] = useState<Org[]>([]);
  const [orgId, setOrgId] = useState<string | null>(() =>
    localStorage.getItem("vaultly_org"),
  );
  const [loading, setLoading] = useState(true);

  const loadOrgs = useCallback(async () => {
    const list: Org[] = await api("/orgs");
    setOrgs(list);
    setOrgId((cur): string | null => {
      const id = list.find((o) => o.id === cur)?.id || list[0]?.id || null;
      if (id) localStorage.setItem("vaultly_org", id);
      return id;
    });
    return list;
  }, []);

  const start = useCallback(
    async (u: User) => {
      setUser(u);
      await loadOrgs();
    },
    [loadOrgs],
  );

  useEffect(() => {
    refreshSession()
      .then((u) => (u ? start(u) : null))
      .finally(() => setLoading(false));
    const onLogout = () => {
      setUser(null);
      setOrgs([]);
    };
    window.addEventListener("vaultly:logout", onLogout);
    return () => window.removeEventListener("vaultly:logout", onLogout);
  }, [start]);

  const value = {
    user,
    orgs,
    loading,
    orgId,
    org: orgs.find((o) => o.id === orgId),
    switchOrg: (id: string) => {
      localStorage.setItem("vaultly_org", id);
      setOrgId(id);
    },
    reloadOrgs: loadOrgs,
    login: async (e: string, p: string) => {
      const r = await authApi.login(e, p);
      if (r.challenge) return r.challenge;
      await start(r.user);
      return null;
    },
    loginMFA: async (ch: string, code: string) =>
      start(await authApi.loginMFA(ch, code)),
    register: async (b: unknown) => start(await authApi.register(b)),
    logout: async () => {
      await authApi.logout();
      setUser(null);
      setOrgs([]);
    },
  };
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}
