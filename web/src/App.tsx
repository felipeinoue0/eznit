import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import { Link, Navigate, NavLink, Route, Routes, useLocation, useParams } from "react-router-dom";
import { checkHealth, serverLabel } from "./api";
import type { Health } from "./api";
import Icon from "./components/Icon";
import type { IconName } from "./components/Icon";
import Logo from "./components/Logo";
import ThemeToggle from "./components/ThemeToggle";
import { ConfirmProvider, ToastProvider } from "./components/ui";
import About from "./pages/About";
import Commands from "./pages/Commands";
import Explore from "./pages/Explore";
import Files from "./pages/Files";
import Login from "./pages/Login";
import MyTemplates from "./pages/MyTemplates";
import RunLocally from "./pages/RunLocally";
import Settings from "./pages/Settings";
import Signup from "./pages/Signup";
import TemplateDetailPage from "./pages/TemplateDetail";
import TemplateEditor from "./pages/TemplateEditor";
import { SessionProvider, useSession } from "./session";

export default function App() {
  return (
    <SessionProvider>
      <ToastProvider>
        <ConfirmProvider>
          <Routes>
            <Route path="/login" element={<Login />} />
            <Route path="/signup" element={<Signup />} />
            <Route path="*" element={<Shell />} />
          </Routes>
        </ConfirmProvider>
      </ToastProvider>
    </SessionProvider>
  );
}

// Keyed by the id, so moving from one template to another is a new page: the
// state starts fresh, and a slow answer for the old one cannot land here.
function TemplateDetailRoute() {
  const { id } = useParams<{ id: string }>();
  return <TemplateDetailPage key={id ?? ""} />;
}

function TemplateEditRoute() {
  const { id } = useParams<{ id: string }>();
  return <TemplateEditor key={id ?? "new"} id={id} />;
}

// RequireAuth sends a visitor to sign in, and back here afterwards.
function RequireAuth({ children }: { children: ReactNode }) {
  const { token, signedOutFrom } = useSession();
  const location = useLocation();
  if (token === null) {
    if (signedOutFrom === location.pathname) {
      return <Navigate to="/" replace />;
    }
    return <Navigate to="/login" replace state={{ from: location.pathname }} />;
  }
  return <>{children}</>;
}

function Shell() {
  const { token, server } = useSession();
  const location = useLocation();
  const [menuOpen, setMenuOpen] = useState(false);

  // A navigation closes the drawer on a narrow screen. Keyed on the path, so
  // the sidebar does not have to call back into here on every link.
  const [lastPath, setLastPath] = useState(location.pathname);
  if (lastPath !== location.pathname) {
    setLastPath(location.pathname);
    setMenuOpen(false);
  }

  return (
    <div className={menuOpen ? "shell menu-open" : "shell"}>
      <aside className="sidebar" aria-label="Main">
        <div className="sidebar-top">
          <Link className="brand" to="/" aria-label="eznit home">
            <Logo className="brand-logo" />
          </Link>
        </div>

        <nav className="nav">
          <NavSection title="Discover">
            <NavItem to="/" icon="compass" label="Explore" end />
          </NavSection>

          {token !== null ? (
            <NavSection title="Workspace">
              <NavItem to="/templates" icon="layers" label="My templates" />
              <NavItem to="/files" icon="folder" label="Files" />
              <NavItem to="/commands" icon="terminal" label="Commands" />
            </NavSection>
          ) : null}

          <NavSection title="Use it">
            <NavItem to="/local" icon="rocket" label="Run locally" />
            <NavItem to="/settings" icon="settings" label="Settings" />
            <NavItem to="/about" icon="info" label="About" />
          </NavSection>
        </nav>

        <div className="sidebar-bottom">
          <ServerStatus key={server} server={server} />
          <Account />
        </div>
      </aside>

      <div className="scrim" onClick={() => setMenuOpen(false)} aria-hidden="true" />

      <div className="content">
        <header className="mobilebar">
          <button
            type="button"
            className="icon-btn"
            aria-label="Open the menu"
            aria-expanded={menuOpen}
            onClick={() => setMenuOpen(true)}
          >
            <Icon name="menu" />
          </button>
          <Link to="/" aria-label="eznit home">
            <Logo className="brand-logo" />
          </Link>
          <ThemeToggle compact />
        </header>

        <main className="main" id="main">
          <Routes>
            <Route path="/" element={<Explore />} />
            <Route
              path="/templates"
              element={
                <RequireAuth>
                  <MyTemplates />
                </RequireAuth>
              }
            />
            <Route
              path="/templates/new"
              element={
                <RequireAuth>
                  <TemplateEditor key="new" />
                </RequireAuth>
              }
            />
            <Route path="/templates/:id" element={<TemplateDetailRoute />} />
            <Route
              path="/templates/:id/edit"
              element={
                <RequireAuth>
                  <TemplateEditRoute />
                </RequireAuth>
              }
            />
            <Route
              path="/files"
              element={
                <RequireAuth>
                  {/* Keyed by the session, so a sign-in as someone else
                      starts from an empty table, not the last account's. */}
                  <Files key={`${server}|${token ?? ""}`} />
                </RequireAuth>
              }
            />
            <Route
              path="/commands"
              element={
                <RequireAuth>
                  <Commands key={`${server}|${token ?? ""}`} />
                </RequireAuth>
              }
            />
            <Route path="/local" element={<RunLocally />} />
            <Route path="/settings" element={<Settings />} />
            <Route path="/about" element={<About />} />
            <Route path="*" element={<NotFound />} />
          </Routes>
        </main>
      </div>
    </div>
  );
}

function NavSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="nav-section">
      <div className="nav-title">{title}</div>
      {children}
    </div>
  );
}

function NavItem({
  to,
  icon,
  label,
  end,
}: {
  to: string;
  icon: IconName;
  label: string;
  end?: boolean;
}) {
  return (
    <NavLink
      to={to}
      end={end}
      className={({ isActive }) => (isActive ? "nav-item active" : "nav-item")}
    >
      <Icon name={icon} />
      <span>{label}</span>
    </NavLink>
  );
}

// ServerStatus is the pill that says which server this UI talks to and
// whether it answers. It re-checks every 30 seconds while the page is open.
function ServerStatus({ server }: { server: string }) {
  const [health, setHealth] = useState<Health | null>(null);

  useEffect(() => {
    let cancelled = false;
    const probe = async (): Promise<void> => {
      const answer = await checkHealth(server);
      if (!cancelled) {
        setHealth(answer);
      }
    };
    void probe();
    const timer = window.setInterval(() => void probe(), 30000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [server]);

  const state = health === null ? "checking" : health.ok ? "online" : "offline";
  const title =
    health === null
      ? "Checking the server…"
      : health.ok
        ? `Online · ${health.ms} ms`
        : (health.error ?? "Offline");

  return (
    <Link to="/settings" className={`server-pill server-${state}`} title={title}>
      <span className="status-dot" aria-hidden="true" />
      <span className="server-pill-text">
        <span className="server-pill-host">{serverLabel(server)}</span>
        <span className="server-pill-state">
          {state === "checking" ? "Checking…" : state === "online" ? "Connected" : "Unreachable"}
        </span>
      </span>
      <Icon name="server" size={16} className="muted" />
    </Link>
  );
}

function Account() {
  const { token, email, signOut } = useSession();

  if (token === null) {
    return (
      <div className="account">
        <Link to="/login" className="btn btn-primary btn-block">
          Sign in
        </Link>
        <ThemeToggle />
      </div>
    );
  }

  const initial = (email ?? "?").charAt(0).toUpperCase();
  return (
    <div className="account">
      <div className="account-row">
        <span className="avatar" aria-hidden="true">
          {initial}
        </span>
        <span className="account-email" title={email ?? undefined}>
          {email ?? "Signed in"}
        </span>
        <button
          type="button"
          className="icon-btn"
          aria-label="Sign out"
          title="Sign out"
          onClick={() => void signOut()}
        >
          <Icon name="logout" size={16} />
        </button>
      </div>
      <ThemeToggle />
    </div>
  );
}

function NotFound() {
  return (
    <div className="page">
      <div className="empty-state">
        <div className="empty-icon">
          <Icon name="compass" size={22} />
        </div>
        <h3>There is no such page</h3>
        <p className="muted">The link may be old, or the template was deleted.</p>
        <div className="empty-action">
          <Link to="/" className="btn btn-primary">
            Explore templates
          </Link>
        </div>
      </div>
    </div>
  );
}
