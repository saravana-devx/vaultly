import { useEffect, useMemo, useRef, useState } from "react";
import { Link, Navigate, useParams } from "react-router-dom";
import { useAuth } from "../auth";
import { Icon, Logo, ThemeToggle } from "../components/ui";
import { PAGES } from "./docs/pages";

const GITHUB = "https://github.com/saravana-devx/vaultly";
const GROUPS = [
  "Getting started",
  "Concepts",
  "Guides",
  "Reference",
  "Operations",
];

const slugify = (t: string) =>
  t
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "");

export default function Docs() {
  const { user } = useAuth();
  const { slug = "introduction" } = useParams();
  const idx = PAGES.findIndex((p) => p.slug === slug);
  const page = PAGES[idx];
  const [q, setQ] = useState("");
  const [toc, setToc] = useState<{ id: string; text: string }[]>([]);
  const [active, setActive] = useState("");
  const article = useRef<HTMLElement>(null);

  // Opening a page starts at the top; headings get ids so the "on this page" links can jump to them.
  useEffect(() => {
    window.scrollTo(0, 0);
    document.title = page ? `${page.title} · Vaultly docs` : "Vaultly docs";
    const heads = [
      ...(article.current?.querySelectorAll("h2") || []),
    ] as HTMLElement[];
    heads.forEach((h) => {
      h.id = slugify(h.textContent || "");
    });
    setToc(heads.map((h) => ({ id: h.id, text: h.textContent || "" })));
    setActive(heads[0]?.id || "");
    if (!heads.length) return undefined;
    const seen = new Map<string, boolean>();
    const io = new IntersectionObserver(
      (entries) => {
        entries.forEach((e) => seen.set(e.target.id, e.isIntersecting));
        const first = heads.find((h) => seen.get(h.id));
        if (first) setActive(first.id);
      },
      { rootMargin: "-80px 0px -65% 0px" },
    );
    heads.forEach((h) => io.observe(h));
    return () => io.disconnect();
  }, [slug, page]);

  const results = useMemo(() => {
    const t = q.trim().toLowerCase();
    if (!t) return null;
    return PAGES.filter((p) =>
      `${p.title} ${p.summary} ${p.keywords || ""}`.toLowerCase().includes(t),
    );
  }, [q]);

  if (!page) return <Navigate to="/docs/introduction" replace />;
  const prev = PAGES[idx - 1],
    next = PAGES[idx + 1];

  return (
    <div className="lp docs">
      <header className="lp-nav scrolled">
        <div className="lp-wrap">
          <Link to="/">
            <Logo />
          </Link>
          <nav className="lp-links">
            <Link to="/">Home</Link>
            <Link to="/docs">Docs</Link>
            <a href={GITHUB} target="_blank" rel="noreferrer">
              GitHub
            </a>
          </nav>
          <div className="right">
            <ThemeToggle />
            {user ? (
              <Link className="btn primary sm" to="/dashboard">
                Open dashboard
              </Link>
            ) : (
              <>
                <Link className="btn ghost sm" to="/login">
                  Sign in
                </Link>
                <Link className="btn primary sm" to="/register">
                  Get started
                </Link>
              </>
            )}
          </div>
        </div>
      </header>

      <div className="lp-wrap docs-grid">
        <aside className="docs-nav" aria-label="Documentation">
          <div className="docs-search">
            <Icon name="search" size={14} />
            <input
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="Search docs"
              aria-label="Search docs"
            />
          </div>
          {results ? (
            <div className="docs-group">
              <span className="docs-label">
                {results.length ? "Results" : "No matches"}
              </span>
              {results.map((p) => (
                <Link
                  key={p.slug}
                  to={`/docs/${p.slug}`}
                  onClick={() => setQ("")}
                  className={p.slug === slug ? "on" : ""}
                >
                  {p.title}
                </Link>
              ))}
            </div>
          ) : (
            GROUPS.map((g) => (
              <div className="docs-group" key={g}>
                <span className="docs-label">{g}</span>
                {PAGES.filter((p) => p.group === g).map((p) => (
                  <Link
                    key={p.slug}
                    to={`/docs/${p.slug}`}
                    className={p.slug === slug ? "on" : ""}
                  >
                    {p.title}
                  </Link>
                ))}
              </div>
            ))
          )}
        </aside>

        <main className="docs-main">
          <article ref={article} key={slug} className="docs-article">
            <div className="docs-crumb">{page.group}</div>
            <h1>{page.title}</h1>
            <p className="lead">{page.summary}</p>
            {page.body()}
          </article>

          <nav className="docs-pager" aria-label="Previous and next page">
            {prev ? (
              <Link to={`/docs/${prev.slug}`}>
                <small>Previous</small>
                <b>{prev.title}</b>
              </Link>
            ) : (
              <span />
            )}
            {next ? (
              <Link className="next" to={`/docs/${next.slug}`}>
                <small>Next</small>
                <b>{next.title}</b>
              </Link>
            ) : (
              <span />
            )}
          </nav>

          <footer className="docs-foot">
            <span>© {new Date().getFullYear()} Vaultly</span>
            <span>
              <Link to="/">Home</Link> ·{" "}
              <a href={GITHUB} target="_blank" rel="noreferrer">
                Source on GitHub
              </a>
            </span>
          </footer>
        </main>

        <aside className="docs-toc" aria-label="On this page">
          {toc.length > 0 && (
            <>
              <span className="docs-label">On this page</span>
              {toc.map((t) => (
                <a
                  key={t.id}
                  href={`#${t.id}`}
                  className={active === t.id ? "on" : ""}
                  onClick={(e) => {
                    e.preventDefault();
                    document
                      .getElementById(t.id)
                      ?.scrollIntoView({ behavior: "smooth", block: "start" });
                    setActive(t.id);
                  }}
                >
                  {t.text}
                </a>
              ))}
            </>
          )}
        </aside>
      </div>
    </div>
  );
}
