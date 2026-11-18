import Icon from "../components/Icon";
import { PageHeader } from "../components/ui";

const REPOSITORY = "https://github.com/fiwon123/eznit";

// The version is fixed when the UI is built: `make web_build` (and the release)
// pass the git tag, and the Docker images pass VERSION, which `make vps-update`
// keeps in .env (#410). A build without one — `npm run dev`, a plain
// `docker compose build` — says "dev".
const VERSION = import.meta.env.VITE_APP_VERSION || "dev";

// About is a description of the web UI, nothing to run: the browser has nothing
// to update or revert, because the server serves whatever release it runs.
export default function About() {
  return (
    <div className="page">
      <PageHeader title="About" />
      <section className="panel about-card" aria-labelledby="about-name">
        <img className="about-logo" src="/logo.svg" alt="" width="96" height="96" />
        <h2 className="about-name" id="about-name">
          eznit
        </h2>
        <p className="about-tagline">Your project's setup, in one place you host.</p>
        <p className="muted about-description">
          Templates, scripts and files you can pull onto any machine — self-hosted, no subscription,
          no third party.
        </p>
        <dl className="meta-list about-meta">
          <dt>Version</dt>
          <dd>
            <code>{VERSION}</code>
          </dd>
          <dt>Repository</dt>
          <dd>
            <a href={REPOSITORY} target="_blank" rel="noreferrer">
              github.com/fiwon123/eznit <Icon name="external" size={14} />
            </a>
          </dd>
        </dl>
      </section>
    </div>
  );
}
