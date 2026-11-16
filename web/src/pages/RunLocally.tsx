import { useState } from "react";
import { Link } from "react-router-dom";
import { serverLabel, serverURL } from "../api";
import { apiAddressFor, cliTarget, envLines } from "../cli";
import type { Shell } from "../cli";
import Icon from "../components/Icon";
import { CodeBlock, PageHeader, Segmented } from "../components/ui";
import { useSession } from "../session";

const RELEASES = "https://github.com/fiwon123/eznit/releases/latest";

const shells: { value: Shell; label: string }[] = [
  { value: "posix", label: "bash / zsh" },
  { value: "fish", label: "fish" },
  { value: "powershell", label: "PowerShell" },
];

export default function RunLocally() {
  const { server } = useSession();
  const [shell, setShell] = useState<Shell>("posix");
  const address = apiAddressFor(serverURL(server));
  const target = cliTarget(address);
  const exe = shell === "powershell" ? ".\\eznit.exe" : "eznit";
  const serveLine =
    shell === "powershell"
      ? `$env:DB_HOST = "localhost"; $env:DB_PORT = "5432"; ${exe} serve`
      : `DB_HOST=localhost DB_PORT=5432 ${exe} serve`;

  return (
    <div className="page">
      <PageHeader
        title="Run locally"
        description={
          <>
            Get templates and files onto the machine in front of you with the eznit CLI — from{" "}
            <strong>{serverLabel(server)}</strong> or from a server of your own.
          </>
        }
      />

      <div className="toolbar">
        <span className="muted small">Commands for</span>
        <Segmented value={shell} options={shells} onChange={setShell} label="Shell" />
      </div>

      <ol className="steps">
        <li className="panel step">
          <span className="step-num">1</span>
          <div className="step-body">
            <h2>Install the CLI</h2>
            <p className="muted">
              One download holds the CLI (<code>eznit</code>), the desktop app and the server. Pick
              the archive for your platform, unpack it and put <code>eznit</code> on your PATH.
            </p>
            <a className="btn btn-secondary" href={RELEASES} target="_blank" rel="noreferrer">
              <Icon name="download" size={16} /> Download from GitHub Releases{" "}
              <Icon name="external" size={14} />
            </a>
            <CodeBlock label="Check it works" code={`${exe} -v`} />
          </div>
        </li>

        <li className="panel step">
          <span className="step-num">2</span>
          <div className="step-body">
            <h2>Point it at this server</h2>
            <p className="muted">
              {target.isDefault ? (
                <>
                  The CLI talks to <code>http://localhost:4000</code> unless told otherwise — that
                  is this server, so there is nothing to set.
                </>
              ) : (
                <>
                  The CLI reads the server from two variables. Set them in your shell profile to
                  make it stick.
                </>
              )}
            </p>
            {!target.isDefault ? (
              <CodeBlock label={address} code={envLines(target, shell)} />
            ) : null}
            <CodeBlock label="Sign in (or sign up)" code={`${exe} login\n${exe} list`} />
          </div>
        </li>

        <li className="panel step">
          <span className="step-num">3</span>
          <div className="step-body">
            <h2>Apply a template</h2>
            <p className="muted">
              Find an id on <Link to="/">Explore</Link>, then write the template into a folder. It
              asks its questions first.
            </p>
            <CodeBlock
              code={`${exe} template list\n${exe} template apply <id> --target ./my-project`}
            />
            <p className="muted small">
              No server at hand? <code>{exe} template init --kind opencode --target ./demo</code>{" "}
              writes a starter offline, and <code>template export</code> /{" "}
              <code>template apply --from-file</code> carry a template as one file.
            </p>
          </div>
        </li>

        <li className="panel step">
          <span className="step-num">4</span>
          <div className="step-body">
            <h2>Publish and sync files</h2>
            <p className="muted">
              Publish a folder as a template, or keep single files and get them back anywhere. Both
              are private until you say otherwise.
            </p>
            <CodeBlock
              code={`${exe} template publish ./my-setup   # add --public to share it\n${exe} upload                       # asks for the path\n${exe} download                     # asks for the file id`}
            />
          </div>
        </li>
      </ol>

      <section className="section">
        <div className="section-head">
          <h2>Run your own server</h2>
          <p className="muted">
            eznit is self-hosted. Any machine with Docker runs the whole stack; this UI can then
            point at it from <Link to="/settings">Settings</Link>.
          </p>
        </div>
        <div className="two-col">
          <div className="panel side-card">
            <h3 className="panel-title">
              <Icon name="server" size={16} /> With Docker Compose
            </h3>
            <p className="muted small">
              From a clone of the repository: API on :4000, UI on :5173.
            </p>
            <CodeBlock code={"make setup && make up"} />
          </div>
          <div className="panel side-card">
            <h3 className="panel-title">
              <Icon name="zap" size={16} /> From the download
            </h3>
            <p className="muted small">
              Inside the unpacked archive: PostgreSQL from its compose file, then the API (it serves
              this UI on :4000). Fill in <code>.env</code> and the secret file first — see
              RUNNING.md in the archive.
            </p>
            <CodeBlock code={`docker compose up -d db migrate\n${serveLine}`} />
          </div>
        </div>
        <div className="callout">
          <Icon name="globe" />
          <div>
            To use <em>this</em> page against a server on another address, set{" "}
            <code>CORS_ALLOWED_ORIGINS={window.location.origin}</code> on that server, then change
            the server in <Link to="/settings">Settings</Link>.
          </div>
        </div>
      </section>
    </div>
  );
}
