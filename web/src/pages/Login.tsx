import { useState } from "react";
import type { FormEvent } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { login, messageOf, refused, setToken } from "../api";

interface LoginProps {
  onAuthenticated: (token: string) => void;
}

// handedOver is the reason a page sent the reader here. App hands over "your
// session ended" when the API refused the token, and the login page is where
// that sentence belongs.
function handedOver(state: unknown): string | null {
  if (typeof state !== "object" || state === null) {
    return null;
  }

  const message = (state as { message?: unknown }).message;
  return typeof message === "string" ? message : null;
}

export default function Login({ onAuthenticated }: LoginProps) {
  const navigate = useNavigate();
  const location = useLocation();
  const [email, setEmail] = useState<string>("");
  const [password, setPassword] = useState<string>("");
  const [error, setError] = useState<string | null>(() => handedOver(location.state));
  const [busy, setBusy] = useState<boolean>(false);

  async function submit(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    setBusy(true);
    setError(null);

    try {
      const response = await login(email, password);
      setToken(response.token);
      onAuthenticated(response.token);
      navigate("/");
    } catch (cause: unknown) {
      // The server refuses to say which half was wrong, so neither does the
      // page — the same one line the window shows (#337).
      setError(refused(cause) ? "Wrong email or password." : messageOf(cause));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="page narrow">
      <div className="pagehead">
        <h1>Log in</h1>
        <p className="lead">
          The catalogue is public; a session lets you publish templates, upload files and see your
          own private ones. The token stays in this browser and nowhere else.
        </p>
      </div>

      <form className="form" onSubmit={submit}>
        <label className="field">
          <span>Email</span>
          <input
            type="email"
            name="email"
            required
            autoComplete="username"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
          />
        </label>

        <label className="field">
          <span>Password</span>
          <input
            type="password"
            name="password"
            required
            autoComplete="current-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </label>

        {error !== null ? <p className="error">{error}</p> : null}

        <button className="primary" type="submit" disabled={busy}>
          {busy ? "Logging in…" : "Log in"}
        </button>
      </form>
    </section>
  );
}
