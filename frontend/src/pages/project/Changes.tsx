import type { FormEvent } from "react";
import type { Row } from "../../types";
import { useEffect, useState } from "react";
import { api } from "../../api";
import { useAuth } from "../../auth";
import { Modal } from "../../components/ui";
import {
  EmptyRow,
  EnvTag,
  Rel,
  TableFoot,
  plural,
} from "../../components/table";

const STATUS_LABEL: Record<string, string> = {
  pending: "Waiting for review",
  applied: "Applied",
  rejected: "Rejected",
  cancelled: "Cancelled",
};

// Change requests on protected environments: someone proposes, someone else approves.
export default function Changes({
  project,
  onCount,
}: {
  project: Row;
  onCount?: (n: number) => void;
}) {
  const pid = project.id;
  const [view, setView] = useState<"pending" | "">("pending");
  const [list, setList] = useState<Row[] | null>(null);
  const [open, setOpen] = useState<string | null>(null);
  const load = () =>
    api(`/projects/${pid}/changes${view ? `?status=${view}` : ""}`).then(
      (l) => {
        setList(l);
        if (view === "pending") onCount?.(l.length);
      },
    );
  useEffect(() => {
    load();
  }, [pid, view]);

  const protectedEnvs = project.environments.filter((e: Row) => e.protected);
  return (
    <>
      <div className="bar" style={{ paddingTop: 18 }}>
        <div className="muted" style={{ flex: 1 }}>
          {protectedEnvs.length ? (
            <>
              Changes to{" "}
              {protectedEnvs.map((e: Row, i: number) => (
                <span key={e.id}>
                  {i > 0 && ", "}
                  <b>{e.name}</b>
                </span>
              ))}{" "}
              by non-admins wait here until someone else approves them.
            </>
          ) : (
            <>
              No environment is protected yet. An admin can turn on approvals in{" "}
              <b>Settings</b>.
            </>
          )}
        </div>
        <div className="seg" role="tablist">
          <button
            role="tab"
            aria-selected={view === "pending"}
            className={view === "pending" ? "on" : ""}
            onClick={() => setView("pending")}
          >
            Open
          </button>
          <button
            role="tab"
            aria-selected={view === ""}
            className={view === "" ? "on" : ""}
            onClick={() => setView("")}
          >
            All
          </button>
        </div>
      </div>
      <div className="scroll-x">
        <table className="table">
          <thead>
            <tr>
              <th style={{ width: "30%" }}>Secret</th>
              <th>Change</th>
              <th>Author</th>
              <th>Status</th>
              <th style={{ width: 130 }}>Opened</th>
            </tr>
          </thead>
          <tbody>
            {(list || []).map((c) => (
              <tr
                key={c.id}
                className="clickable"
                onClick={() => setOpen(c.id)}
              >
                <td className="c-main mono">
                  <EnvTag slug={c.env} />{" "}
                  {c.path !== "/" && <span className="dim">{c.path}/</span>}
                  {c.key}
                </td>
                <td>
                  {c.op === "delete" ? (
                    <span className="badge delete">delete</span>
                  ) : (
                    <span
                      className={`badge ${c.base_version ? "update" : "create"}`}
                    >
                      {c.base_version ? "update" : "create"}
                    </span>
                  )}
                  {c.stale && (
                    <span
                      className="badge expired"
                      style={{ marginLeft: 6 }}
                      title="The secret changed after this request was opened"
                    >
                      stale
                    </span>
                  )}
                </td>
                <td className="dim">{c.author}</td>
                <td>
                  <span className={`cr-status ${c.status}`}>
                    {STATUS_LABEL[c.status]}
                  </span>
                  {c.reviewer && c.status !== "pending" && (
                    <span className="dim"> · {c.reviewer}</span>
                  )}
                </td>
                <td className="dim">
                  <Rel iso={c.created_at} />
                </td>
              </tr>
            ))}
            {list && !list.length && (
              <EmptyRow
                cols={5}
                icon="check"
                title={
                  view ? "Nothing waiting for review" : "No change requests yet"
                }
                hint={
                  view
                    ? "New requests from protected environments show up here."
                    : undefined
                }
              />
            )}
          </tbody>
        </table>
        <TableFoot>{plural((list || []).length, "request")}</TableFoot>
      </div>
      {open && (
        <Review
          pid={pid}
          id={open}
          onClose={() => setOpen(null)}
          onDone={() => {
            setOpen(null);
            load();
          }}
        />
      )}
    </>
  );
}

