import { useState } from "react";
import type { FormEvent } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { login, messageOf, refused } from "../api";
import AuthLayout from "../components/AuthLayout";
import { Spinner } from "../components/ui";
import { useSession } from "../session";

// handedOver reads what the page that sent the reader here left: a reason
// ("your session ended") and the page to go back to.
function handedOver(state: unknown): { message: string | null; from: string } {
  if (typeof state !== "object" || state === null) {
    return { message: null, from: "/" };
  }
  const { message, from } = state as { message?: unknown; from?: unknown };
  return {
    message: typeof message === "string" ? message : null,
    from: typeof from === "string" ? from : "/",
  };
}

export default function Login() {
  const navigate = useNavigate();
  const location = useLocation();
  const { signIn } = useSession();
  const handed = handedOver(location.state);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(handed.message);
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    setBusy(true);
    setError(null);

    try {
      const response = await login(email, password);
      signIn(response.token, email.trim());
      navigate(handed.from, { replace: true });
    } catch (cause: unknown) {
      // The server refuses to say which half was wrong, so neither does the
      // page — the same one line the window shows (#337).
      setError(refused(cause) ? "Wrong email or password." : messageOf(cause));
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthLayout
      title="Welcome back"
      subtitle="Sign in to publish templates, manage your files and keep your commands."
    >
      <form className="form" onSubmit={(event) => void submit(event)}>
        <label className="field">
          <span className="field-label">Email</span>
          <input
            type="email"
            name="email"
            required
            autoComplete="username"
            placeholder="you@example.com"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
          />
        </label>

        <label className="field">
          <span className="field-label">Password</span>
          <input
            type="password"
            name="password"
            required
            autoComplete="current-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </label>

        {error !== null ? (
          <p className="form-error" role="alert">
            {error}
          </p>
        ) : null}

        <button className="btn btn-primary btn-block btn-lg" type="submit" disabled={busy}>
          {busy ? <Spinner /> : null}
          {busy ? "Signing in…" : "Sign in"}
        </button>
      </form>

      <p className="auth-switch">
        New here? <Link to="/signup">Create an account</Link>
      </p>
    </AuthLayout>
  );
}
