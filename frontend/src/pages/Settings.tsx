import type { FormEvent } from "react";
import type { Row } from "../types";
import { useEffect, useRef, useState } from "react";
import { api, apiBlob, apiForm } from "../api";
import { useAuth } from "../auth";
import { Expiry, Modal, Icon, copy } from "../components/ui";
import { ask, toast } from "../components/feedback";
import { EmptyRow, Rel, RowMenu, TableFoot, plural } from "../components/table";

export default function Settings() {
  const { org, orgId, user } = useAuth();
  return (
    <>
      <div className="head">
        <div>
          <h1>Settings</h1>
          <p>{user?.email}</p>
        </div>
      </div>
      <TwoFactor />
      <PersonalTokens />
      {org?.role === "admin" && <Backup orgId={orgId!} orgName={org.name} />}
    </>
  );
}

function TwoFactor() {
  const [st, setSt] = useState<Row | null>(null);
  const [setup, setSetup] = useState<Row | null>(null);
  const [codes, setCodes] = useState<string[] | null>(null);
  const [action, setAction] = useState<"disable" | "regen" | null>(null);
  const [err, setErr] = useState("");
  const load = () => api("/me/mfa").then(setSt);
  useEffect(() => {
    load();
  }, []);

  const start = async () => {
    setErr("");
    setSetup(await api("/me/mfa/setup", { method: "POST", quiet: true }));
  };
  const confirm = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setErr("");
    const code = String(new FormData(e.currentTarget).get("code"));
    try {
      const d = await api("/me/mfa/enable", { method: "POST", body: { code } });
      setSetup(null);
      setCodes(d.recovery_codes);
      load();
    } catch (x) {
      setErr(x.message);
    }
  };
  const act = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setErr("");
    const f = new FormData(e.currentTarget);
    try {
      if (action === "disable")
        await api("/me/mfa/disable", {
          method: "POST",
          body: { password: f.get("password"), code: f.get("code") },
        });
      else
        setCodes(
          (
            await api("/me/mfa/recovery-codes", {
              method: "POST",
              body: { code: f.get("code") },
            })
          ).recovery_codes,
        );
      setAction(null);
      load();
    } catch (x) {
      setErr(x.message);
    }
  };

  return (
    <section className="block">
      <div className="sect">
        <div>
          <h2>Two-factor sign-in</h2>
          <small>
            Ask for a code from an authenticator app (1Password, Google
            Authenticator, Authy…) after your password.
          </small>
        </div>
        {st && !st.enabled && (
          <button className="btn primary" onClick={start}>
            <Icon name="shield" size={15} /> Turn on
          </button>
        )}
      </div>
      {st &&
        (st.enabled ? (
          <div className="mfa-on">
            <div>
              <span className="badge create">On</span>{" "}
              <span className="dim">
                since{" "}
                {new Date(st.enabled_at).toLocaleDateString(undefined, {
                  day: "numeric",
                  month: "short",
                  year: "numeric",
                })}{" "}
                · {plural(st.recovery_codes_left, "recovery code")} left
              </span>
              {st.recovery_codes_left <= 3 && (
                <div className="alert warn" style={{ margin: "12px 0 0" }}>
                  You are running low on recovery codes. Create new ones.
                </div>
              )}
            </div>
            <div className="row-actions">
              <button
                className="btn"
                onClick={() => {
                  setErr("");
                  setAction("regen");
                }}
              >
                New recovery codes
              </button>
              <button
                className="btn danger"
                onClick={() => {
                  setErr("");
                  setAction("disable");
                }}
              >
                Turn off
              </button>
            </div>
          </div>
        ) : (
          <p className="muted" style={{ fontSize: 13.5 }}>
            Off. Anyone with your password can sign in.
          </p>
        ))}

      {setup && (
        <Modal title="Set up two-factor sign-in" onClose={() => setSetup(null)}>
          <form onSubmit={confirm}>
            {err && <div className="alert">{err}</div>}
            <p className="muted" style={{ fontSize: 13.5, marginTop: 0 }}>
              1. Scan this code with your authenticator app.
            </p>
            <img
              className="qr"
              src={setup.qr_png}
              alt="QR code for your authenticator app"
              width={192}
              height={192}
            />
            <details className="manual">
              <summary>Can’t scan it? Enter the key by hand</summary>
              <div className="tokenbox">
                {setup.secret.match(/.{1,4}/g).join(" ")}
              </div>
            </details>
            <div className="field">
              <label>2. Enter the 6-digit code it shows</label>
              <input
                className="input mono otp"
                name="code"
                required
                autoFocus
                autoComplete="one-time-code"
                inputMode="numeric"
                pattern="[0-9 ]{6,7}"
                maxLength={7}
                placeholder="123 456"
              />
            </div>
            <div className="modal-actions">
              <button
                type="button"
                className="btn"
                onClick={() => setSetup(null)}
              >
                Cancel
              </button>
              <button className="btn primary">Turn on</button>
            </div>
          </form>
        </Modal>
      )}

      {codes && (
        <Modal title="Save your recovery codes" onClose={() => setCodes(null)}>
          <div className="alert warn">
            If you lose your phone, these codes are the only way back in. Each
            works once. They won’t be shown again.
          </div>
          <ol className="rcodes">
            {codes.map((c) => (
              <li key={c}>{c}</li>
            ))}
          </ol>
          <div className="modal-actions">
            <button
              className="btn"
              onClick={() => copy(codes.join("\n"), "Recovery codes copied")}
            >
              <Icon name="copy" size={15} /> Copy
            </button>
            <button
              className="btn"
              onClick={() => {
                const a = document.createElement("a");
                a.href = URL.createObjectURL(
                  new Blob([codes.join("\n") + "\n"], { type: "text/plain" }),
                );
                a.download = "vaultly-recovery-codes.txt";
                a.click();
              }}
            >
              <Icon name="download" size={15} /> Download
            </button>
            <button className="btn primary" onClick={() => setCodes(null)}>
              I saved them
            </button>
          </div>
        </Modal>
      )}

      {action && (
        <Modal
          title={
            action === "disable"
              ? "Turn off two-factor sign-in"
              : "Create new recovery codes"
          }
          onClose={() => setAction(null)}
        >
          <form onSubmit={act}>
            {err && <div className="alert">{err}</div>}
            <p className="muted" style={{ fontSize: 13.5, marginTop: 0 }}>
              {action === "disable"
                ? "Your recovery codes will be deleted too."
                : "Your current recovery codes will stop working."}
            </p>
            {action === "disable" && (
              <div className="field">
                <label>Password</label>
                <input
                  className="input"
                  name="password"
                  type="password"
                  required
                  autoFocus
                  autoComplete="current-password"
                />
              </div>
            )}
            <div className="field">
              <label>Code from your app, or a recovery code</label>
              <input
                className="input mono"
                name="code"
                required
                autoFocus={action !== "disable"}
                autoComplete="one-time-code"
                spellCheck={false}
              />
            </div>
            <div className="modal-actions">
              <button
                type="button"
                className="btn"
                onClick={() => setAction(null)}
              >
                Cancel
              </button>
              <button
                className={`btn ${action === "disable" ? "danger" : "primary"}`}
              >
                {action === "disable" ? "Turn off" : "Create codes"}
              </button>
            </div>
          </form>
        </Modal>
      )}
    </section>
  );
}

