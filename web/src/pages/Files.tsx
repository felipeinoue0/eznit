import { useEffect, useMemo, useRef, useState } from "react";
import type { ChangeEvent, DragEvent } from "react";
import {
  deleteFile,
  downloadFile,
  extOf,
  fileNameOf,
  formatBytes,
  listFiles,
  MAX_UPLOAD_BYTES,
  messageOf,
  refused,
  replaceFile,
  shareFile,
  uploadFile,
} from "../api";
import Icon from "../components/Icon";
import {
  EmptyState,
  ErrorState,
  PageHeader,
  Skeleton,
  Spinner,
  Switch,
  useConfirm,
  useFailure,
  useToast,
} from "../components/ui";
import { useSession } from "../session";
import type { FileData } from "../types";

export default function Files() {
  const { endSession } = useSession();
  const fail = useFailure();
  const toast = useToast();
  const confirm = useConfirm();
  const [files, setFiles] = useState<FileData[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);
  const [uploading, setUploading] = useState<{ done: number; total: number } | null>(null);
  const [pending, setPending] = useState<Set<string>>(new Set());
  const [dragging, setDragging] = useState(false);
  const [query, setQuery] = useState("");
  const uploadInput = useRef<HTMLInputElement>(null);
  const replaceInput = useRef<HTMLInputElement>(null);
  const replaceTarget = useRef<FileData | null>(null);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const response = await listFiles();
        if (!cancelled) {
          setFiles(response.data ?? []);
          setError(null);
        }
      } catch (cause: unknown) {
        if (cancelled) {
          return;
        }
        // A refusal is not a message under the page: the token is dead, and
        // leaving the table up would say the session is fine (#339).
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

  // reload re-reads the list after a change, so the page shows what the API
  // stored instead of a guess at it.
  async function reload(): Promise<void> {
    try {
      const response = await listFiles();
      setFiles(response.data ?? []);
    } catch (cause: unknown) {
      fail(cause, "Could not refresh the list");
    }
  }

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

  async function upload(list: File[]): Promise<void> {
    const accepted = list.filter((file) => {
      if (file.size > MAX_UPLOAD_BYTES) {
        toast.error(
          `${file.name} is too large`,
          `An upload may carry ${formatBytes(MAX_UPLOAD_BYTES)}.`,
        );
        return false;
      }
      return true;
    });
    if (accepted.length === 0) {
      return;
    }
    setUploading({ done: 0, total: accepted.length });
    let stored = 0;
    for (const file of accepted) {
      try {
        await uploadFile(file);
        stored += 1;
      } catch (cause: unknown) {
        if (refused(cause)) {
          endSession();
          return;
        }
        toast.error(`Could not upload ${file.name}`, messageOf(cause));
      }
      setUploading({ done: stored, total: accepted.length });
    }
    setUploading(null);
    if (stored > 0) {
      toast.success(stored === 1 ? `Uploaded ${accepted[0]?.name}` : `Uploaded ${stored} files`);
    }
    await reload();
  }

  function onPick(event: ChangeEvent<HTMLInputElement>): void {
    const input = event.currentTarget;
    const picked = input.files === null ? [] : Array.from(input.files);
    input.value = "";
    void upload(picked);
  }

  function onDrop(event: DragEvent<HTMLElement>): void {
    event.preventDefault();
    setDragging(false);
    void upload(Array.from(event.dataTransfer.files));
  }

  function startReplace(file: FileData): void {
    replaceTarget.current = file;
    replaceInput.current?.click();
  }

  async function onReplace(event: ChangeEvent<HTMLInputElement>): Promise<void> {
    const input = event.currentTarget;
    const picked = input.files?.[0];
    input.value = "";
    const target = replaceTarget.current;
    if (picked === undefined || target === null) {
      return;
    }
    if (picked.size > MAX_UPLOAD_BYTES) {
      toast.error(
        `${picked.name} is too large`,
        `An upload may carry ${formatBytes(MAX_UPLOAD_BYTES)}.`,
      );
      return;
    }
    mark(target.id, true);
    try {
      await replaceFile(target.id, picked);
      toast.success(`${fileNameOf(target)} updated`, `Now v${target.version + 1}.`);
      await reload();
    } catch (cause: unknown) {
      fail(cause, `Could not replace ${fileNameOf(target)}`);
    } finally {
      mark(target.id, false);
    }
  }

  async function onShare(file: FileData, next: boolean): Promise<void> {
    if (
      next &&
      !(await confirm({
        title: "Make this file public?",
        body: (
          <>
            Anyone on this server — signed in or not — will be able to download{" "}
            <strong>{fileNameOf(file)}</strong>.
          </>
        ),
        confirm: "Make public",
      }))
    ) {
      return;
    }
    mark(file.id, true);
    try {
      await shareFile(file.id, next);
      setFiles((all) =>
        (all ?? []).map((item) => (item.id === file.id ? { ...item, is_public: next } : item)),
      );
      toast.success(next ? "Now public" : "Now private", fileNameOf(file));
    } catch (cause: unknown) {
      fail(cause, "Could not change the visibility");
    } finally {
      mark(file.id, false);
    }
  }

  async function onDownload(file: FileData): Promise<void> {
    mark(file.id, true);
    try {
      await downloadFile(file);
    } catch (cause: unknown) {
      fail(cause, `Could not download ${fileNameOf(file)}`);
    } finally {
      mark(file.id, false);
    }
  }

  async function onDelete(file: FileData): Promise<void> {
    const sure = await confirm({
      title: "Delete this file?",
      body: (
        <>
          <strong>{fileNameOf(file)}</strong> and all of its versions will be removed from this
          server. This cannot be undone.
        </>
      ),
      confirm: "Delete file",
      danger: true,
    });
    if (!sure) {
      return;
    }
    mark(file.id, true);
    try {
      await deleteFile(file.id);
      setFiles((all) => (all ?? []).filter((item) => item.id !== file.id));
      toast.success("File deleted", fileNameOf(file));
    } catch (cause: unknown) {
      fail(cause, `Could not delete ${fileNameOf(file)}`);
    } finally {
      mark(file.id, false);
    }
  }

  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return (files ?? []).filter(
      (file) => needle === "" || fileNameOf(file).toLowerCase().includes(needle),
    );
  }, [files, query]);

  return (
    <div className="page">
      <PageHeader
        title="Files"
        description="Single files you keep on this server — dotfiles, configs, scripts. Upload a new copy to version one; make it public to share it."
        actions={
          <button
            type="button"
            className="btn btn-primary"
            onClick={() => uploadInput.current?.click()}
            disabled={uploading !== null}
          >
            {uploading !== null ? <Spinner /> : <Icon name="upload" size={16} />}
            {uploading !== null ? `Uploading ${uploading.done}/${uploading.total}` : "Upload files"}
          </button>
        }
      />

      <input
        ref={uploadInput}
        type="file"
        multiple
        hidden
        aria-label="Choose files to upload"
        onChange={onPick}
      />
      <input
        ref={replaceInput}
        type="file"
        hidden
        aria-label="Choose the new version"
        onChange={(event) => void onReplace(event)}
      />

      <button
        type="button"
        className={dragging ? "dropzone active" : "dropzone"}
        onDragOver={(event) => {
          event.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={onDrop}
        onClick={() => uploadInput.current?.click()}
      >
        <span className="dropzone-icon">
          <Icon name="upload" size={20} />
        </span>
        <span>
          <strong>Drop files here</strong> or click to choose
          <span className="muted small"> · up to {formatBytes(MAX_UPLOAD_BYTES)} each</span>
        </span>
      </button>

      {error !== null ? (
        <ErrorState message={error} onRetry={() => setAttempt((n) => n + 1)} />
      ) : files === null ? (
        <Skeleton rows={4} />
      ) : files.length === 0 ? (
        <EmptyState icon="folder" title="No files yet">
          Upload a file to keep it here and get it back from any machine with eznit download.
        </EmptyState>
      ) : (
        <>
          <div className="toolbar">
            <label className="search">
              <Icon name="search" size={16} />
              <span className="visually-hidden">Search files</span>
              <input
                type="search"
                value={query}
                placeholder="Search files"
                onChange={(event) => setQuery(event.target.value)}
              />
            </label>
            <span className="muted small">
              {files.length} {files.length === 1 ? "file" : "files"} ·{" "}
              {files.filter((file) => file.is_public).length} public
            </span>
          </div>

          <div className="panel table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th className="hide-sm">Type</th>
                  <th className="hide-sm">Version</th>
                  <th>Visibility</th>
                  <th>
                    <span className="visually-hidden">Actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {visible.map((file) => {
                  const busy = pending.has(file.id);
                  const name = fileNameOf(file);
                  return (
                    <tr key={file.id}>
                      <td>
                        <span className="file-cell row-title">
                          <span className="file-icon">
                            <Icon name="file" size={16} />
                          </span>
                          {name}
                        </span>
                      </td>
                      <td className="hide-sm">
                        <span className="badge">{extOf(file) === "" ? "—" : extOf(file)}</span>
                      </td>
                      <td className="muted hide-sm">v{file.version}</td>
                      <td>
                        <Switch
                          checked={file.is_public}
                          label={`Public: ${name}`}
                          disabled={busy}
                          onChange={(next) => void onShare(file, next)}
                        />
                      </td>
                      <td>
                        <div className="row-actions">
                          {busy ? <Spinner /> : null}
                          <button
                            type="button"
                            className="icon-btn"
                            aria-label={`Download ${name}`}
                            title="Download"
                            disabled={busy}
                            onClick={() => void onDownload(file)}
                          >
                            <Icon name="download" size={16} />
                          </button>
                          <button
                            type="button"
                            className="icon-btn"
                            aria-label={`Upload a new version of ${name}`}
                            title="Upload a new version"
                            disabled={busy}
                            onClick={() => startReplace(file)}
                          >
                            <Icon name="replace" size={16} />
                          </button>
                          <button
                            type="button"
                            className="icon-btn icon-btn-danger"
                            aria-label={`Delete ${name}`}
                            title="Delete"
                            disabled={busy}
                            onClick={() => void onDelete(file)}
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
