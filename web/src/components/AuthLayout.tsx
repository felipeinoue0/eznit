import { useState } from "react";
import type { FormEvent, ReactNode } from "react";
import { Link } from "react-router-dom";
import { checkHealth, messageOf, normalizeServer, serverLabel } from "../api";
import { useSession } from "../session";
import Icon from "./Icon";
import Logo from "./Logo";
import { Spinner } from "./ui";

const points: { icon: "layers" | "globe" | "rocket"; title: string; body: string }[] = [
  {
    icon: "layers",
    title: "Templates for every project",
    body: "Keep the agent rules, scripts and config a project needs as one template.",
  },
  {
    icon: "globe",
    title: "Private or public, your call",
    body: "Share a template with everyone on the server, or keep it to yourself.",
  },
  {
    icon: "rocket",
    title: "On any machine in one command",
    body: "eznit template apply writes it into a folder — from your own server.",
  },
];

export default function AuthLayout({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className="auth">
      <section className="auth-brand" aria-hidden="false">
        <Link to="/" className="auth-logo" aria-label="eznit home">
          <Logo className="brand-logo" />
        </Link>
        <div className="auth-pitch">
          <h2>Set up any project on any machine.</h2>
          <p>
            eznit keeps the files a project needs — templates, scripts and configuration — on a
            server you run, and puts them wherever you work.
          </p>
          <ul className="auth-points">
            {points.map((point) => (
              <li key={point.title}>
                <span className="auth-point-icon">
                  <Icon name={point.icon} size={18} />
                </span>
                <span>
                  <strong>{point.title}</strong>
                  <span>{point.body}</span>
                </span>
              </li>
            ))}
          </ul>
        </div>
        <p className="auth-foot">Self-hosted · open source · MIT</p>
      </section>

      <section className="auth-panel">
        <div className="auth-card">
          <Link to="/" className="auth-back">
            <Icon name="arrowLeft" size={16} /> Back to templates
          </Link>
          <h1>{title}</h1>
          <p className="muted">{subtitle}</p>
          <ServerChip />
          {children}
        </div>
      </section>
    </div>
  );
}

// ServerChip says which server the account lives on, and changes it in place:
// an account belongs to one server, so this is the first thing to get right.
function ServerChip() {
  const { server, switchServer } = useSession();
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState(server);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    setError(null);
    let next: string;
    try {
      next = normalizeServer(value);
    } catch (cause: unknown) {
      setError(messageOf(cause));
      return;
    }
    setBusy(true);
    const health = await checkHealth(next);
    setBusy(false);
    if (!health.ok) {
      setError(health.error ?? "The server did not answer.");
      return;
    }
    switchServer(next);
    setEditing(false);
  }

  if (!editing) {
    return (
      <div className="server-chip">
        <Icon name="server" size={16} />
        <span>
          Server <strong>{serverLabel(server)}</strong>
        </span>
        <button
          type="button"
          className="link"
          onClick={() => {
            setValue(server);
            setEditing(true);
          }}
        >
          Change
        </button>
      </div>
    );
  }

  return (
    <form className="server-chip-form" onSubmit={(event) => void submit(event)}>
      <label className="field">
        <span className="field-label">Server address</span>
        <input
          type="text"
          inputMode="url"
          placeholder={`${window.location.origin} (this server)`}
          value={value}
          onChange={(event) => setValue(event.target.value)}
        />
        <span className="field-hint">Leave empty for the server that serves this page.</span>
      </label>
      {error !== null ? (
        <p className="form-error" role="alert">
          {error}
        </p>
      ) : null}
      <div className="row-actions">
        <button type="button" className="btn btn-ghost" onClick={() => setEditing(false)}>
          Cancel
        </button>
        <button type="submit" className="btn btn-secondary" disabled={busy}>
          {busy ? <Spinner /> : null} Connect
        </button>
      </div>
    </form>
  );
}