function PersonalTokens() {
  const [list, setList] = useState<Row | null>(null);
  const [open, setOpen] = useState(false);
  const [created, setCreated] = useState<string | null>(null);
  const [f, setF] = useState<Row>({ name: "", expire_days: 90 });
  const [err, setErr] = useState("");
  const load = () => api("/me/tokens").then(setList);
  useEffect(() => {
    load();
  }, []);
  return (
    <section className="block">
      <div className="sect">
        <div>
          <h2>Personal access tokens</h2>
          <small>
            Call the API as yourself, with your roles and environment access.
            Use them for scripts and local tooling.
          </small>
        </div>
        <button
          className="btn"
          onClick={() => {
            setErr("");
            setOpen(true);
          }}
        >
          <Icon name="plus" size={15} /> New token
        </button>
      </div>
      <div className="scroll-x">
        <table className="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Token</th>
              <th>Expires</th>
              <th>Last used</th>
              <th style={{ width: 56 }} />
            </tr>
          </thead>
          <tbody>
            {(list || []).map((t: Row) => (
              <tr key={t.id}>
                <td className="c-main">{t.name}</td>
                <td className="mono dim" style={{ fontSize: 12.5 }}>
                  {t.prefix}…
                </td>
                <td>
                  <Expiry iso={t.expires_at} />
                </td>
                <td className="dim">
                  <Rel iso={t.last_used_at} />
                </td>
                <td>
                  <RowMenu
                    items={[
                      {
                        label: "Revoke token",
                        icon: "trash",
                        danger: true,
                        run: async () => {
                          if (
                            await ask(`Revoke ${t.name}?`, {
                              confirm: "Revoke",
                              danger: true,
                            })
                          ) {
                            await api(`/me/tokens/${t.id}`, {
                              method: "DELETE",
                            });
                            load();
                          }
                        },
                      },
                    ]}
                  />
                </td>
              </tr>
            ))}
            {list && !list.length && (
              <EmptyRow
                cols={5}
                icon="key2"
                title="No personal tokens"
                hint="Create one to call the API from scripts as yourself."
              />
            )}
          </tbody>
        </table>
        <TableFoot>{plural((list || []).length, "token")}</TableFoot>
      </div>
      {open && (
        <Modal title="New personal access token" onClose={() => setOpen(false)}>
          <form
            onSubmit={async (e) => {
              e.preventDefault();
              try {
                const d = await api("/me/tokens", {
                  method: "POST",
                  body: { name: f.name, expire_days: +f.expire_days },
                });
                setOpen(false);
                setCreated(d.token);
                load();
              } catch (x) {
                setErr(x.message);
              }
            }}
          >
            {err && <div className="alert">{err}</div>}
            <div className="field">
              <label>Name</label>
              <input
                className="input"
                required
                autoFocus
                value={f.name}
                onChange={(e) => setF({ ...f, name: e.target.value })}
                placeholder="laptop"
              />
            </div>
            <div className="field">
              <label>
                Expires in<span className="hint">days, 0 = never</span>
              </label>
              <input
                className="input"
                type="number"
                min="0"
                value={f.expire_days}
                onChange={(e) => setF({ ...f, expire_days: e.target.value })}
              />
            </div>
            <div className="modal-actions">
              <button
                type="button"
                className="btn"
                onClick={() => setOpen(false)}
              >
                Cancel
              </button>
              <button className="btn primary">Create token</button>
            </div>
          </form>
        </Modal>
      )}
      {created && (
        <Modal title="Token created" onClose={() => setCreated(null)}>
          <div className="alert warn">
            Copy this token now — it won’t be shown again.
          </div>
          <div className="tokenbox">{created}</div>
          <p className="muted mono" style={{ fontSize: 12.5 }}>
            curl -H "Authorization: Bearer {created.slice(0, 10)}…" /api/orgs
          </p>
          <div className="modal-actions">
            <button
              className="btn"
              onClick={async () => copy(created, "Copied")}
            >
              Copy
            </button>
            <button className="btn primary" onClick={() => setCreated(null)}>
              Done
            </button>
          </div>
        </Modal>
      )}
    </section>
  );
}

