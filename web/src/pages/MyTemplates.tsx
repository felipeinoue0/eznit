import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import {
  deleteTemplate,
  downloadTemplateArchive,
  listMyTemplates,
  messageOf,
  refused,
  relativeTime,
  setTemplateVisibility,
} from "../api";
import Icon from "../components/Icon";
import {
  EmptyState,
  ErrorState,
  KindBadge,
  PageHeader,
  Skeleton,
  Switch,
  useConfirm,
  useFailure,
  useToast,
} from "../components/ui";
import { useSession } from "../session";
import type { TemplateData } from "../types";

export default function MyTemplates() {
  const { endSession } = useSession();
  const fail = useFailure();
  const toast = useToast();
  const confirm = useConfirm();
  const [templates, setTemplates] = useState<TemplateData[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);
  const [pending, setPending] = useState<Set<string>>(new Set());
  const [query, setQuery] = useState("");

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const response = await listMyTemplates();
        if (!cancelled) {
          setTemplates(response.data ?? []);
          setError(null);
        }
      } catch (cause: unknown) {
        if (cancelled) {
          return;
        }
        if (refused(cause)) {
          endSession();
          return;
        }
        setError(messageOf(cause));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [attempt, endSession]);

  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return (templates ?? []).filter((template) =>
      needle === ""
        ? true
        : `${template.name} ${template.description} ${template.kind}`
            .toLowerCase()
            .includes(needle),
    );
  }, [templates, query]);

  const publicCount = (templates ?? []).filter((template) => template.is_public).length;

  function mark(id: string, on: boolean): void {
    setPending((all) => {
      const next = new Set(all);
      if (on) {
        next.add(id);
      } else {
        next.delete(id);
      }
      return next;
    });
  }

  async function toggle(template: TemplateData, next: boolean): Promise<void> {
    if (
      next &&
      !(await confirm({
        title: "Make this template public?",
        body: (
          <>
            Anyone who can open this server will see <strong>{template.name}</strong> and every file
            in it.
          </>
        ),
        confirm: "Make public",
      }))
    ) {
      return;
    }
    mark(template.id, true);
    try {
      const updated = await setTemplateVisibility(template.id, next);
      setTemplates((all) =>
        (all ?? []).map((item) =>
          item.id === template.id
            ? { ...item, is_public: updated.is_public, version: updated.version }
            : item,
        ),
      );
      toast.success(next ? "Now public" : "Now private", template.name);
    } catch (cause: unknown) {
      fail(cause, "Could not change the visibility");
    } finally {
      mark(template.id, false);
    }
  }

  async function remove(template: TemplateData): Promise<void> {
    const sure = await confirm({
      title: "Delete this template?",
      body: (
        <>
          <strong>{template.name}</strong> will be removed from this server. Projects it was applied
          to keep their copies.
        </>
      ),
      confirm: "Delete template",
      danger: true,
    });
    if (!sure) {
      return;
    }
    mark(template.id, true);
    try {
      await deleteTemplate(template.id);
      setTemplates((all) => (all ?? []).filter((item) => item.id !== template.id));
      toast.success("Template deleted", template.name);
    } catch (cause: unknown) {
      fail(cause, "Could not delete the template");
    } finally {
      mark(template.id, false);
    }
  }

  async function download(template: TemplateData): Promise<void> {
    try {
      await downloadTemplateArchive(template);
    } catch (cause: unknown) {
      fail(cause, "Could not download the archive");
    }
  }

  return (
    <div className="page">
      <PageHeader
        title="My templates"
        description="Everything you have published, private and public. Edit, share or retire them here."
        actions={
          <Link to="/templates/new" className="btn btn-primary">
            <Icon name="plus" size={16} /> New template
          </Link>
        }
      />

      {templates !== null && templates.length > 0 ? (
        <div className="stats">
          <Stat label="Templates" value={templates.length} />
          <Stat label="Public" value={publicCount} />
          <Stat label="Private" value={templates.length - publicCount} />
        </div>
      ) : null}

      {error !== null ? (
        <ErrorState message={error} onRetry={() => setAttempt((n) => n + 1)} />
      ) : templates === null ? (
        <Skeleton rows={4} />
      ) : templates.length === 0 ? (
        <EmptyState
          icon="layers"
          title="You have not published a template yet"
          action={
            <Link to="/templates/new" className="btn btn-primary">
              <Icon name="plus" size={16} /> Create a template
            </Link>
          }
        >
          A template is a folder of files — agent rules, scripts, configuration — that eznit writes
          into a project. Start from scratch here, or run eznit template publish &lt;folder&gt;.
        </EmptyState>
      ) : (
        <>
          <div className="toolbar">
            <label className="search">
              <Icon name="search" size={16} />
              <span className="visually-hidden">Search your templates</span>
              <input
                type="search"
                value={query}
                placeholder="Search your templates"
                onChange={(event) => setQuery(event.target.value)}
              />
            </label>
          </div>
          <div className="panel table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>Template</th>
                  <th className="hide-sm">Kind</th>
                  <th className="hide-sm">Files</th>
                  <th className="hide-sm">Updated</th>
                  <th>Visibility</th>
                  <th>
                    <span className="visually-hidden">Actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {visible.map((template) => {
                  const busy = pending.has(template.id);
                  return (
                    <tr key={template.id}>
                      <td>
                        <Link to={`/templates/${template.id}`} className="row-title">
                          {template.name}
                        </Link>
                        <div className="muted small truncate">
                          {template.description === ""
                            ? `v${template.version}`
                            : template.description}
                        </div>
                      </td>
                      <td className="hide-sm">
                        <KindBadge kind={template.kind} />
                      </td>
                      <td className="num hide-sm">{template.file_count}</td>
                      <td className="muted nowrap hide-sm">{relativeTime(template.updated_at)}</td>
                      <td>
                        <Switch
                          checked={template.is_public}
                          label={`Public: ${template.name}`}
                          disabled={busy}
                          onChange={(next) => void toggle(template, next)}
                        />
                      </td>
                      <td>
                        <div className="row-actions">
                          <button
                            type="button"
                            className="icon-btn"
                            aria-label={`Download ${template.name}`}
                            title="Download .zip"
                            onClick={() => void download(template)}
                          >
                            <Icon name="download" size={16} />
                          </button>
                          <Link
                            to={`/templates/${template.id}/edit`}
                            className="icon-btn"
                            aria-label={`Edit ${template.name}`}
                            title="Edit"
                          >
                            <Icon name="edit" size={16} />
                          </Link>
                          <button
                            type="button"
                            className="icon-btn icon-btn-danger"
                            aria-label={`Delete ${template.name}`}
                            title="Delete"
                            disabled={busy}
                            onClick={() => void remove(template)}
                          >
                            <Icon name="trash" size={16} />
                          </button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
            {visible.length === 0 ? (
              <p className="muted table-empty">Nothing matches “{query}”.</p>
            ) : null}
          </div>
        </>
      )}
    </div>
  );
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div className="stat">
      <span className="stat-value">{value}</span>
      <span className="stat-label">{label}</span>
    </div>
  );
}
