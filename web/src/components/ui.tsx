import { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { messageOf, refused } from "../api";
import { useSession } from "../session";
import Icon from "./Icon";
import type { IconName } from "./Icon";

// ── Toasts ──────────────────────────────────────────────────────────────────

type ToastTone = "success" | "error" | "info";

interface Toast {
  id: number;
  tone: ToastTone;
  title: string;
  detail?: string;
}

interface ToastApi {
  show: (tone: ToastTone, title: string, detail?: string) => void;
  success: (title: string, detail?: string) => void;
  error: (title: string, detail?: string) => void;
  info: (title: string, detail?: string) => void;
}

const ToastContext = createContext<ToastApi | null>(null);

const toastIcons: Record<ToastTone, IconName> = {
  success: "check",
  error: "alert",
  info: "info",
};

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const next = useRef(0);

  const dismiss = useCallback((id: number) => {
    setToasts((all) => all.filter((toast) => toast.id !== id));
  }, []);

  const show = useCallback(
    (tone: ToastTone, title: string, detail?: string) => {
      next.current += 1;
      const id = next.current;
      setToasts((all) => [...all.slice(-3), { id, tone, title, detail }]);
      // An error stays longer: it is the one a person needs to read.
      window.setTimeout(() => dismiss(id), tone === "error" ? 8000 : 4000);
    },
    [dismiss],
  );

  const [toastApi] = useState<ToastApi>(() => ({
    show: (...args) => show(...args),
    success: (title, detail) => show("success", title, detail),
    error: (title, detail) => show("error", title, detail),
    info: (title, detail) => show("info", title, detail),
  }));

  return (
    <ToastContext.Provider value={toastApi}>
      {children}
      <div className="toasts" aria-live="polite">
        {toasts.map((toast) => (
          <div key={toast.id} className={`toast toast-${toast.tone}`}>
            <Icon name={toastIcons[toast.tone]} className="toast-icon" />
            <div className="toast-text">
              <strong>{toast.title}</strong>
              {toast.detail !== undefined ? <span>{toast.detail}</span> : null}
            </div>
            <button
              type="button"
              className="icon-btn"
              aria-label="Dismiss"
              onClick={() => dismiss(toast.id)}
            >
              <Icon name="x" size={16} />
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastApi {
  const toast = useContext(ToastContext);
  if (toast === null) {
    throw new Error("useToast outside ToastProvider");
  }
  return toast;
}

// useFailure is how a page reports a failed action: a refused token ends the
// session (the page cannot fix that, #339), anything else is a toast.
export function useFailure(): (cause: unknown, title?: string) => void {
  const { endSession } = useSession();
  const toast = useToast();
  return useCallback(
    (cause: unknown, title = "Something went wrong") => {
      if (refused(cause)) {
        endSession();
        return;
      }
      toast.error(title, messageOf(cause));
    },
    [endSession, toast],
  );
}

// ── Dialogs ─────────────────────────────────────────────────────────────────

interface ModalProps {
  open: boolean;
  title: string;
  description?: ReactNode;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
  wide?: boolean;
}

// Modal is a native <dialog>: focus is trapped, Escape closes it and the page
// behind is inert, without a library.
export function Modal({ open, title, description, onClose, children, footer, wide }: ModalProps) {
  const ref = useRef<HTMLDialogElement>(null);

  // A click on the backdrop lands on the dialog element itself. Listened for
  // natively: a click handler on <dialog> in JSX reads as a click on a
  // non-interactive element, and Escape is already handled by onCancel.
  const closeRef = useRef(onClose);
  useEffect(() => {
    closeRef.current = onClose;
  });
  useEffect(() => {
    const dialog = ref.current;
    if (dialog === null) {
      return;
    }
    const onBackdrop = (event: MouseEvent): void => {
      if (event.target === dialog) {
        closeRef.current();
      }
    };
    dialog.addEventListener("click", onBackdrop);
    return () => dialog.removeEventListener("click", onBackdrop);
  }, []);

  useEffect(() => {
    const dialog = ref.current;
    if (dialog === null) {
      return;
    }
    if (open && !dialog.open) {
      dialog.showModal();
    } else if (!open && dialog.open) {
      dialog.close();
    }
  }, [open]);

  return (
    <dialog
      ref={ref}
      className={wide === true ? "modal modal-wide" : "modal"}
      onCancel={(event) => {
        event.preventDefault();
        onClose();
      }}
    >
      {open ? (
        <div className="modal-body">
          <header className="modal-head">
            <div>
              <h2>{title}</h2>
              {description !== undefined ? <p className="muted">{description}</p> : null}
            </div>
            <button type="button" className="icon-btn" aria-label="Close" onClick={onClose}>
              <Icon name="x" />
            </button>
          </header>
          <div className="modal-content">{children}</div>
          {footer !== undefined ? <footer className="modal-foot">{footer}</footer> : null}
        </div>
      ) : null}
    </dialog>
  );
}

interface ConfirmOptions {
  title: string;
  body: ReactNode;
  confirm: string;
  danger?: boolean;
}

type ConfirmFn = (options: ConfirmOptions) => Promise<boolean>;

const ConfirmContext = createContext<ConfirmFn | null>(null);

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [pending, setPending] = useState<
    (ConfirmOptions & { resolve: (answer: boolean) => void }) | null
  >(null);

  const confirm = useCallback<ConfirmFn>(
    (options) => new Promise<boolean>((resolve) => setPending({ ...options, resolve })),
    [],
  );

  function answer(value: boolean): void {
    pending?.resolve(value);
    setPending(null);
  }

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      <Modal
        open={pending !== null}
        title={pending?.title ?? ""}
        onClose={() => answer(false)}
        footer={
          <>
            <button type="button" className="btn btn-secondary" onClick={() => answer(false)}>
              Cancel
            </button>
            <button
              type="button"
              className={pending?.danger === true ? "btn btn-danger" : "btn btn-primary"}
              onClick={() => answer(true)}
            >
              {pending?.confirm}
            </button>
          </>
        }
      >
        <div className="confirm-body">{pending?.body}</div>
      </Modal>
    </ConfirmContext.Provider>
  );
}

export function useConfirm(): ConfirmFn {
  const confirm = useContext(ConfirmContext);
  if (confirm === null) {
    throw new Error("useConfirm outside ConfirmProvider");
  }
  return confirm;
}

// ── Small pieces ────────────────────────────────────────────────────────────

interface PageHeaderProps {
  title: string;
  description?: ReactNode;
  actions?: ReactNode;
  eyebrow?: ReactNode;
}

export function PageHeader({ title, description, actions, eyebrow }: PageHeaderProps) {
  return (
    <header className="page-header">
      <div className="page-header-text">
        {eyebrow !== undefined ? <div className="eyebrow">{eyebrow}</div> : null}
        <h1>{title}</h1>
        {description !== undefined ? <p className="page-description">{description}</p> : null}
      </div>
      {actions !== undefined ? <div className="page-actions">{actions}</div> : null}
    </header>
  );
}

interface EmptyStateProps {
  icon: IconName;
  title: string;
  children?: ReactNode;
  action?: ReactNode;
}

export function EmptyState({ icon, title, children, action }: EmptyStateProps) {
  return (
    <div className="empty-state">
      <div className="empty-icon">
        <Icon name={icon} size={22} />
      </div>
      <h3>{title}</h3>
      {children !== undefined ? <p className="muted">{children}</p> : null}
      {action !== undefined ? <div className="empty-action">{action}</div> : null}
    </div>
  );
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="empty-state empty-error" role="alert">
      <div className="empty-icon">
        <Icon name="alert" size={22} />
      </div>
      <h3>Could not load this</h3>
      <p className="muted">{message}</p>
      {onRetry !== undefined ? (
        <div className="empty-action">
          <button type="button" className="btn btn-secondary" onClick={onRetry}>
            <Icon name="refresh" size={16} /> Try again
          </button>
        </div>
      ) : null}
    </div>
  );
}

export function Skeleton({ rows = 3, height = 56 }: { rows?: number; height?: number }) {
  return (
    <div className="skeleton-list" aria-busy="true" aria-label="Loading">
      {Array.from({ length: rows }, (_, index) => (
        <div key={index} className="skeleton" style={{ height }} />
      ))}
    </div>
  );
}

export function Spinner({ size = 16 }: { size?: number }) {
  return <span className="spinner" style={{ width: size, height: size }} aria-hidden="true" />;
}

interface SwitchProps {
  checked: boolean;
  onChange: (next: boolean) => void;
  label: string;
  disabled?: boolean;
  showText?: boolean;
}

// Switch is the public/private toggle: a button with role="switch", so a
// screen reader announces its state and Space flips it.
export function Switch({ checked, onChange, label, disabled, showText = true }: SwitchProps) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      className={checked ? "switch switch-on" : "switch"}
      onClick={() => onChange(!checked)}
      disabled={disabled}
    >
      <span className="switch-track">
        <span className="switch-thumb" />
      </span>
      {showText ? <span className="switch-text">{checked ? "Public" : "Private"}</span> : null}
    </button>
  );
}