function Backup({ orgId, orgName }: { orgId: string; orgName: string }) {
  const [pass, setPass] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [rpass, setRpass] = useState("");
  const [result, setResult] = useState<Row | null>(null);
  const file = useRef<HTMLInputElement>(null);
  const [fileName, setFileName] = useState("");

  const download = async (e: FormEvent) => {
    e.preventDefault();
    setErr("");
    setBusy(true);
    try {
      const blob = await apiBlob(`/orgs/${orgId}/backup`, { passphrase: pass });
      const a = document.createElement("a");
      a.href = URL.createObjectURL(blob);
      a.download = `vaultly-backup-${new Date().toISOString().slice(0, 10)}.vbk`;
      a.click();
      setPass("");
      toast("Encrypted backup downloaded");
    } catch (x) {
      setErr(x.message);
    }
    setBusy(false);
  };
  const restore = async (e: FormEvent) => {
    e.preventDefault();
    setErr("");
    setResult(null);
    const fd = new FormData();
    fd.append("file", file.current!.files![0]);
    fd.append("passphrase", rpass);
    setBusy(true);
    try {
      setResult(await apiForm(`/orgs/${orgId}/restore`, fd));
      setRpass("");
      file.current!.value = "";
      setFileName("");
      window.dispatchEvent(new Event("vaultly:projects"));
    } catch (x) {
      setErr(x.message);
    }
    setBusy(false);
  };
  return (
    <section className="block">
      <div className="sect">
        <div>
          <h2>Backup &amp; restore</h2>
          <small>{orgName}</small>
        </div>
      </div>
      {err && <div className="alert">{err}</div>}
      <div className="cols2">
        <form onSubmit={download}>
          <h3 className="h3">Create a backup</h3>
          <p className="muted" style={{ fontSize: 13.5, margin: "4px 0 14px" }}>
            Every project, environment, secret and version as ciphertext, sealed
            again with your passphrase. Tokens, webhooks and integration
            credentials are not included.
          </p>
          <div className="field">
            <label>
              Passphrase
              <span className="hint">
                12+ characters — you’ll need it to restore
              </span>
            </label>
            <input
              className="input"
              type="password"
              minLength={12}
              required
              autoComplete="new-password"
              value={pass}
              onChange={(e) => setPass(e.target.value)}
            />
          </div>
          <button className="btn" disabled={busy}>
            <Icon name="download" size={15} /> Download encrypted backup
          </button>
        </form>
        <form onSubmit={restore}>
          <h3 className="h3">Restore from a backup</h3>
          <p className="muted" style={{ fontSize: 13.5, margin: "4px 0 14px" }}>
            Projects keep their original IDs and are never overwritten.
            Restoring needs the same root key the backup was made with: the
            OpenBao data, or MASTER_KEY in local mode.
          </p>
          <div className="field">
            <label>Backup file</label>
            <label className="filepick">
              <input
                type="file"
                accept=".vbk"
                required
                ref={file}
                onChange={(e) => setFileName(e.target.files?.[0]?.name || "")}
              />
              <span className="btn sm">
                <Icon name="upload" size={14} /> Choose file
              </span>
              <span className={`fname${fileName ? "" : " dim"}`}>
                {fileName || "No file chosen (.vbk)"}
              </span>
            </label>
          </div>
          <div className="field">
            <label>Passphrase</label>
            <input
              className="input"
              type="password"
              required
              autoComplete="off"
              value={rpass}
              onChange={(e) => setRpass(e.target.value)}
            />
          </div>
          <button className="btn" disabled={busy}>
            <Icon name="upload" size={15} /> Restore
          </button>
        </form>
      </div>
      {result && (
        <div className="alert warn" style={{ marginTop: 18 }}>
          Restored {result.restored} project{result.restored === 1 ? "" : "s"} (
          {result.secrets} secrets).
          {result.skipped?.map((s: Row) => (
            <div key={s.project}>
              Skipped <b>{s.project}</b>: {s.reason}
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
