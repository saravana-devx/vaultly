import type { ReactNode } from "react";
import { Icon, copy } from "../../components/ui";

// A command or snippet: plain text beside an accent rule, with a copy button.
export function Cmd({ children }: { children: ReactNode }) {
  const text = String(children).trim();
  return (
    <div className="cmd">
      <pre>{text}</pre>
      <button
        className="iconbtn"
        onClick={() => copy(text, "Copied")}
        aria-label="Copy"
        title="Copy"
      >
        <Icon name="copy" size={15} />
      </button>
    </div>
  );
}

// A short callout: tip, note or warning.
export function Note({
  kind = "note",
  children,
}: {
  kind?: "note" | "tip" | "warn";
  children: ReactNode;
}) {
  const label = kind === "warn" ? "Important" : kind === "tip" ? "Tip" : "Note";
  return (
    <div className={`callout ${kind}`}>
      <b>{label}</b>
      <div>{children}</div>
    </div>
  );
}

export function Table({ head, rows }: { head: string[]; rows: ReactNode[][] }) {
  return (
    <div className="doc-table-wrap">
      <table className="doc-table">
        <thead>
          <tr>
            {head.map((h) => (
              <th key={h}>{h}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={i}>
              {r.map((c, j) => (
                <td key={j}>{c}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export interface DocPage {
  slug: string;
  title: string;
  group: string;
  summary: string;
  keywords?: string;
  body: () => ReactNode;
}
