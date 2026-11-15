import { useCallback, useState } from "react";
import { Link, Route, Routes, useNavigate, useParams } from "react-router-dom";
import { clearToken, getToken, setToken } from "./api";
import Logo from "./components/Logo";
import Catalogue from "./pages/Catalogue";
import Files from "./pages/Files";
import Login from "./pages/Login";
import Signup from "./pages/Signup";
import TemplateDetailPage from "./pages/TemplateDetail";

// sessionEnded is what the login page says when it is reached because the API
// refused the token. One line, and the same one the desktop window uses.
const sessionEnded = "Your session ended. Sign in again.";

// TemplateDetailPageRoute keys the detail page by the template it shows, so
// moving from one template to another is a new page: the download state, the
// copied flag and the refusal all start fresh, and a slow answer for the old
// template cannot land on the new one.
function TemplateDetailPageRoute() {
  const { id } = useParams<{ id: string }>();
  return <TemplateDetailPage key={id ?? ""} />;
}

export default function App() {
  const navigate = useNavigate();

  // The token lives in localStorage; this state is only there so the top bar
  // and the pages agree on whether there is a session right now.
  const [token, setTokenState] = useState<string | null>(getToken);

  function logIn(next: string): void {
    setToken(next);
    setTokenState(next);
  }

  function logOut(): void {
    clearToken();
    setTokenState(null);
  }

  // endSession is logOut for the API's own answer: the token was refused, so
  // the token goes, the top bar notices, and the login page says why. It is
  // memoised because a page may call it from an effect.
  const endSession = useCallback(() => {
    logOut();
    navigate("/login", { state: { message: sessionEnded } });
  }, [navigate]);

  return (
    <div className="app">
      <header className="topbar">
        <Link className="brand" to="/" aria-label="eznit">
          {/* The mark is inlined as a component so currentColor follows the
              theme; an <img>-loaded SVG isolates its document and made the
              wordmark black-on-black on the dark default. The mark itself is
              decorative, so the link that wraps it is what names it. */}
          <Logo className="brand-logo" />
        </Link>
        <nav className="topnav">
          <Link to="/">catalogue</Link>
          {token === null ? (
            <>
              <Link to="/login">log in</Link>
              <Link to="/signup">sign up</Link>
            </>
          ) : (
            <>
              <Link to="/files">files</Link>
              <button className="linkbutton" type="button" onClick={logOut}>
                log out
              </button>
            </>
          )}
        </nav>
      </header>

      <main className="main">
        <Routes>
          <Route path="/" element={<Catalogue />} />
          {/* Keyed by the id, so moving to another template is a new page: the
              refusal, the copied flag and the download state all start fresh,
              and a slow answer for the old one cannot land here. */}
          <Route path="/templates/:id" element={<TemplateDetailPageRoute />} />
          {/* Keyed by the session, so a log-out — or a log-in as someone else —
              starts from an empty table instead of the last account's rows.
              A 401 would have the page look signed in for ever, so it tells
              App to end the session instead. */}
          <Route
            path="/files"
            element={<Files key={token ?? "anon"} onSessionEnded={endSession} />}
          />
          <Route path="/login" element={<Login onAuthenticated={logIn} />} />
          <Route path="/signup" element={<Signup onAuthenticated={logIn} />} />
          <Route
            path="*"
            element={<p className="empty">There is no such page. The catalogue is over here.</p>}
          />
        </Routes>
      </main>

      <footer className="footer">
        <span>eznit — self-hosted file storage</span>
        <code>eznit template list</code>
      </footer>
    </div>
  );
}
