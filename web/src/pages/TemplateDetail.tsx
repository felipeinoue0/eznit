import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import {
  base64Size,
  deleteTemplate,
  downloadTemplateArchive,
  downloadTemplateFile,
  formatBytes,
  getTemplate,
  listMyTemplates,
  messageOf,
  relativeTime,
  serverURL,
  setTemplateVisibility,
} from "../api";
import { apiAddressFor, cliTarget, envLines } from "../cli";
import FileTree, { buildTree } from "../components/FileTree";
import Icon from "../components/Icon";
import { initials } from "../components/TemplateCard";
import {
  CodeBlock,
  CopyButton,
  ErrorState,
  KindBadge,
  Skeleton,
  Spinner,
  Switch,
  VisibilityBadge,
  useConfirm,
  useFailure,
  useToast,
} from "../components/ui";
import { useSession } from "../session";
import type { TemplateDetail, TemplateFileData } from "../types";

interface DetailResult {
  template: TemplateDetail | null;
  error: string | null;
}

export default function TemplateDetailPage() {
  const { id = "" } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { token, server } = useSession();
  const fail = useFailure();
  const toast = useToast();
  const confirm = useConfirm();

  const [attempt, setAttempt] = useState(0);
  const [result, setResult] = useState<DetailResult | null>(null);
  const [owned, setOwned] = useState(false);
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [busy, setBusy] = useState<"zip" | "visibility" | "delete" | null>(null);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const template = await getTemplate(id);
        if (!cancelled) {
          setResult({ template, error: null });
          setSelectedPath((previous) => previous ?? template.files[0]?.path ?? null);
        }
      } catch (cause: unknown) {
        if (!cancelled) {
          setResult({ template: null, error: messageOf(cause) });
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [id, attempt, server, token]);

  // The detail names the owner by id, and the UI does not know its own: the
  // caller's list is what says whether this one is theirs.
  useEffect(() => {
    if (token === null) {
      return;
    }
    let cancelled = false;
    void (async () => {
      try {
        const mine = await listMyTemplates();
        if (!cancelled) {
          setOwned((mine.data ?? []).some((template) => template.id === id));
        }
      } catch {
        // Not owning is the safe default: the actions just do not show.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [id, token, server]);

  const template = result?.template ?? null;
  const isOwner = token !== null && owned;
  const tree = useMemo(() => (template === null ? [] : buildTree(template.files)), [template]);
  const selected = template?.files.find((file) => file.path === selectedPath) ?? null;

  async function downloadZip(): Promise<void> {
    if (template === null) {
      return;
    }
    setBusy("zip");
    try {
      await downloadTemplateArchive(template);
    } catch (cause: unknown) {
      fail(cause, "Could not download the archive");
    } finally {
      setBusy(null);
    }
  }

  async function changeVisibility(next: boolean): Promise<void> {
    if (template === null) {
      return;
    }
    if (
      next &&
      !(await confirm({
        title: "Make this template public?",
        body: (
          <>
            Anyone who can open this server — signed in or not — will see{" "}
            <strong>{template.name}</strong> and every file in it.
          </>
        ),
        confirm: "Make public",
      }))
    ) {
      return;
    }
    setBusy("visibility");
    try {
      const updated = await setTemplateVisibility(template.id, next);
      setResult({ template: updated, error: null });
      toast.success(next ? "Template is now public" : "Template is now private");
    } catch (cause: unknown) {
      fail(cause, "Could not change the visibility");
    } finally {
      setBusy(null);
    }
  }

  async function remove(): Promise<void> {
    if (template === null) {
      return;
    }
    const sure = await confirm({
      title: "Delete this template?",
      body: (
        <>
          <strong>{template.name}</strong> and its {template.files.length} files will be removed
          from this server. Projects it was applied to keep their copies.
        </>
      ),
      confirm: "Delete template",
      danger: true,
    });
    if (!sure) {
      return;
    }
    setBusy("delete");
    try {
      await deleteTemplate(template.id);
      toast.success("Template deleted", template.name);
      navigate("/templates");
    } catch (cause: unknown) {
      fail(cause, "Could not delete the template");
      setBusy(null);
    }
  }

  if (result === null) {
    return (
      <div className="page">
        <Skeleton rows={1} height={96} />
        <div className="detail-grid">
          <Skeleton rows={1} height={420} />
          <Skeleton rows={3} height={120} />
        </div>
      </div>
    );
  }

  if (template === null) {
    return (
      <div className="page">
        <Breadcrumb name="Not found" />
        <ErrorState
          message={result.error ?? "There is no such template."}
          onRetry={() => setAttempt((n) => n + 1)}
        />
      </div>
    );
  }

  const address = apiAddressFor(serverURL(server));
  const target = cliTarget(address);
  const variables = [...template.variables].sort((a, b) => a.position - b.position);
  const commands = template.commands ?? [];

  return (
    <div className="page">
      <Breadcrumb name={template.name} />

      <header className="detail-header">
        <div className="detail-title">
          <span className={`tile tile-lg kind-${template.kind}`} aria-hidden="true">
            {initials(template.name)}
          </span>
          <div>
            <h1>{template.name}</h1>
            <p className="page-description">
              {template.description === "" ? "No description." : template.description}
            </p>
            <div className="badge-row">
              <KindBadge kind={template.kind} />
              <VisibilityBadge isPublic={template.is_public} />
              <span className="badge">v{template.version}</span>
              {isOwner ? <span className="badge badge-mine">Yours</span> : null}
            </div>
          </div>
        </div>

        <div className="page-actions">
          <button
            type="button"
            className="btn btn-secondary"
            onClick={() => void downloadZip()}
            disabled={busy !== null}
          >
            {busy === "zip" ? <Spinner /> : <Icon name="download" size={16} />} Download .zip
          </button>
          {isOwner ? (
            <>
              <Link to={`/templates/${template.id}/edit`} className="btn btn-primary">
                <Icon name="edit" size={16} /> Edit
              </Link>
              <button
                type="button"
                className="icon-btn icon-btn-danger"
                aria-label="Delete template"
                title="Delete template"
                onClick={() => void remove()}
                disabled={busy !== null}
              >
                <Icon name="trash" />
              </button>
            </>
          ) : null}
        </div>
      </header>

      <div className="detail-grid">
        <section className="panel viewer" aria-label="Files">
          <aside className="viewer-tree">
            <div className="panel-title">
              Files <span className="count">{template.files.length}</span>
            </div>
            <FileTree
              nodes={tree}
              selected={selectedPath}
              onSelect={(file) => setSelectedPath(file.path)}
            />
          </aside>
          <div className="viewer-main">
            {selected === null ? (
              <p className="muted viewer-empty">Pick a file to read it.</p>
            ) : (
              <FileViewer templateID={template.id} file={selected} />
            )}
          </div>
        </section>

        <div className="detail-side">
          <section className="panel side-card">
            <h2 className="panel-title">
              <Icon name="rocket" size={16} /> Use this template
            </h2>
            <p className="muted small">
              With the <Link to="/local">eznit CLI</Link>, in the folder you want it in:
            </p>
            {!target.isDefault ? (
              <CodeBlock label="Point the CLI at this server" code={envLines(target, "posix")} />
            ) : null}
            <CodeBlock label="Apply" code={`eznit template apply ${template.id}`} />
            <p className="muted small">
              {variables.length > 0
                ? `It asks ${variables.length} ${variables.length === 1 ? "question" : "questions"} first.`
                : "It writes the files as they are."}{" "}
              Or download the .zip and unpack it anywhere.
            </p>
          </section>

          {isOwner ? (
            <section className="panel side-card">
              <h2 className="panel-title">Visibility</h2>
              <div className="setting-row">
                <span className="small">
                  {template.is_public
                    ? "Anyone on this server can see and apply it."
                    : "Only you can see and apply it."}
                </span>
                <Switch
                  checked={template.is_public}
                  label="Public"
                  disabled={busy !== null}
                  onChange={(next) => void changeVisibility(next)}
                />
              </div>
            </section>
          ) : null}

          <section className="panel side-card">
            <h2 className="panel-title">
              <Icon name="variable" size={16} /> Questions{" "}
              <span className="count">{variables.length}</span>
            </h2>
            {variables.length === 0 ? (
              <p className="muted small">None — applying it asks nothing.</p>
            ) : (
              <ul className="var-list">
                {variables.map((variable) => (
                  <li key={variable.key}>
                    <code>{`{{${variable.key}}}`}</code>
                    <span>{variable.prompt === "" ? variable.key : variable.prompt}</span>
                    <span className="muted small">
                      {variable.required ? "Required" : "Optional"}
                      {variable.default_value === ""
                        ? ""
                        : ` · default "${variable.default_value}"`}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </section>

          {commands.length > 0 ? (
            <section className="panel side-card">
              <h2 className="panel-title">
                <Icon name="terminal" size={16} /> Commands{" "}
                <span className="count">{commands.length}</span>
              </h2>
              <ul className="var-list">
                {commands.map((command) => (
                  <li key={command.command_id}>
                    <code>{command.target_path}</code>
                    <span className="muted small">
                      from {command.name} · {formatBytes(command.size)}
                    </span>
                  </li>
                ))}
              </ul>
            </section>
          ) : null}

          <section className="panel side-card">
            <h2 className="panel-title">Details</h2>
            <dl className="meta-list">
              <dt>ID</dt>
              <dd className="mono-row">
                <code>{template.id}</code>
                <CopyButton value={template.id} label="Copy the id" />
              </dd>
              {template.slug !== "" ? (
                <>
                  <dt>Slug</dt>
                  <dd>
                    <code>{template.slug}</code>
                  </dd>
                </>
              ) : null}
              <dt>Updated</dt>
              <dd>{relativeTime(template.updated_at)}</dd>
              <dt>Created</dt>
              <dd>{new Date(template.created_at).toLocaleDateString()}</dd>
            </dl>
          </section>
        </div>
      </div>
    </div>
  );
}

function Breadcrumb({ name }: { name: string }) {
  return (
    <nav className="breadcrumb" aria-label="Breadcrumb">
      <Link to="/">Explore</Link>
      <span aria-hidden="true">/</span>
      <span aria-current="page">{name}</span>
    </nav>
  );
}

function FileViewer({ templateID, file }: { templateID: string; file: TemplateFileData }) {
  const fail = useFailure();
  const [downloading, setDownloading] = useState(false);
  const binary = file.kind === "binary";
  const content = file.content ?? "";
  const size = binary
    ? (file.size ?? base64Size(file.content_base64 ?? ""))
    : new TextEncoder().encode(content).length;
  const lines = binary ? [] : content.split("\n");

  async function download(): Promise<void> {
    setDownloading(true);
    try {
      await downloadTemplateFile(templateID, file);
    } catch (cause: unknown) {
      fail(cause, "Could not download the file");
    } finally {
      setDownloading(false);
    }
  }

  return (
    <>
      <div className="viewer-head">
        <code className="viewer-path">{file.path}</code>
        <span className="viewer-meta">
          {file.is_executable ? <span className="badge">executable</span> : null}
          <span className="muted small">
            {binary ? formatBytes(size) : `${lines.length} lines · ${formatBytes(size)}`}
          </span>
          {binary ? (
            <button
              type="button"
              className="btn btn-secondary btn-sm"
              onClick={() => void download()}
              disabled={downloading}
            >
              {downloading ? <Spinner /> : <Icon name="download" size={15} />} Download
            </button>
          ) : (
            <CopyButton value={content} label="Copy the file" />
          )}
        </span>
      </div>
      {binary ? (
        <div className="viewer-empty">
          <Icon name="file" size={28} className="muted" />
          <p className="muted">A binary file — download it to open it.</p>
        </div>
      ) : content === "" ? (
        <p className="muted viewer-empty">This file is empty.</p>
      ) : (
        <pre className="code">
          <code>
            {lines.map((line, index) => (
              <span key={index} className="code-line">
                <span className="code-ln" aria-hidden="true">
                  {index + 1}
                </span>
                <span className="code-text">{line === "" ? " " : line}</span>
                {"\n"}
              </span>
            ))}
          </code>
        </pre>
      )}
    </>
  );
}
