import type { Row } from "../types";
import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api } from "../api";
import { useAuth } from "../auth";
import { Logo } from "../components/ui";

export default function Invite() {
  const { token } = useParams();
  const { user, loading, reloadOrgs, switchOrg } = useAuth();
  const nav = useNavigate();
  const [inv, setInv] = useState<Row | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    api(`/invites/${token}`, { auth: false })
      .then(setInv)
      .catch((e) => setErr(e.message));
  }, [token]);

  const accept = async () => {
    setBusy(true);
    setErr("");
    try {
      const r = await api(`/invites/${token}/accept`, { method: "POST" });
      await reloadOrgs();
      switchOrg(r.org_id);
      nav("/dashboard");
    } catch (e) {
      setErr(e.message);
      setBusy(false);
    }
  };
  const next = encodeURIComponent(`/invite/${token}`);
  const state = inv?.state;
  return (
    <div className="auth" style={{ gridTemplateColumns: "1fr" }}>
      <div className="auth-l" style={{ alignItems: "center" }}>
        <header style={{ width: "100%" }}>
          <Link to="/">
            <Logo />
          </Link>
        </header>
        <div className="auth-form" style={{ textAlign: "center" }}>
          {!inv && !err && (
            <div className="spinner" style={{ margin: "0 auto" }} />
          )}
          {err && !inv && (
            <>
              <h1>Invitation not found</h1>
              <p className="sub">{err}</p>
              <Link className="btn" to="/">
                Go home
              </Link>
            </>
          )}
          {inv && state !== "pending" && (
            <>
              <h1>
                {state === "accepted"
                  ? "Already accepted"
                  : "Invitation expired"}
              </h1>
              <p className="sub">
                {state === "accepted"
                  ? "This invitation has already been used."
                  : "Ask an admin to send a new one."}
              </p>
              <Link className="btn" to={user ? "/dashboard" : "/login"}>
                {user ? "Open dashboard" : "Sign in"}
              </Link>
            </>
          )}
          {inv && state === "pending" && (
            <>
              <h1>Join {inv.org}</h1>
              <p className="sub">
                <b>{inv.invited_by}</b> invited you as <b>{inv.role}</b>.<br />
                For <span className="mono">{inv.email_hint}</span>
              </p>
              {err && (
                <div className="alert" role="alert">
                  {err}
                </div>
              )}
              {loading ? null : user ? (
                <>
                  <button
                    className="btn primary lg block"
                    disabled={busy}
                    onClick={accept}
                  >
                    {busy ? "Joining…" : `Accept as ${user.email}`}
                  </button>
                  <p className="alt">
                    Wrong account?{" "}
                    <Link
                      to={`/login?next=${next}`}
                      onClick={async () => {
                        await api("/auth/logout", {
                          method: "POST",
                          auth: false,
                        }).catch(() => {});
                      }}
                    >
                      Switch
                    </Link>
                  </p>
                </>
              ) : (
                <>
                  <Link
                    className="btn primary lg block"
                    to={`${inv.has_account ? "/login" : "/register"}?next=${next}`}
                  >
                    {inv.has_account
                      ? "Sign in to accept"
                      : "Create account to accept"}
                  </Link>
                  <p className="alt">
                    {inv.has_account ? (
                      <>
                        No account yet?{" "}
                        <Link to={`/register?next=${next}`}>Create one</Link>
                      </>
                    ) : (
                      <>
                        Already have an account?{" "}
                        <Link to={`/login?next=${next}`}>Sign in</Link>
                      </>
                    )}
                  </p>
                </>
              )}
            </>
          )}
        </div>
        <footer />
      </div>
    </div>
  );
}
