import type { Row } from "../../types";
import { useEffect, useState } from "react";
import { api } from "../../api";
import { Icon } from "../../components/ui";
import { EmptyRow, EnvTag, TableFoot, plural } from "../../components/table";

type Filter = "drift" | "all";
const LETTERS = "ABCDEFGHIJ";

// Lines up environments key by key. Values never leave the server: each cell shows which
// variant an environment has (same letter = same value), so drift is visible without reveals.
export default function Compare({ project }: { project: Row }) {
  const pid = project.id;
  const all: string[] = project.environments.map((e: Row) => e.slug);
  const [envs, setEnvs] = useState<string[]>(all);
  const [path, setPath] = useState("/");
  const [draft, setDraft] = useState("/");
  const [filter, setFilter] = useState<Filter>("drift");
  const [data, setData] = useState<Row | null>(null);
  const [err, setErr] = useState("");

  useEffect(() => {
    if (envs.length < 2) {
      setData(null);
      return;
    }
    setErr("");
    api(
      `/projects/${pid}/secrets/compare?envs=${encodeURIComponent(envs.join(","))}&path=${encodeURIComponent(path)}`,
    )
      .then(setData)
      .catch((x) => {
        setData(null);
        setErr(x.message);
      });
  }, [pid, envs, path]);

  const toggle = (slug: string) =>
    setEnvs((cur) =>
      cur.includes(slug)
        ? cur.filter((e) => e !== slug)
        : all.filter((e) => cur.includes(e) || e === slug),
    );
  const rows: Row[] = (data?.rows || []).filter(
    (r: Row) => filter === "all" || r.status !== "same",
  );
  const cols = (data?.envs.length || 0) + 2;

  return (
    <>
      <div
        className="bar"
        style={{ paddingTop: 18, flexWrap: "wrap", gap: 10 }}
      >
        <div className="seg" role="group" aria-label="Environments to compare">
          {project.environments.map((e: Row) => (
            <button
              key={e.id}
              aria-pressed={envs.includes(e.slug)}
              className={envs.includes(e.slug) ? "on" : ""}
              onClick={() => toggle(e.slug)}
            >
              {e.name}
            </button>
          ))}
        </div>
        <form
          className="cmp-path"
          onSubmit={(e) => {
            e.preventDefault();
            setPath(draft.trim() || "/");
          }}
        >
          <Icon name="folder" size={14} />
          <input
            className="input sm mono"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            aria-label="Folder"
            placeholder="/"
          />
        </form>
        <div style={{ flex: 1 }} />
        <div className="seg" role="tablist">
          <button
            role="tab"
            aria-selected={filter === "drift"}
            className={filter === "drift" ? "on" : ""}
            onClick={() => setFilter("drift")}
          >
            Differences
          </button>
          <button
            role="tab"
            aria-selected={filter === "all"}
            className={filter === "all" ? "on" : ""}
            onClick={() => setFilter("all")}
          >
            All keys
          </button>
        </div>
      </div>

      {envs.length < 2 && (
        <div className="alert warn">Pick at least two environments.</div>
      )}
      {err && <div className="alert">{err}</div>}

      {data && (
        <div className="cmp-sum">
          <span>
            <b className="cmp-dot missing" />
            {plural(data.summary.missing, "key")} missing somewhere
          </span>
          <span>
            <b className="cmp-dot differs" />
            {data.summary.differs} with different values
          </span>
          <span>
            <b className="cmp-dot same" />
            {data.summary.same} identical
          </span>
        </div>
      )}

      {data && (
        <div className="scroll-x">
          <table className="table cmp">
            <thead>
              <tr>
                <th style={{ width: "32%" }}>Key</th>
                {data.envs.map((e: string) => (
                  <th key={e}>
                    <EnvTag slug={e} />
                  </th>
                ))}
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r: Row) => (
                <tr key={r.path + r.key}>
                  <td className="c-main mono">
                    {r.path !== "/" && <span className="dim">{r.path}/</span>}
                    {r.key}
                  </td>
                  {data.envs.map((e: string) => (
                    <td key={e}>
                      {e in r.variant ? (
                        <span
                          className={`variant v${r.variant[e]}`}
                          title={`Value ${LETTERS[r.variant[e]]}`}
                        >
                          {LETTERS[r.variant[e]] || "?"}
                        </span>
                      ) : (
                        <span className="variant none" title="Missing">
                          —
                        </span>
                      )}
                    </td>
                  ))}
                  <td>
                    <span className={`cmp-status ${r.status}`}>
                      {r.status === "missing"
                        ? "Missing"
                        : r.status === "differs"
                          ? "Differs"
                          : "Same"}
                    </span>
                  </td>
                </tr>
              ))}
              {!rows.length && (
                <EmptyRow
                  cols={cols}
                  icon="check"
                  title={
                    data.rows.length ? "No differences" : "No secrets here"
                  }
                  hint={
                    data.rows.length
                      ? `Every key under ${data.path} matches across these environments.`
                      : "Try another folder."
                  }
                />
              )}
            </tbody>
          </table>
          <TableFoot
            right={
              <span className="dim">
                Same letter = same value. Values are compared on the server and
                never sent.
              </span>
            }
          >
            {plural(rows.length, "key")}
          </TableFoot>
        </div>
      )}
    </>
  );
}
