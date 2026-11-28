import { toast } from "./components/feedback";

// Every JSON response is { success, message, data }. api() returns `data`, and shows the
// server's message as a toast after a successful write (pass quiet: true to opt out).
// Access token lives in memory only; the refresh token is an httpOnly cookie.
let accessToken: string | null = null;
let refreshing: Promise<any> | null = null;

export interface ApiOptions {
  method?: string;
  body?: unknown;
  auth?: boolean;
  quiet?: boolean;
}

export const setToken = (t: string | null) => {
  accessToken = t;
};

async function raw(
  path: string,
  { method = "GET", body, auth = true }: ApiOptions = {},
) {
  const headers: Record<string, string> = {};
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (auth && accessToken) headers.Authorization = `Bearer ${accessToken}`;
  return fetch(`/api${path}`, {
    method,
    headers,
    credentials: "same-origin",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

export async function refreshSession() {
  refreshing ||= raw("/auth/refresh", { method: "POST", auth: false })
    .then(async (r): Promise<any> => {
      if (!r.ok) return null;
      const d = (await r.json()).data;
      accessToken = d.access_token;
      return d.user;
    })
    .finally(() => {
      refreshing = null;
    });
  return refreshing;
}

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

export async function api(path: string, opts: ApiOptions = {}): Promise<any> {
  let r = await raw(path, opts);
  if (r.status === 401 && opts.auth !== false) {
    if (await refreshSession()) r = await raw(path, opts);
    else window.dispatchEvent(new Event("vaultly:logout"));
  }
  if (r.status === 204) return null;
  const ct = r.headers.get("content-type") || "";
  if (!ct.includes("json")) {
    const text = await r.text();
    if (!r.ok) throw new ApiError(r.status, "Something went wrong");
    return text;
  }
  const body = await r.json();
  if (!r.ok)
    throw new ApiError(r.status, body.message || "Something went wrong");
  if (opts.method && opts.method !== "GET" && !opts.quiet && body.message)
    toast(body.message);
  return body.data ?? null;
}

export const authApi = {
  // Returns the user, or { challenge } when the account has two-factor sign-in on.
  login: async (email: string, password: string) => {
    const d = await api("/auth/login", {
      method: "POST",
      body: { email, password },
      auth: false,
      quiet: true,
    });
    if (d.mfa_required) return { challenge: d.challenge as string };
    accessToken = d.access_token;
    return { user: d.user };
  },
  loginMFA: async (challenge: string, code: string) => {
    const d = await api("/auth/login/mfa", {
      method: "POST",
      body: { challenge, code },
      auth: false,
      quiet: true,
    });
    accessToken = d.access_token;
    return d.user;
  },
  register: async (body: unknown) => {
    const d = await api("/auth/register", {
      method: "POST",
      body,
      auth: false,
      quiet: true,
    });
    accessToken = d.access_token;
    return d.user;
  },
  logout: async () => {
    await api("/auth/logout", {
      method: "POST",
      auth: false,
      quiet: true,
    }).catch(() => {});
    accessToken = null;
  },
};

async function authed(path: string, init: RequestInit) {
  const go = () =>
    fetch(`/api${path}`, {
      ...init,
      credentials: "same-origin",
      headers: {
        ...((init.headers as Record<string, string>) || {}),
        ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}),
      },
    });
  let r = await go();
  if (r.status === 401 && (await refreshSession())) r = await go();
  return r;
}

export async function apiBlob(path: string, body: unknown) {
  const r = await authed(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!r.ok)
    throw new ApiError(
      r.status,
      (await r.json().catch(() => ({}))).message || "Something went wrong",
    );
  return r.blob();
}

export async function apiForm(path: string, form: FormData) {
  const r = await authed(path, { method: "POST", body: form });
  const d = await r.json().catch(() => ({}));
  if (!r.ok) throw new ApiError(r.status, d.message || "Something went wrong");
  return d.data ?? null;
}
