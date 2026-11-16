import { useEffect, useMemo, useState } from "react";
import type { ChangeEvent, FormEvent } from "react";
import {
  base64Size,
  createCommand,
  deleteCommand,
  formatBytes,
  listCommands,
  MAX_UPLOAD_BYTES,
  messageOf,
  readLocalFile,
  refused,
  relativeTime,
  updateCommand,
} from "../api";
import Icon from "../components/Icon";
import {
  CodeBlock,
  EmptyState,
  ErrorState,
  Modal,
  PageHeader,
  Skeleton,
  Spinner,
  useConfirm,
  useFailure,
  useToast,
} from "../components/ui";
import { useSession } from "../session";
import type { CommandData, CommandRequest } from "../types";

interface Form {
  id: string | null;
  name: string;
  kind: "text" | "binary";
  content: string;
  base64: string;
  isExecutable: boolean;
}

function blank(): Form {
  return {
    id: null,
    name: "",
    kind: "text",
    content: "#!/usr/bin/env bash\nset -euo pipefail\n\n",
    base64: "",
    isExecutable: true,
  };
}

function formOf(command: CommandData): Form {
  return {
    id: command.id,
    name: command.name,
    kind: command.kind,
    content: command.content ?? "",
    base64: command.content_base64 ?? "",
    isExecutable: command.is_executable,
  };
}