function Review({
  pid,
  id,
  onClose,
  onDone,
}: {
  pid: string;
  id: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const { user } = useAuth();
  const [c, setC] = useState<Row | null>(null);
  const [note, setNote] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    api(`/projects/${pid}/changes/${id}`)
      .then(setC)
      .catch((x) => setErr(x.message));
  }, [pid, id]);

  const act =
    (verb: "approve" | "reject" | "cancel") => async (e?: FormEvent) => {
      e?.preventDefault();
      setErr("");
      setBusy(true);
      try {
        await api(`/projects/${pid}/changes/${id}/${verb}`, {
          method: "POST",
          body: verb === "cancel" ? undefined : { note },
        });
        onDone();
      } catch (x) {
        setErr(x.message);
        setBusy(false);
      }
    };
  const mine = c && c.author_id === user?.id;
  return (
    <Modal
      title={
        c
          ? `${c.op === "delete" ? "Delete" : c.base_version ? "Update" : "Create"} ${c.key}`
          : "Change request"
      }
      onClose={onClose}
    >
      {err && <div className="alert">{err}</div>}
      {!c ? (
        !err && <div className="spinner" style={{ margin: 20 }} />
      ) : (
        <form onSubmit={act("approve")}>
          <p className="muted" style={{ fontSize: 13.5, marginTop: 0 }}>
            <EnvTag slug={c.env} /> <span className="mono">{c.path}</span> ·
            proposed by <b>{c.author}</b> <Rel iso={c.created_at} />
            {c.comment && (
              <>
                <br />“{c.comment}”
              </>
            )}
          </p>
          {c.stale && (
            <div className="alert warn">
              This secret changed after the request was opened (now version{" "}
              {c.current_version}, request was based on{" "}
              {c.base_version || "nothing"}). It can't be applied. Ask the
              author to open a new one.
            </div>
          )}
          <div className="cr-diff">
            <div>
              <label>
                Current{c.base_version ? ` · v${c.current_version}` : ""}
              </label>
              <pre className={c.current == null ? "none" : "old"}>
                {c.current ?? "Does not exist yet"}
              </pre>
            </div>
            <div>
              <label>Proposed</label>
              <pre className={c.op === "delete" ? "none" : "new"}>
                {c.op === "delete" ? "Deleted" : c.proposed}
              </pre>
            </div>
          </div>
          {c.status !== "pending" ? (
            <p className="muted" style={{ fontSize: 13.5 }}>
              <span className={`cr-status ${c.status}`}>
                {STATUS_LABEL[c.status]}
              </span>
              {c.reviewer && (
                <>
                  {" "}
                  by <b>{c.reviewer}</b>
                </>
              )}{" "}
              <Rel iso={c.decided_at} />
              {c.note && (
                <>
                  <br />“{c.note}”
                </>
              )}
            </p>
          ) : mine ? (
            <div className="modal-actions">
              <span className="muted" style={{ flex: 1, fontSize: 13 }}>
                Someone else has to review your change.
              </span>
              <button type="button" className="btn" onClick={onClose}>
                Close
              </button>
              <button
                type="button"
                className="btn danger"
                disabled={busy}
                onClick={() => act("cancel")()}
              >
                Cancel request
              </button>
            </div>
          ) : (
            <>
              <div className="field">
                <label>
                  Note
                  <span className="hint">Optional, shown to the author</span>
                </label>
                <input
                  className="input"
                  value={note}
                  maxLength={500}
                  onChange={(e) => setNote(e.target.value)}
                />
              </div>
              <div className="modal-actions">
                <button type="button" className="btn" onClick={onClose}>
                  Close
                </button>
                <button
                  type="button"
                  className="btn danger"
                  disabled={busy}
                  onClick={() => act("reject")()}
                >
                  Reject
                </button>
                <button className="btn primary" disabled={busy || c.stale}>
                  Approve and apply
                </button>
              </div>
            </>
          )}
        </form>
      )}
    </Modal>
  );
}
