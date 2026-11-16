import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { Link } from "react-router-dom";
import { checkHealth, messageOf, normalizeServer, serverLabel, serverURL } from "../api";
import type { Health } from "../api";
import Icon from "../components/Icon";
import ThemeToggle from "../components/ThemeToggle";
import { PageHeader, Spinner, useToast } from "../components/ui";
import { useSession } from "../session";

type Mode = "here" | "remote";

export default function Settings() {
  const { server, token, email, switchServer, signOut } = useSession();
  const toast = useToast();
  const [mode, setMode] = useState<Mode>(server === "" ? "here" : "remote");
  const [address, setAddress] = useState(server);
  const [testing, setTesting] = useState(false);
  const [test, setTest] = useState<Health | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [health, setHealth] = useState<Health | null>(null);

  useEffect(() => {
    let cancelled = false;
    void checkHealth(server).then((answer) => {
      if (!cancelled) {
        setHealth(answer);
      }
    });
    return () => {
      cancelled = true;
    };
  }, [server]);

  function target(): string | null {
    if (mode === "here") {
      return "";
    }
    try {
      const normalized = normalizeServer(address);
      if (normalized === "") {
        setError("Type the address of the other server.");
        return null;
      }
      return normalized;
    } catch (cause: unknown) {
      setError(messageOf(cause));
      return null;
    }
  }

  async function runTest(): Promise<void> {
    setError(null);
    const next = target();
    if (next === null) {
      return;
    }
    setTesting(true);
    setTest(await checkHealth(next));
    setTesting(false);
  }

  async function save(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    setError(null);
    const next = target();
    if (next === null) {
      return;
    }
    setTesting(true);
    const answer = await checkHealth(next);
    setTesting(false);
    setTest(answer);
    if (!answer.ok) {
      setError(answer.error ?? "The server did not answer.");
      return;
    }
    switchServer(next);
    setHealth(answer);
    toast.success("Server changed", `Now using ${serverLabel(next)}.`);
  }

  const changed = mode === "here" ? server !== "" : address.trim() !== server;

  return (
    <div className="page page-narrow">
      <PageHeader
        title="Settings"
        description="Which server this page talks to, and how it looks."
      />

      <section className="panel form-section">
        <div className="form-section-head">
          <h2>Server</h2>
          <p className="muted small">
            Every account, template and file lives on one server. Each server keeps its own sign-in
            in this browser.
          </p>
        </div>

        <div className="current-server">
          <span className={`status-dot ${health === null ? "" : health.ok ? "ok" : "bad"}`} />
          <div className="grow">
            <strong>{serverURL(server)}</strong>
            <div className="muted small">
              {health === null
                ? "Checking…"
                : health.ok
                  ? `Online · answered in ${health.ms} ms`
                  : (health.error ?? "Unreachable")}
            </div>
          </div>
          <span className="badge">{server === "" ? "This server" : "Remote"}</span>
        </div>

        <form className="form" onSubmit={(event) => void save(event)}>
          <div className="choice-cards">
            <label className={mode === "here" ? "choice active" : "choice"}>
              <input
                type="radio"
                name="server-mode"
                checked={mode === "here"}
                onChange={() => {
                  setMode("here");
                  setTest(null);
                  setError(null);
                }}
              />
              <Icon name="monitor" size={18} />
              <span>
                <strong>This server</strong>
                <span className="muted small">
                  {window.location.host} — the one serving this page
                </span>
              </span>
            </label>
            <label className={mode === "remote" ? "choice active" : "choice"}>
              <input
                type="radio"
                name="server-mode"
                checked={mode === "remote"}
                onChange={() => {
                  setMode("remote");
                  setTest(null);
                  setError(null);
                }}
              />
              <Icon name="globe" size={18} />
              <span>
                <strong>Another server</strong>
                <span className="muted small">A self-hosted eznit anywhere, by URL</span>
              </span>
            </label>
          </div>

          {mode === "remote" ? (
            <label className="field">
              <span className="field-label">Server address</span>
              <input
                type="text"
                inputMode="url"
                placeholder="https://eznit.example.com"
                value={address}
                onChange={(event) => {
                  setAddress(event.target.value);
                  setTest(null);
                }}
              />
              <span className="field-hint">
                The server must allow this page:{" "}
                <code>CORS_ALLOWED_ORIGINS={window.location.origin}</code>.
              </span>
            </label>
          ) : null}

          {test !== null ? (
            <p className={test.ok ? "form-ok" : "form-error"} aria-live="polite">
              <Icon name={test.ok ? "check" : "alert"} size={16} />
              {test.ok ? `Reachable · ${test.ms} ms` : test.error}
            </p>
          ) : null}
          {error !== null && test?.error !== error ? (
            <p className="form-error" role="alert">
              {error}
            </p>
          ) : null}

          <div className="row-actions">
            <button
              type="button"
              className="btn btn-secondary"
              onClick={() => void runTest()}
              disabled={testing}
            >
              {testing ? <Spinner /> : <Icon name="zap" size={16} />} Test connection
            </button>
            <button type="submit" className="btn btn-primary" disabled={testing || !changed}>
              Use this server
            </button>
          </div>
        </form>
      </section>

      <section className="panel form-section">
        <div className="form-section-head">
          <h2>Account</h2>
          <p className="muted small">The session on {serverLabel(server)}.</p>
        </div>
        {token === null ? (
          <div className="setting-row">
            <span>Not signed in.</span>
            <div className="row-actions">
              <Link to="/signup" className="btn btn-secondary">
                Create account
              </Link>
              <Link to="/login" className="btn btn-primary">
                Sign in
              </Link>
            </div>
          </div>
        ) : (
          <div className="setting-row">
            <span className="account-row">
              <span className="avatar" aria-hidden="true">
                {(email ?? "?").charAt(0).toUpperCase()}
              </span>
              <span>
                <strong>{email ?? "Signed in"}</strong>
                <span className="muted small block">The token stays in this browser.</span>
              </span>
            </span>
            <button type="button" className="btn btn-secondary" onClick={() => void signOut()}>
              <Icon name="logout" size={16} /> Sign out
            </button>
          </div>
        )}
      </section>

      <section className="panel form-section">
        <div className="form-section-head">
          <h2>Appearance</h2>
        </div>
        <div className="setting-row">
          <span>Theme</span>
          <ThemeToggle />
        </div>
      </section>

      <section className="panel form-section">
        <div className="form-section-head">
          <h2>API</h2>
          <p className="muted small">The server documents its own API.</p>
        </div>
        <div className="setting-row">
          <span className="small">OpenAPI reference (Swagger UI)</span>
          <a
            className="btn btn-secondary"
            href={`${serverURL(server)}/docs`}
            target="_blank"
            rel="noreferrer"
          >
            Open /docs <Icon name="external" size={14} />
          </a>
        </div>
      </section>
    </div>
  );
}