export default function Commands() {
  const { endSession } = useSession();
  const fail = useFailure();
  const toast = useToast();
  const confirm = useConfirm();
  const [commands, setCommands] = useState<CommandData[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);
  const [form, setForm] = useState<Form | null>(null);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [selected, setSelected] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const response = await listCommands();
        if (!cancelled) {
          setCommands(response.data ?? []);
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

  const sorted = useMemo(
    () => [...(commands ?? [])].sort((a, b) => a.name.localeCompare(b.name)),
    [commands],
  );
  const shown = sorted.find((command) => command.id === selected) ?? sorted[0] ?? null;

  function open(next: Form): void {
    setFormError(null);
    setForm(next);
  }

  async function onFile(event: ChangeEvent<HTMLInputElement>): Promise<void> {
    const input = event.currentTarget;
    const file = input.files?.[0];
    input.value = "";
    if (file === undefined || form === null) {
      return;
    }
    if (file.size > MAX_UPLOAD_BYTES) {
      setFormError(`${file.name} is larger than ${formatBytes(MAX_UPLOAD_BYTES)}.`);
      return;
    }
    const read = await readLocalFile(file);
    setForm({
      ...form,
      name: form.name === "" ? file.name : form.name,
      kind: read.kind,
      content: read.kind === "text" ? read.content : "",
      base64: read.kind === "binary" ? read.base64 : "",
    });
  }

  async function save(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    if (form === null) {
      return;
    }
    if (form.name.trim() === "") {
      setFormError("Give the command a name.");
      return;
    }
    const body: CommandRequest =
      form.kind === "binary"
        ? {
            name: form.name.trim(),
            kind: "binary",
            content_base64: form.base64,
            is_executable: form.isExecutable,
          }
        : {
            name: form.name.trim(),
            kind: "text",
            content: form.content,
            is_executable: form.isExecutable,
          };
    setSaving(true);
    setFormError(null);
    try {
      const saved =
        form.id === null ? await createCommand(body) : await updateCommand(form.id, body);
      toast.success(form.id === null ? "Command added" : "Command saved", saved.name);
      setForm(null);
      setSelected(saved.id);
      setAttempt((n) => n + 1);
    } catch (cause: unknown) {
      if (refused(cause)) {
        endSession();
        return;
      }
      setFormError(messageOf(cause));
    } finally {
      setSaving(false);
    }
  }

  async function remove(command: CommandData): Promise<void> {
    const sure = await confirm({
      title: "Delete this command?",
      body: (
        <>
          <strong>{command.name}</strong> will be removed from your library. A template that pulls
          it in may refuse to delete it while it is in use.
        </>
      ),
      confirm: "Delete command",
      danger: true,
    });
    if (!sure) {
      return;
    }
    try {
      await deleteCommand(command.id);
      setCommands((all) => (all ?? []).filter((item) => item.id !== command.id));
      toast.success("Command deleted", command.name);
    } catch (cause: unknown) {
      fail(cause, `Could not delete ${command.name}`);
    }
  }

  return (
    <div className="page">
      <PageHeader
        title="Commands"
        description="Your library of scripts. A template pulls a command in by reference, so fixing it here fixes every template that uses it."
        actions={
          <button type="button" className="btn btn-primary" onClick={() => open(blank())}>
            <Icon name="plus" size={16} /> New command
          </button>
        }
      />

      {error !== null ? (
        <ErrorState message={error} onRetry={() => setAttempt((n) => n + 1)} />
      ) : commands === null ? (
        <Skeleton rows={3} />
      ) : commands.length === 0 ? (
        <EmptyState
          icon="terminal"
          title="Your library is empty"
          action={
            <button type="button" className="btn btn-primary" onClick={() => open(blank())}>
              <Icon name="plus" size={16} /> Add your first command
            </button>
          }
        >
          Keep a setup script, a git hook or a helper binary here once, and add it to as many
          templates as you like.
        </EmptyState>
      ) : (
        <div className="split">
          <div className="panel list">
            {sorted.map((command) => (
              <button
                key={command.id}
                type="button"
                className={
                  shown?.id === command.id ? "list-row list-button active" : "list-row list-button"
                }
                onClick={() => setSelected(command.id)}
              >
                <span className="file-icon">
                  <Icon name="terminal" size={16} />
                </span>
                <span className="list-main">
                  <strong className="truncate">{command.name}</strong>
                  <span className="muted small">
                    {command.kind} · {formatBytes(command.size)}
                    {command.is_executable ? " · executable" : ""}
                  </span>
                </span>
              </button>
            ))}
          </div>

          {shown !== null ? (
            <section className="panel command-view">
              <header className="viewer-head">
                <div>
                  <code className="viewer-path">{shown.name}</code>
                  <div className="muted small">Updated {relativeTime(shown.updated_at)}</div>
                </div>
                <div className="row-actions">
                  <button
                    type="button"
                    className="btn btn-secondary btn-sm"
                    onClick={() => open(formOf(shown))}
                  >
                    <Icon name="edit" size={15} /> Edit
                  </button>
                  <button
                    type="button"
                    className="icon-btn icon-btn-danger"
                    aria-label={`Delete ${shown.name}`}
                    onClick={() => void remove(shown)}
                  >
                    <Icon name="trash" size={16} />
                  </button>
                </div>
              </header>
              {shown.kind === "binary" ? (
                <div className="viewer-empty">
                  <Icon name="file" size={28} className="muted" />
                  <p className="muted">A binary command · {formatBytes(shown.size)}</p>
                </div>
              ) : (
                <CodeBlock
                  code={
                    shown.content === "" || shown.content === undefined ? "(empty)" : shown.content
                  }
                />
              )}
            </section>
          ) : null}
        </div>
      )}

      <Modal
        open={form !== null}
        wide
        title={form?.id === null ? "New command" : "Edit command"}
        description="A script or binary that templates can write into a project."
        onClose={() => setForm(null)}
      >
        {form !== null ? (
          <form className="form" onSubmit={(event) => void save(event)}>
            <div className="form-grid">
              <label className="field">
                <span className="field-label">Name</span>
                <input
                  type="text"
                  value={form.name}
                  placeholder="setup.sh"
                  onChange={(event) => setForm({ ...form, name: event.target.value })}
                />
              </label>
              <div className="field">
                <span className="field-label">Content</span>
                <div className="row-actions">
                  <label className="btn btn-secondary btn-sm">
                    <Icon name="upload" size={15} /> Load from a file
                    <input type="file" hidden onChange={(event) => void onFile(event)} />
                  </label>
                  <label className="check">
                    <input
                      type="checkbox"
                      checked={form.isExecutable}
                      onChange={(event) => setForm({ ...form, isExecutable: event.target.checked })}
                    />
                    Executable
                  </label>
                </div>
              </div>
            </div>
            {form.kind === "binary" ? (
              <div className="callout">
                <Icon name="file" />
                <div>
                  Binary content · {formatBytes(base64Size(form.base64))}.{" "}
                  <button
                    type="button"
                    className="link"
                    onClick={() => setForm({ ...form, kind: "text", base64: "", content: "" })}
                  >
                    Write text instead
                  </button>
                </div>
              </div>
            ) : (
              <textarea
                className="code-editor"
                rows={14}
                spellCheck={false}
                aria-label="Script"
                value={form.content}
                onChange={(event) => setForm({ ...form, content: event.target.value })}
              />
            )}
            {formError !== null ? (
              <p className="form-error" role="alert">
                {formError}
              </p>
            ) : null}
            <div className="modal-foot inline">
              <button type="button" className="btn btn-secondary" onClick={() => setForm(null)}>
                Cancel
              </button>
              <button type="submit" className="btn btn-primary" disabled={saving}>
                {saving ? <Spinner /> : null}
                {form.id === null ? "Add command" : "Save command"}
              </button>
            </div>
          </form>
        ) : null}
      </Modal>
    </div>
  );
}
