import { useEffect, useMemo, useRef, useState } from "react";
import type { ChangeEvent, DragEvent, FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import {
  base64Size,
  formatBytes,
  getTemplate,
  listCommands,
  MAX_UPLOAD_BYTES,
  messageOf,
  publishTemplate,
  readLocalFile,
  refused,
  toPublishRequest,
  updateTemplate,
} from "../api";
import Icon from "../components/Icon";
import { ErrorState, Skeleton, Spinner, useFailure, useToast } from "../components/ui";
import { useSession } from "../session";
import { KIND_LABELS, KINDS } from "../types";
import type { CommandData, Kind, PublishRequest } from "../types";

// The rules the API enforces (internal/domain/templates/service.go), checked
// here first so a mistake is pointed at, not answered with a 400.
const SLUG_PATTERN = /^[a-z0-9][a-z0-9-]{0,62}$/;
const KEY_PATTERN = /^[A-Z][A-Z0-9_]{0,63}$/;
const PLACEHOLDER = /\{\{([A-Z][A-Z0-9_]{0,63})\}\}/g;
const MAX_FILES = 200;
const MAX_VARIABLES = 50;

interface EditorFile {
  uid: string;
  path: string;
  kind: "text" | "binary";
  content: string;
  base64: string;
  isExecutable: boolean;
}

interface EditorVariable {
  uid: string;
  key: string;
  prompt: string;
  defaultValue: string;
  required: boolean;
}

interface EditorCommand {
  uid: string;
  commandID: string;
  targetPath: string;
}

interface Draft {
  name: string;
  slug: string;
  description: string;
  kind: string;
  isPublic: boolean;
  files: EditorFile[];
  variables: EditorVariable[];
  commands: EditorCommand[];
}

let counter = 0;
function uid(): string {
  counter += 1;
  return `u${counter}`;
}

// A new template starts with one file that shows how a question works, so
// the first thing a person sees is the feature, not an empty box.
function starter(): Draft {
  return {
    name: "",
    slug: "",
    description: "",
    kind: "generic",
    isPublic: false,
    files: [
      {
        uid: uid(),
        path: "AGENTS.md",
        kind: "text",
        content:
          "# {{PROJECT_NAME}}\n\nInstructions for the agents working in this project.\n\n## Conventions\n\n- \n",
        base64: "",
        isExecutable: false,
      },
    ],
    variables: [
      {
        uid: uid(),
        key: "PROJECT_NAME",
        prompt: "What is the project called?",
        defaultValue: "",
        required: true,
      },
    ],
    commands: [],
  };
}

function fromRequest(request: PublishRequest): Draft {
  return {
    name: request.name,
    slug: request.slug,
    description: request.description,
    kind: request.kind,
    isPublic: request.is_public,
    files: request.files.map((file) => ({
      uid: uid(),
      path: file.path,
      kind: file.kind,
      content: file.content ?? "",
      base64: file.content_base64 ?? "",
      isExecutable: file.is_executable,
    })),
    variables: request.variables.map((variable) => ({
      uid: uid(),
      key: variable.key,
      prompt: variable.prompt,
      defaultValue: variable.default_value,
      required: variable.required,
    })),
    commands: request.commands.map((command) => ({
      uid: uid(),
      commandID: command.command_id,
      targetPath: command.target_path,
    })),
  };
}

function toRequest(draft: Draft): PublishRequest {
  return {
    name: draft.name.trim(),
    slug: draft.slug.trim(),
    description: draft.description.trim(),
    kind: draft.kind,
    is_public: draft.isPublic,
    files: draft.files.map((file) =>
      file.kind === "binary"
        ? {
            path: cleanPath(file.path),
            kind: "binary",
            content_base64: file.base64,
            is_executable: file.isExecutable,
          }
        : {
            path: cleanPath(file.path),
            kind: "text",
            content: file.content,
            is_executable: file.isExecutable,
          },
    ),
    variables: draft.variables.map((variable) => ({
      key: variable.key.trim(),
      prompt: variable.prompt.trim(),
      default_value: variable.defaultValue,
      required: variable.required,
    })),
    commands: draft.commands.map((command) => ({
      command_id: command.commandID,
      target_path: cleanPath(command.targetPath),
    })),
  };
}

function cleanPath(path: string): string {
  return path
    .trim()
    .replace(/\\/g, "/")
    .replace(/^\.\/+/, "");
}

// problems lists what the API would refuse, by field, so the form can say it
// next to the thing to fix.
function problems(draft: Draft): string[] {
  const found: string[] = [];
  if (draft.name.trim() === "") {
    found.push("Give the template a name.");
  }
  if (draft.slug.trim() !== "" && !SLUG_PATTERN.test(draft.slug.trim())) {
    found.push("The slug may hold lowercase letters, digits and dashes only.");
  }
  if (draft.files.length === 0) {
    found.push("A template needs at least one file.");
  }
  if (draft.files.length > MAX_FILES) {
    found.push(`A template holds at most ${MAX_FILES} files.`);
  }
  const paths = new Set<string>();
  for (const file of draft.files) {
    const path = cleanPath(file.path);
    if (path === "") {
      found.push("Every file needs a path.");
      break;
    }
    if (path.startsWith("/") || path.split("/").includes("..")) {
      found.push(`"${path}" must be a path inside the project (no leading / or ..).`);
    }
    if (paths.has(path)) {
      found.push(`Two files share the path "${path}".`);
    }
    paths.add(path);
  }
  const keys = new Set<string>();
  for (const variable of draft.variables) {
    const key = variable.key.trim();
    if (!KEY_PATTERN.test(key)) {
      found.push(
        `"${key || "(empty)"}" is not a variable name: use UPPER_CASE, like PROJECT_NAME.`,
      );
    } else if (keys.has(key)) {
      found.push(`The question ${key} is declared twice.`);
    }
    keys.add(key);
  }
  if (draft.variables.length > MAX_VARIABLES) {
    found.push(`A template asks at most ${MAX_VARIABLES} questions.`);
  }
  for (const command of draft.commands) {
    if (command.commandID === "" || cleanPath(command.targetPath) === "") {
      found.push("Every command needs a script and a path.");
      break;
    }
    if (paths.has(cleanPath(command.targetPath))) {
      found.push(`"${command.targetPath}" is both a file and a command path.`);
    }
  }
  return found;
}

export default function TemplateEditor({ id }: { id?: string }) {
  const navigate = useNavigate();
  const { endSession } = useSession();
  const fail = useFailure();
  const toast = useToast();
  const editing = id !== undefined;

  const [draft, setDraft] = useState<Draft | null>(editing ? null : starter);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [selected, setSelected] = useState<string | null>(null);
  const [library, setLibrary] = useState<CommandData[]>([]);
  const [saving, setSaving] = useState(false);
  const [showProblems, setShowProblems] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [dragging, setDragging] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);
  const folderInput = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!editing) {
      return;
    }
    let cancelled = false;
    void (async () => {
      try {
        const template = await getTemplate(id);
        if (!cancelled) {
          setDraft(fromRequest(toPublishRequest(template)));
        }
      } catch (cause: unknown) {
        if (cancelled) {
          return;
        }
        if (refused(cause)) {
          endSession();
          return;
        }
        setLoadError(messageOf(cause));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [editing, id, endSession]);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const response = await listCommands();
        if (!cancelled) {
          setLibrary(response.data ?? []);
        }
      } catch {
        // The library is optional: without it, the commands section is empty.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  // A reader who closes the tab with unsaved work is asked first.
  useEffect(() => {
    if (!dirty) {
      return;
    }
    const warn = (event: BeforeUnloadEvent): void => {
      event.preventDefault();
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);

  const issues = useMemo(() => (draft === null ? [] : problems(draft)), [draft]);

  // Placeholders used in a text file that no question declares: applying the
  // template would leave them in the files as typed.
  const undeclared = useMemo(() => {
    if (draft === null) {
      return [];
    }
    const declared = new Set(draft.variables.map((variable) => variable.key.trim()));
    const used = new Set<string>();
    for (const file of draft.files) {
      if (file.kind === "text") {
        for (const match of file.content.matchAll(PLACEHOLDER)) {
          used.add(match[1]);
        }
      }
    }
    return [...used].filter((key) => !declared.has(key)).sort();
  }, [draft]);

  if (loadError !== null) {
    return (
      <div className="page">
        <ErrorState message={loadError} />
      </div>
    );
  }

  if (draft === null) {
    return (
      <div className="page">
        <Skeleton rows={1} height={80} />
        <Skeleton rows={2} height={260} />
      </div>
    );
  }

  const current = draft;
  const selectedFile =
    current.files.find((file) => file.uid === selected) ?? current.files[0] ?? null;

  function update(change: Partial<Draft>): void {
    setDraft((previous) => (previous === null ? previous : { ...previous, ...change }));
    setDirty(true);
  }

  function updateFile(key: string, change: Partial<EditorFile>): void {
    update({
      files: current.files.map((file) => (file.uid === key ? { ...file, ...change } : file)),
    });
  }

  function addTextFile(): void {
    const base = "new-file.md";
    let path = base;
    let n = 1;
    while (current.files.some((file) => file.path === path)) {
      n += 1;
      path = `new-file-${n}.md`;
    }
    const file: EditorFile = {
      uid: uid(),
      path,
      kind: "text",
      content: "",
      base64: "",
      isExecutable: false,
    };
    update({ files: [...current.files, file] });
    setSelected(file.uid);
  }

  function removeFile(key: string): void {
    update({ files: current.files.filter((file) => file.uid !== key) });
    if (selected === key) {
      setSelected(null);
    }
  }

  // addLocalFiles reads files a person picked or dropped. A folder upload
  // keeps the paths inside the folder, without the folder's own name.
  async function addLocalFiles(list: FileList | File[], fromFolder: boolean): Promise<void> {
    const picked = Array.from(list);
    if (picked.length === 0) {
      return;
    }
    const added: EditorFile[] = [];
    for (const file of picked) {
      if (file.size > MAX_UPLOAD_BYTES) {
        toast.error(`${file.name} is too large`, "A template is sent in one request of 32 MB.");
        continue;
      }
      const relative = (file as File & { webkitRelativePath?: string }).webkitRelativePath ?? "";
      const path =
        fromFolder && relative.includes("/") ? relative.split("/").slice(1).join("/") : file.name;
      const read = await readLocalFile(file);
      added.push({
        uid: uid(),
        path,
        kind: read.kind,
        content: read.kind === "text" ? read.content : "",
        base64: read.kind === "binary" ? read.base64 : "",
        // A shell script is usually meant to run.
        isExecutable: /\.(sh|bash|zsh|fish|ps1)$/i.test(path),
      });
    }
    // A file with the same path replaces the old one: re-uploading a fixed
    // copy is the common case, not a duplicate.
    const replaced = new Set(added.map((file) => file.path));
    update({ files: [...current.files.filter((file) => !replaced.has(file.path)), ...added] });
    if (added[0] !== undefined) {
      setSelected(added[0].uid);
    }
    toast.success(added.length === 1 ? `Added ${added[0]?.path}` : `Added ${added.length} files`);
  }

  function onPick(event: ChangeEvent<HTMLInputElement>, fromFolder: boolean): void {
    const input = event.currentTarget;
    const files = input.files === null ? [] : Array.from(input.files);
    input.value = "";
    void addLocalFiles(files, fromFolder);
  }

  function onDrop(event: DragEvent<HTMLDivElement>): void {
    event.preventDefault();
    setDragging(false);
    void addLocalFiles(event.dataTransfer.files, false);
  }

  function addVariable(key = ""): void {
    update({
      variables: [
        ...current.variables,
        { uid: uid(), key, prompt: "", defaultValue: "", required: false },
      ],
    });
  }

  function updateVariable(key: string, change: Partial<EditorVariable>): void {
    update({
      variables: current.variables.map((variable) =>
        variable.uid === key ? { ...variable, ...change } : variable,
      ),
    });
  }

  function addCommand(): void {
    const first = library[0];
    update({
      commands: [
        ...current.commands,
        {
          uid: uid(),
          commandID: first?.id ?? "",
          targetPath: first === undefined ? "" : `scripts/${first.name}`,
        },
      ],
    });
  }

  async function save(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    if (issues.length > 0) {
      setShowProblems(true);
      return;
    }
    const body = toRequest(current);
    const bytes = new TextEncoder().encode(JSON.stringify(body)).length;
    if (bytes > MAX_UPLOAD_BYTES) {
      toast.error(
        "The template is too large",
        `It is ${formatBytes(bytes)}; the server takes ${formatBytes(MAX_UPLOAD_BYTES)} at once.`,
      );
      return;
    }
    setSaving(true);
    try {
      const saved = editing ? await updateTemplate(id, body) : await publishTemplate(body);
      setDirty(false);
      toast.success(
        editing ? "Template saved" : "Template published",
        `${saved.name} · v${saved.version}`,
      );
      navigate(`/templates/${saved.id}`);
    } catch (cause: unknown) {
      fail(cause, editing ? "Could not save the template" : "Could not publish the template");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="page page-editor">
      <nav className="breadcrumb" aria-label="Breadcrumb">
        <Link to="/templates">My templates</Link>
        <span aria-hidden="true">/</span>
        <span aria-current="page">{editing ? `Edit ${current.name}` : "New template"}</span>
      </nav>

      <form onSubmit={(event) => void save(event)} noValidate>
        <header className="page-header">
          <div className="page-header-text">
            <h1>{editing ? "Edit template" : "New template"}</h1>
            <p className="page-description">
              {editing
                ? "Saving replaces the template and raises its version by one."
                : "Name it, add the files a project needs, and ask for what changes from project to project."}
            </p>
          </div>
        </header>

        {showProblems && issues.length > 0 ? (
          <div className="callout callout-error" role="alert">
            <Icon name="alert" />
            <div>
              <strong>Fix these first:</strong>
              <ul>
                {issues.map((issue) => (
                  <li key={issue}>{issue}</li>
                ))}
              </ul>
            </div>
          </div>
        ) : null}

        <section className="panel form-section">
          <div className="form-section-head">
            <h2>Details</h2>
            <p className="muted small">How the template shows up in the catalogue.</p>
          </div>
          <div className="form-grid">
            <label className="field">
              <span className="field-label">Name</span>
              <input
                type="text"
                value={current.name}
                maxLength={200}
                placeholder="Go service starter"
                required
                onChange={(event) => update({ name: event.target.value })}
              />
            </label>
            <label className="field">
              <span className="field-label">
                Slug <span className="muted">(optional)</span>
              </span>
              <input
                type="text"
                value={current.slug}
                placeholder="go-service-starter"
                aria-invalid={current.slug !== "" && !SLUG_PATTERN.test(current.slug)}
                onChange={(event) => update({ slug: event.target.value.toLowerCase() })}
              />
              <span className="field-hint">
                Lowercase, digits and dashes. Derived from the name when empty.
              </span>
            </label>
            <label className="field field-wide">
              <span className="field-label">Description</span>
              <textarea
                rows={2}
                maxLength={1000}
                value={current.description}
                placeholder="What a project gets when it applies this template."
                onChange={(event) => update({ description: event.target.value })}
              />
            </label>
            <label className="field">
              <span className="field-label">Kind</span>
              <select
                value={current.kind}
                onChange={(event) => update({ kind: event.target.value })}
              >
                {KINDS.map((kind) => (
                  <option key={kind} value={kind}>
                    {KIND_LABELS[kind as Kind]}
                  </option>
                ))}
              </select>
            </label>
            <fieldset className="field visibility-field">
              <legend className="field-label">Visibility</legend>
              <div className="choice-cards">
                <label className={!current.isPublic ? "choice active" : "choice"}>
                  <input
                    type="radio"
                    name="visibility"
                    checked={!current.isPublic}
                    onChange={() => update({ isPublic: false })}
                  />
                  <Icon name="lock" size={18} />
                  <span>
                    <strong>Private</strong>
                    <span className="muted small">Only you</span>
                  </span>
                </label>
                <label className={current.isPublic ? "choice active" : "choice"}>
                  <input
                    type="radio"
                    name="visibility"
                    checked={current.isPublic}
                    onChange={() => update({ isPublic: true })}
                  />
                  <Icon name="globe" size={18} />
                  <span>
                    <strong>Public</strong>
                    <span className="muted small">Everyone on this server</span>
                  </span>
                </label>
              </div>
            </fieldset>
          </div>
        </section>

        <section className="panel form-section">
          <div className="form-section-head with-actions">
            <div>
              <h2>
                Files <span className="count">{current.files.length}</span>
              </h2>
              <p className="muted small">
                Paths are relative to the project root. Use <code>{"{{NAME}}"}</code> in a text file
                for a value asked when the template is applied.
              </p>
            </div>
            <div className="row-actions">
              <button type="button" className="btn btn-secondary btn-sm" onClick={addTextFile}>
                <Icon name="plus" size={15} /> New file
              </button>
              <button
                type="button"
                className="btn btn-secondary btn-sm"
                onClick={() => fileInput.current?.click()}
              >
                <Icon name="upload" size={15} /> Upload files
              </button>
              <button
                type="button"
                className="btn btn-secondary btn-sm"
                onClick={() => folderInput.current?.click()}
              >
                <Icon name="folder" size={15} /> Upload folder
              </button>
              <input
                ref={fileInput}
                type="file"
                multiple
                hidden
                aria-label="Choose files to add"
                onChange={(event) => onPick(event, false)}
              />
              <input
                ref={folderInput}
                type="file"
                multiple
                hidden
                aria-label="Choose a folder to add"
                {...{ webkitdirectory: "", directory: "" }}
                onChange={(event) => onPick(event, true)}
              />
            </div>
          </div>

          <div
            className={dragging ? "file-editor dropping" : "file-editor"}
            onDragOver={(event) => {
              event.preventDefault();
              setDragging(true);
            }}
            onDragLeave={() => setDragging(false)}
            onDrop={onDrop}
          >
            <ul className="file-list" aria-label="Files in the template">
              {current.files.map((file) => (
                <li key={file.uid}>
                  <button
                    type="button"
                    className={selectedFile?.uid === file.uid ? "file-item active" : "file-item"}
                    onClick={() => setSelected(file.uid)}
                  >
                    <Icon name={file.kind === "binary" ? "file" : "fileCode"} size={15} />
                    <span className="truncate">{file.path === "" ? "(no path)" : file.path}</span>
                  </button>
                </li>
              ))}
              {current.files.length === 0 ? (
                <li className="muted small file-list-empty">Drop files here, or add one.</li>
              ) : null}
            </ul>

            <div className="file-pane">
              {selectedFile === null ? (
                <div className="viewer-empty">
                  <Icon name="upload" size={28} className="muted" />
                  <p className="muted">Drop files here, or use the buttons above.</p>
                </div>
              ) : (
                <>
                  <div className="file-pane-head">
                    <label className="field grow">
                      <span className="visually-hidden">Path</span>
                      <input
                        type="text"
                        className="mono"
                        value={selectedFile.path}
                        placeholder="path/inside/the/project.md"
                        onChange={(event) =>
                          updateFile(selectedFile.uid, { path: event.target.value })
                        }
                      />
                    </label>
                    <label className="check">
                      <input
                        type="checkbox"
                        checked={selectedFile.isExecutable}
                        onChange={(event) =>
                          updateFile(selectedFile.uid, { isExecutable: event.target.checked })
                        }
                      />
                      Executable
                    </label>
                    <button
                      type="button"
                      className="icon-btn icon-btn-danger"
                      aria-label={`Remove ${selectedFile.path}`}
                      title="Remove file"
                      onClick={() => removeFile(selectedFile.uid)}
                    >
                      <Icon name="trash" size={16} />
                    </button>
                  </div>
                  {selectedFile.kind === "binary" ? (
                    <div className="viewer-empty">
                      <Icon name="file" size={28} className="muted" />
                      <p className="muted">
                        Binary file · {formatBytes(base64Size(selectedFile.base64))}. It is stored
                        as uploaded.
                      </p>
                    </div>
                  ) : (
                    <textarea
                      className="code-editor"
                      spellCheck={false}
                      value={selectedFile.content}
                      aria-label={`Content of ${selectedFile.path}`}
                      onChange={(event) =>
                        updateFile(selectedFile.uid, { content: event.target.value })
                      }
                    />
                  )}
                </>
              )}
            </div>
            {dragging ? <div className="drop-overlay">Drop to add to the template</div> : null}
          </div>
        </section>

        <section className="panel form-section">
          <div className="form-section-head with-actions">
            <div>
              <h2>
                Questions <span className="count">{current.variables.length}</span>
              </h2>
              <p className="muted small">
                Asked when the template is applied; each answer replaces its{" "}
                <code>{"{{KEY}}"}</code> in the files.
              </p>
            </div>
            <button
              type="button"
              className="btn btn-secondary btn-sm"
              onClick={() => addVariable()}
            >
              <Icon name="plus" size={15} /> Add question
            </button>
          </div>

          {undeclared.length > 0 ? (
            <div className="callout callout-warn">
              <Icon name="variable" />
              <div>
                Your files use placeholders no question declares:{" "}
                {undeclared.map((key) => (
                  <button
                    key={key}
                    type="button"
                    className="chip"
                    onClick={() => addVariable(key)}
                    title={`Add a question for ${key}`}
                  >
                    <Icon name="plus" size={12} /> {key}
                  </button>
                ))}
              </div>
            </div>
          ) : null}

          {current.variables.length === 0 ? (
            <p className="muted small">No questions: the files are written as they are.</p>
          ) : (
            <div className="var-editor">
              <div className="var-editor-head" aria-hidden="true">
                <span>Key</span>
                <span>Question</span>
                <span>Default</span>
                <span>Required</span>
                <span />
              </div>
              {current.variables.map((variable) => (
                <div key={variable.uid} className="var-editor-row">
                  <input
                    type="text"
                    className="mono"
                    aria-label="Key"
                    placeholder="PROJECT_NAME"
                    value={variable.key}
                    aria-invalid={variable.key !== "" && !KEY_PATTERN.test(variable.key)}
                    onChange={(event) =>
                      updateVariable(variable.uid, {
                        key: event.target.value.toUpperCase().replace(/[^A-Z0-9_]/g, "_"),
                      })
                    }
                  />
                  <input
                    type="text"
                    aria-label="Question"
                    placeholder="What is the project called?"
                    value={variable.prompt}
                    onChange={(event) =>
                      updateVariable(variable.uid, { prompt: event.target.value })
                    }
                  />
                  <input
                    type="text"
                    aria-label="Default"
                    placeholder="—"
                    value={variable.defaultValue}
                    onChange={(event) =>
                      updateVariable(variable.uid, { defaultValue: event.target.value })
                    }
                  />
                  <label className="check">
                    <input
                      type="checkbox"
                      checked={variable.required}
                      onChange={(event) =>
                        updateVariable(variable.uid, { required: event.target.checked })
                      }
                    />
                    <span className="visually-hidden">Required</span>
                  </label>
                  <button
                    type="button"
                    className="icon-btn icon-btn-danger"
                    aria-label={`Remove ${variable.key || "question"}`}
                    onClick={() =>
                      update({
                        variables: current.variables.filter((item) => item.uid !== variable.uid),
                      })
                    }
                  >
                    <Icon name="x" size={16} />
                  </button>
                </div>
              ))}
            </div>
          )}
        </section>

        <section className="panel form-section">
          <div className="form-section-head with-actions">
            <div>
              <h2>
                Commands <span className="count">{current.commands.length}</span>
              </h2>
              <p className="muted small">
                Scripts from your <Link to="/commands">command library</Link>, written into the
                project at the path you choose. Edit one there and every template picks it up.
              </p>
            </div>
            <button
              type="button"
              className="btn btn-secondary btn-sm"
              onClick={addCommand}
              disabled={library.length === 0}
              title={library.length === 0 ? "Your command library is empty" : undefined}
            >
              <Icon name="plus" size={15} /> Add command
            </button>
          </div>
          {current.commands.length === 0 ? (
            <p className="muted small">
              {library.length === 0
                ? "Your library is empty. Add a script on the Commands page first."
                : "No commands in this template."}
            </p>
          ) : (
            <div className="cmd-editor">
              {current.commands.map((command) => (
                <div key={command.uid} className="cmd-editor-row">
                  <select
                    aria-label="Command"
                    value={command.commandID}
                    onChange={(event) =>
                      update({
                        commands: current.commands.map((item) =>
                          item.uid === command.uid
                            ? { ...item, commandID: event.target.value }
                            : item,
                        ),
                      })
                    }
                  >
                    {command.commandID !== "" &&
                    !library.some((item) => item.id === command.commandID) ? (
                      <option value={command.commandID}>(a command you no longer have)</option>
                    ) : null}
                    {library.map((item) => (
                      <option key={item.id} value={item.id}>
                        {item.name}
                      </option>
                    ))}
                  </select>
                  <Icon name="arrowRight" size={16} className="muted" />
                  <input
                    type="text"
                    className="mono"
                    aria-label="Path in the project"
                    placeholder="scripts/setup.sh"
                    value={command.targetPath}
                    onChange={(event) =>
                      update({
                        commands: current.commands.map((item) =>
                          item.uid === command.uid
                            ? { ...item, targetPath: event.target.value }
                            : item,
                        ),
                      })
                    }
                  />
                  <button
                    type="button"
                    className="icon-btn icon-btn-danger"
                    aria-label="Remove command"
                    onClick={() =>
                      update({
                        commands: current.commands.filter((item) => item.uid !== command.uid),
                      })
                    }
                  >
                    <Icon name="x" size={16} />
                  </button>
                </div>
              ))}
            </div>
          )}
        </section>

        <div className="savebar">
          <span className="muted small">
            {dirty ? "Unsaved changes" : editing ? "No changes yet" : "Not published yet"}
            {issues.length > 0 && showProblems ? ` · ${issues.length} to fix` : ""}
          </span>
          <div className="row-actions">
            <Link to={editing ? `/templates/${id}` : "/templates"} className="btn btn-ghost">
              Cancel
            </Link>
            <button type="submit" className="btn btn-primary" disabled={saving}>
              {saving ? <Spinner /> : <Icon name="check" size={16} />}
              {editing ? "Save changes" : "Publish template"}
            </button>
          </div>
        </div>
      </form>
    </div>
  );
}
