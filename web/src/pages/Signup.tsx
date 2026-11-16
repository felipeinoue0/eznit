import { useState } from "react";
import type { FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { login, messageOf, refused, signup } from "../api";
import AuthLayout from "../components/AuthLayout";
import { Spinner } from "../components/ui";
import { useSession } from "../session";

// The API answers "user created!" for an address that already has an account —
// on purpose, so signup cannot be used to discover one — and the login that
// follows is what fails. This names the likely case and the way out. It is
// used for a 401 only: a signup followed by a server that went away gets its
// own sentence (#337).
const signupNotSignedIn = "That email already has an account. Sign in with its password.";

export default function Signup() {
  const navigate = useNavigate();
  const { signIn } = useSession();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const mismatch = confirmPassword !== "" && confirmPassword !== password;

  async function submit(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    if (mismatch) {
      setError("The passwords do not match.");
      return;
    }
    setBusy(true);
    setError(null);

    let signedUp = false;
    try {
      await signup(email, password, confirmPassword);
      // A new account has no session yet, so the sign-in follows rather than
      // throwing the reader back to a form to type it all again.
      signedUp = true;
      const response = await login(email, password);
      signIn(response.token, email.trim());
      navigate("/templates", { replace: true });
    } catch (cause: unknown) {
      setError(signedUp && refused(cause) ? signupNotSignedIn : messageOf(cause));
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthLayout
      title="Create your account"
      subtitle="An email and a password is all it takes. The first account on a server is its admin."
    >
      <form className="form" onSubmit={(event) => void submit(event)}>
        <label className="field">
          <span className="field-label">Email</span>
          <input
            type="email"
            name="email"
            required
            autoComplete="email"
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
            autoComplete="new-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </label>

        <label className="field">
          <span className="field-label">Confirm password</span>
          <input
            type="password"
            name="confirm_password"
            required
            autoComplete="new-password"
            aria-invalid={mismatch}
            value={confirmPassword}
            onChange={(event) => setConfirmPassword(event.target.value)}
          />
          {mismatch ? <span className="field-hint field-hint-error">Does not match.</span> : null}
        </label>

        {error !== null ? (
          <p className="form-error" role="alert">
            {error}
          </p>
        ) : null}

        <button className="btn btn-primary btn-block btn-lg" type="submit" disabled={busy}>
          {busy ? <Spinner /> : null}
          {busy ? "Creating your account…" : "Create account"}
        </button>
      </form>

      <p className="auth-switch">
        Already have an account? <Link to="/login">Sign in</Link>
      </p>
    </AuthLayout>
  );
}