export function VisibilityBadge({ isPublic }: { isPublic: boolean }) {
  return isPublic ? (
    <span className="badge badge-public">
      <Icon name="globe" size={13} /> Public
    </span>
  ) : (
    <span className="badge badge-private">
      <Icon name="lock" size={13} /> Private
    </span>
  );
}

export function KindBadge({ kind }: { kind: string }) {
  return <span className={`badge badge-kind kind-${kind}`}>{kind}</span>;
}

// CopyButton copies a value and says so for a moment.
export function CopyButton({ value, label = "Copy" }: { value: string; label?: string }) {
  const [copied, setCopied] = useState(false);

  async function copy(): Promise<void> {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1600);
    } catch {
      setCopied(false);
    }
  }

  return (
    <button
      type="button"
      className="icon-btn copy-btn"
      onClick={() => void copy()}
      aria-label={copied ? "Copied" : label}
      title={copied ? "Copied" : label}
    >
      <Icon name={copied ? "check" : "copy"} size={15} />
    </button>
  );
}

// CodeBlock is a command a person copies: one line or a few, with a copy
// button that takes the whole block.
export function CodeBlock({ code, label }: { code: string; label?: string }) {
  return (
    <div className="codeblock">
      {label !== undefined ? <div className="codeblock-label">{label}</div> : null}
      <pre>
        <code>{code}</code>
      </pre>
      <CopyButton value={code} />
    </div>
  );
}

// Tabs is a small segmented control for a few choices.
export function Segmented<T extends string>({
  value,
  options,
  onChange,
  label,
}: {
  value: T;
  options: { value: T; label: string }[];
  onChange: (value: T) => void;
  label: string;
}) {
  return (
    <div className="segmented" role="tablist" aria-label={label}>
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          role="tab"
          aria-selected={option.value === value}
          className={option.value === value ? "segment active" : "segment"}
          onClick={() => onChange(option.value)}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}
