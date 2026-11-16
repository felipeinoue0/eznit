import { createContext, useCallback, useContext, useMemo, useState } from "react";
import type { ReactNode } from "react";
import { useNavigate } from "react-router-dom";
import * as api from "./api";

// sessionEnded is what the login page says when it is reached because the API
// refused the token. One line, and the same one the desktop window uses.
export const sessionEnded = "Your session ended. Sign in again.";

interface SessionState {
  server: string;
  token: string | null;
  email: string | null;
  // signedOutFrom is the page a deliberate sign-out happened on. The guard on
  // a protected page sends the reader to Explore from there, not to the
  // sign-in form they just chose to leave.
  signedOutFrom: string | null;
}

interface Session extends SessionState {
  signIn: (token: string, email: string) => void;
  signOut: () => Promise<void>;
  // endSession is signOut for the API's own answer: the token was refused, so
  // it goes, and the login page says why.
  endSession: () => void;
  // switchServer points the UI at another server. Each server keeps its own
  // session, so the token shown afterwards is the one stored for that server.
  switchServer: (server: string) => void;
}

const SessionContext = createContext<Session | null>(null);

function snapshot(server = api.getServer()): SessionState {
  return { server, token: api.getToken(server), email: api.getEmail(server), signedOutFrom: null };
}

export function SessionProvider({ children }: { children: ReactNode }) {
  const navigate = useNavigate();
  const [state, setState] = useState<SessionState>(() => snapshot());

  const signIn = useCallback((token: string, email: string) => {
    api.saveSession(token, email);
    setState(snapshot());
  }, []);

  // A deliberate sign-out stays on a public page, and leaves a protected one
  // for Explore (RequireAuth reads signedOutFrom). It is not a navigate()
  // here: React Router runs navigations as transitions, so the cleared
  // session would render on the old page first and its redirect would win.
  const signOut = useCallback(async () => {
    await api.logout();
    api.clearSession();
    setState({ ...snapshot(), signedOutFrom: window.location.pathname });
  }, []);

  // Idempotent: a refusal that lands after the session is already gone — an
  // answer to a request sent just before a sign-out — has nothing to end, and
  // must not send a reader who signed out on purpose to the sign-in form.
  const endSession = useCallback(() => {
    if (api.getToken() === null) {
      return;
    }
    api.clearSession();
    setState(snapshot());
    navigate("/login", { state: { message: sessionEnded } });
  }, [navigate]);

  const switchServer = useCallback((server: string) => {
    api.setServer(server);
    setState(snapshot(server));
  }, []);

  const value = useMemo<Session>(
    () => ({ ...state, signIn, signOut, endSession, switchServer }),
    [state, signIn, signOut, endSession, switchServer],
  );

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): Session {
  const session = useContext(SessionContext);
  if (session === null) {
    throw new Error("useSession outside SessionProvider");
  }
  return session;
}
