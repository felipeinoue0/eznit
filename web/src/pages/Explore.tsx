import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import {
  downloadFile,
  fileNameOf,
  listMyTemplates,
  listSharedFiles,
  listTemplates,
  messageOf,
} from "../api";
import Icon from "../components/Icon";
import TemplateCard from "../components/TemplateCard";
import { EmptyState, ErrorState, PageHeader, Skeleton, useFailure } from "../components/ui";
import { useSession } from "../session";
import { KIND_LABELS, KINDS } from "../types";
import type { Kind, SharedFileData, TemplateData } from "../types";

// One catalogue answer, tagged with what it answers, so a slow reply for an
// old filter or an old session never lands on the page that replaced it.
interface CatalogueResult {
  key: string;
  templates: TemplateData[];
  error: string | null;
}

type Scope = "all" | "public" | "mine";

export default function Explore() {
  const { token, server } = useSession();
  const fail = useFailure();
  const [kind, setKind] = useState("");
  const [query, setQuery] = useState("");
  const [scope, setScope] = useState<Scope>("all");
  const [attempt, setAttempt] = useState(0);
  const [result, setResult] = useState<CatalogueResult | null>(null);
  const [mine, setMine] = useState<Set<string>>(new Set());
  const [shared, setShared] = useState<SharedFileData[]>([]);

  const key = `${server}|${token ?? ""}|${kind}|${attempt}`;

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const response = await listTemplates(kind);
        if (!cancelled) {
          setResult({ key, templates: response.data ?? [], error: null });
        }
      } catch (cause: unknown) {
        if (!cancelled) {
          setResult({ key, templates: [], error: messageOf(cause) });
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [key, kind]);

  // The caller's own ids mark "Yours" and feed the scope filter. A visitor
  // has none, so the request is not made.
  useEffect(() => {
    if (token === null) {
      return;
    }
    let cancelled = false;
    void (async () => {
      try {
        const response = await listMyTemplates();
        if (!cancelled) {
          setMine(new Set((response.data ?? []).map((template) => template.id)));
        }
      } catch (cause: unknown) {
        if (!cancelled) {
          fail(cause, "Could not load your templates");
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [token, server, fail]);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const response = await listSharedFiles();
        if (!cancelled) {
          setShared(response.data ?? []);
        }
      } catch {
        // The shared list is a side panel: its absence is not worth an error.
        if (!cancelled) {
          setShared([]);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [server, token]);

  const current = result !== null && result.key === key ? result : null;
  const myIds = useMemo(() => (token === null ? new Set<string>() : mine), [token, mine]);

  // Search and scope run in the browser: the API filters by kind, and a
  // keystroke is not worth a round trip.
  const visible = useMemo(() => {
    const templates = current?.templates ?? [];
    const needle = query.trim().toLowerCase();
    return templates.filter((template) => {
      if (scope === "public" && !template.is_public) {
        return false;
      }
      if (scope === "mine" && !myIds.has(template.id)) {
        return false;
      }
      if (needle === "") {
        return true;
      }
      return [template.name, template.description, template.kind, template.slug]
        .join(" ")
        .toLowerCase()
        .includes(needle);
    });
  }, [current, query, scope, myIds]);

  return (
    <div className="page">
      <PageHeader
        title="Explore templates"
        description="Project setups published on this server. Open one to read its files, then apply it on any machine."
        actions={
          token !== null ? (
            <Link to="/templates/new" className="btn btn-primary">
              <Icon name="plus" size={16} /> New template
            </Link>
          ) : (
            <Link to="/signup" className="btn btn-primary">
              Get started
            </Link>
          )
        }
      />

      {token === null ? (
        <div className="callout">
          <Icon name="info" />
          <div>
            You are browsing as a visitor: you see public templates and shared files.{" "}
            <Link to="/login">Sign in</Link> to publish your own, or{" "}
            <Link to="/local">run eznit locally</Link>.
          </div>
        </div>
      ) : null}

      <div className="toolbar">
        <label className="search">
          <Icon name="search" size={16} />
          <span className="visually-hidden">Search templates</span>
          <input
            type="search"
            value={query}
            placeholder="Search by name, description or kind"
            onChange={(event) => setQuery(event.target.value)}
          />
        </label>
        <label className="select">
          <span className="visually-hidden">Kind</span>
          <select value={kind} onChange={(event) => setKind(event.target.value)}>
            <option value="">All kinds</option>
            {KINDS.map((known) => (
              <option key={known} value={known}>
                {KIND_LABELS[known as Kind]}
              </option>
            ))}
          </select>
        </label>
        {token !== null ? (
          <fieldset className="segmented">
            <legend className="visually-hidden">Show</legend>
            {(["all", "public", "mine"] as Scope[]).map((option) => (
              <button
                key={option}
                type="button"
                aria-pressed={scope === option}
                className={scope === option ? "segment active" : "segment"}
                onClick={() => setScope(option)}
              >
                {option === "all" ? "All" : option === "public" ? "Public" : "Yours"}
              </button>
            ))}
          </fieldset>
        ) : null}
      </div>

      {current === null ? (
        <div className="card-grid">
          <Skeleton rows={6} height={180} />
        </div>
      ) : current.error !== null ? (
        <ErrorState message={current.error} onRetry={() => setAttempt((n) => n + 1)} />
      ) : visible.length === 0 ? (
        <EmptyState
          icon="layers"
          title={query !== "" || kind !== "" || scope !== "all" ? "No matches" : "No templates yet"}
          action={
            token !== null ? (
              <Link to="/templates/new" className="btn btn-primary">
                <Icon name="plus" size={16} /> Publish the first one
              </Link>
            ) : undefined
          }
        >
          {query !== "" || kind !== "" || scope !== "all"
            ? "Nothing matches these filters. Try a broader search."
            : "Publish one from this page, or from a folder with eznit template publish <folder>."}
        </EmptyState>
      ) : (
        <>
          <p className="result-count">
            {visible.length} {visible.length === 1 ? "template" : "templates"}
          </p>
          <div className="card-grid">
            {visible.map((template) => (
              <TemplateCard key={template.id} template={template} mine={myIds.has(template.id)} />
            ))}
          </div>
        </>
      )}

      {shared.length > 0 ? <SharedFiles files={shared} /> : null}
    </div>
  );
}

function SharedFiles({ files }: { files: SharedFileData[] }) {
  const fail = useFailure();

  async function download(file: SharedFileData): Promise<void> {
    try {
      await downloadFile(file);
    } catch (cause: unknown) {
      fail(cause, `Could not download ${fileNameOf(file)}`);
    }
  }

  return (
    <section className="section">
      <div className="section-head">
        <h2>Shared files</h2>
        <p className="muted">
          Single files people on this server made public. A shared file is not a template.
        </p>
      </div>
      <div className="panel list">
        {files.map((file) => (
          <div key={file.id} className="list-row">
            <span className="file-icon">
              <Icon name="file" size={16} />
            </span>
            <span className="list-main">
              <strong>{fileNameOf(file)}</strong>
              <span className="muted small">
                {file.owner_email === ""
                  ? "Shared on this server"
                  : `Shared by ${file.owner_email}`}{" "}
                · v{file.version}
              </span>
            </span>
            <button
              type="button"
              className="btn btn-secondary btn-sm"
              onClick={() => void download(file)}
              aria-label={`Download ${fileNameOf(file)}`}
            >
              <Icon name="download" size={15} /> Download
            </button>
          </div>
        ))}
      </div>
    </section>
  );
}
