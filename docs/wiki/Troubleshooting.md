<sub>🏠 [Home](https://github.com/fiwon123/eznit/wiki) › 🛡️ Operate › 🧯 Troubleshooting</sub>

# 🧯 Troubleshooting

**Find your symptom, then apply the fix.** Each entry names the cause first,
because the cause usually is the fix.

| Area | Symptoms |
| --- | --- |
| 🧩 [Templates](#-templates) | apply stops · publish skips a file · 401 |
| 🖥️ [Window: Local server](#%EF%B8%8F-window-local-server) | not a bundle · database could not be prepared · password failed · port in use · starts and stops |
| 🪟 [Window: startup](#-window-startup) | GL or display error · `GLIBC_2.38 not found` |
| 🔄 [Updates](#-updates) | download refused · old version after an update |
| 🐳 [Stack and sandbox](#-stack-and-sandbox) | missing secret · port already in use · UI loads but requests fail · sandbox permission errors |
| ✅ [Gates](#-gates) | `make check` green but nothing tested · migration variables missing |

> [!TIP]
> Most window errors open a dialog with the runtime's full output and a **Copy
> output** button. Paste that output into an
> [issue](https://github.com/fiwon123/eznit/issues/new). It nearly always names
> the cause.

---

## 🧩 Templates

### `template apply` stops without writing anything

**Cause:** one of the target files already exists. `apply` checks every file
before writing any of them.
**Fix:** move the conflicting file, or rerun with `--force`.

### `template publish` skips a file, or the server refuses one

**Cause:** a symlink (a template holds files, not links) or a limit: 256 KiB per
text file, 4 MiB of text in total, 10 MiB per binary file, 32 MiB in total, 200
files. The CLI checks these before uploading and names the file.

### `template apply <id>` answers 401

**Cause:** a private template needs a session.
**Fix:** `eznit login`. Public templates apply without one.

---

## 🖥️ Window: Local server

### "not a server bundle"

**Cause:** Settings → Local server runs `eznit serve` from the **binary beside
the window**, and the chosen folder has none.
**Fix:** extract the release archive (one per platform holds the window, the
server, the migrations and the Compose file) and run `eznit-gui` from inside it,
or point **Browse…** at that folder. The note under the header names what is
missing.

> [!NOTE]
> You only need a local server to *run* one. To use a server that already runs,
> type its address in **Settings → Server** and **Connect**.

### "The database could not be prepared"

Before **Start** runs the API, a step prepares the database, and its message names the
missing piece:

| Message | Fix |
| --- | --- |
| no `docker-compose.yml` | The folder is not a bundle. Start a PostgreSQL yourself and set `DB_HOST` / `DB_PORT`. |
| no container runtime on PATH | Install Docker or Podman (Fedora: `podman-compose`), or start a PostgreSQL yourself. |
| no `secrets/db_password.txt` | Set the password in **Configuration** and save. Postgres keeps it from the volume's creation. |
| **another database already answers** on this folder's `DB_HOST` and `DB_PORT` | Something else holds the port, usually another eznit stack (a checkout's `make dev-up` publishes 5432). Stop it (`make dev-down` there) or set another `DB_PORT` (for example 5433) in **Configuration**. |
| did not answer after it was started | Press **Start** again. A first image pull or a slow disk can outlast the 30-second wait. `docker compose logs db` says the rest. |

> [!NOTE]
> **Reset** refuses the "another database" case too, and erases nothing. Before
> **v0.37.2** both failed instead with
> `docker compose run --rm migrate: exit status 1`, because Compose could not
> start the folder's own `db` on the taken port.

### `password authentication failed for user "root"`

The API reached PostgreSQL and the password did not match. There are two causes:

| Cause | Fix |
| --- | --- |
| **No password was sent.** The API reads `DB_PASSWORD_FILE` first, then `DB_PWD`. A folder without `secrets/db_password.txt` (or with an `.env` from before it existed) sends an empty one. | Set the password in **Configuration** and save. Since v0.32.0 Start refuses with a note instead. |
| **The database kept an older password.** Postgres applies it when it *creates* its volume and never again. | Put the original password into **Configuration**, or change it inside the database ([how](https://github.com/fiwon123/eznit/wiki/Deploying-on-a-VPS#-changing-the-database-password)). Recreating the volume also works, **but deletes the data**. |

> [!WARNING]
> A green status dot does not mean the database is fine. `/v1/healthcheck`
> answers without touching the database, so the dot means "the process is up".

### "the port is in use"

**Cause:** something already listens on `API_PORT` (4000 by default). It is often a
server this window started in another session, or a `make run_api`.
**Fix:** stop it, or change the port in **Configuration** and save.

### The server starts and stops at once

Read the **Output** tab: the API prints why it exited. Usually the database is
unreachable, so check `DB_HOST`, `DB_PORT` and that `secrets/db_password.txt` exists.
If you run the API by hand, set `DB_PASSWORD_FILE` or `DB_PWD`.

### A Stop takes a while

That is the drain: the API gives in-flight requests up to 10 seconds. On Windows
the stop is immediate, because Windows cannot deliver an interrupt to another
process.

---

## 🪟 Window: startup

### `eznit-gui` exits with a GL or display error

**Cause:** Fyne needs OpenGL at run time. The Linux window links `libGL.so.1` and
`libwayland-client.so.0`.
**Fix:** install `libgl1` and `libwayland-client0` (Debian/Ubuntu) and a working GL
driver, and run it on a machine with a display. A headless server should use the CLI
and the web UI.

Building is a different requirement: `make build_gui` needs cgo and the GL/X11
**headers**. If `pkg-config` cannot find `gl.pc`, the headers are missing.

<details>
<summary><code>version 'GLIBC_2.38' not found</code> (v0.23.0 and earlier)</summary>

Those releases built the Linux window on a current runner, so it needed glibc 2.38,
which is newer than Debian 12 (2.36) and Ubuntu 22.04 (2.35). From **v0.23.1** the
window is linked inside a Debian 12 image and needs glibc **2.34**. The CLI is
`CGO_ENABLED=0` and was never affected.

</details>

---

## 🔄 Updates

### `eznit update` refuses the download

**Cause (deliberate):** the updater fetches `checksums.txt` **before** the archive
and refuses a missing, malformed or mismatched entry.
**Fix:** check that the release has both the archive and `checksums.txt`, and that
the tag is `vMAJOR.MINOR.PATCH`. Only HTTPS is accepted.

### The update installed, but `-v` still shows the old version

**Cause:** the running process keeps the binary it started from.
**Fix:** restart `eznit` or the window. `eznit update --revert` puts back the
previous version.

<details>
<summary>Older versions: <code>checksums.txt has no entry</code> (≤ v0.28.0), Restart starts the old window (≤ v0.30.0)</summary>

- **≤ v0.28.0** asked for an archive name the release stopped publishing in
  v0.24.0 (`eznit-cli_…` instead of `eznit_…`). Upgrade once by hand: download
  `eznit_v0.28.1_<os>_<arch>`, verify it against `checksums.txt`, extract it, and
  run it from there. From v0.28.1 the built-in update works.
- **≤ v0.30.0**: **Restart now** could start the backup (`<binary>.old`)
  instead of the new binary. The install itself was fine. Close the window and
  start it normally once. Fixed in v0.30.1.

</details>

---

## 🐳 Stack and sandbox

### `docker compose up` fails on a missing secret

**Cause:** the database password is a file secret, and Compose refuses to start
without it.
**Fix:** `make setup` creates it from the committed example.

### `bind: address already in use` on port 4000 or 5173

The product and the dev loop publish the same ports, so run one at a time:

```sh
make docker_down      # stop the product, then: make dev-up
make dev-down         # stop the dev loop, then: make up
```

When the port belongs to something else (a `make run_api`, a second checkout),
move one of them:

```sh
HOST_PORT=4001 make dev-up         # the sandbox API
WEB_PORT=5174 make dev-up          # the dev UI (hot reload follows it)
API_PORT=4001 docker compose up    # the product API
WEB_PORT=5174 docker compose up    # the product UI
```

### The UI loads on `:5173` but every request fails

**Cause:** the UI proxies `/v1` to the API, so an API that is down looks like a broken
UI.
**Fix:** check the API first, then the sandbox log (`make dev-log`) for a compile
error:

```sh
curl -s -o /dev/null -w '%{http_code}\n' localhost:4000/v1/healthcheck
```

### `EACCES: permission denied, mkdir '/home/devuser/.cache/opencode'` or `mkdir /go/pkg/mod/cache: permission denied`

**Cause:** a sandbox created before the image made its own cache directories keeps
the old root-owned volumes, because Docker never re-initialises an existing volume.
**Fix:** recreate them once:

```sh
make dev-reset     # docker compose down --volumes
make dev-up
```

> [!CAUTION]
> `make dev-reset` also drops `postgres-data` and `api-data`, the dev database and
> its uploaded files. That is why it is not called `reset`.

### `set: pipefail: invalid option name` or `bad interpreter` in the sandbox

**Cause:** a checkout from before the `.gitattributes` rule keeps CRLF line
endings, and bash reads the bind-mounted bytes as they are.
**Fix:** re-check the files out with LF endings:

```sh
git rm --cached -r . >/dev/null && git reset --hard
```

### Permission denied reading a secret file

**Cause:** the file is mode `600` and owned by another user. See
[Configuration → Files and secrets](https://github.com/fiwon123/eznit/wiki/Configuration#-files-and-secrets).

---

## ✅ Gates

### `make check` passes, but nothing seems tested

`make check` lists the packages it did not cover. Only `cmd/cli` has no test file
today. A green run means "the tested packages passed and the rest compiled", **not**
"it works". SQL is covered by `make test_integration`.

### `ERROR: DB_USER, DB_HOST and DB_NAME are not set`

**Cause:** the migration targets need a database connection.
**Fix:** `make setup`, or copy `.env.example` to `.env` and fill it in. `make check`
and `make build` do not need it.

---

## 🆘 Still stuck?

[Open an issue](https://github.com/fiwon123/eznit/issues/new) with the command you
ran, the full output (**Copy output** in the window), your OS, and `eznit -v`.
