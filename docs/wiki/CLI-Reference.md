<sub>🏠 [Home](https://github.com/fiwon123/eznit/wiki) › 📖 Reference › 💻 CLI Reference</sub>

# 💻 CLI Reference

**One binary, `eznit` (alias `ez`), is both the client and the server.** A
laptop, a bundle or a VPS needs one program, not two.

```sh
eznit --help          # every command
eznit -v              # the build's version (the server's too: same program)
eznit --debug <cmd>   # debug-level logs
```

| Section | Commands |
| --- | --- |
| 🖥️ [Server](#%EF%B8%8F-server) | `serve` |
| 🗂️ [Files and account](#%EF%B8%8F-files-and-account) | `signup` `login` `logout` `upload` `download` `list` `delete` `share` `unshare` |
| 🧩 [Templates](#-templates) | `template publish` `list` `show` `apply` `export` `init` |
| 📑 [Commands library](#-commands-library) | `command add` `list` `show` `remove` `export` `import` |
| 🐙 [Git hosts](#-git-hosts) | `repo auth` `list` `clone` |
| 🔄 [Updates](#-updates) | `update` |
| 🧾 [Config file](#-the-config-file) · 🌱 [Environment](#-cli-environment-variables) | |

> [!NOTE]
> **The core commands are interactive.** They ask for what they need instead of
> taking flags. Subcommands take flags, and some of them (`template
> publish`/`show`/`apply`/`init`, `repo auth`) also ask.

## 🖥️ Server

| Command | Flags | What it does |
| --- | --- | --- |
| `eznit serve` | `--debug` | Runs the API. It reads `API_*` / `DB_*`, serves the embedded web UI on `:4000`, and drains in-flight requests on Ctrl-C. |
| `eznit -v` | | The version, which is the server's version too |

```sh
DB_HOST=localhost DB_PORT=5432 DB_PASSWORD_FILE=secrets/db_password.txt ./eznit serve
```

`serve` needs PostgreSQL (`DB_HOST`, `DB_PORT`, `DB_USER`, `DB_NAME`, and
`DB_PASSWORD_FILE` pointing at a file). It exits non-zero when it cannot reach the
database and prints why as one JSON line. That line is what the window shows in
**Settings → Local server → Output**, and **Start** there runs this command for
you, after bringing the database up.

## 🗂️ Files and account

| Command | Asks / flags | What it does |
| --- | --- | --- |
| `eznit signup` | email, password, confirm | Creates an account. The **first one is the admin**. |
| `eznit login` | email, password | Starts a session and keeps the token in `~/.eznit/config.json` (mode `0600`) |
| `eznit logout` | | Ends the session. A git-host token stays. |
| `eznit upload` | file path | Uploads a file |
| `eznit upload --update <id>` | file path | Replaces the contents of one of your files |
| `eznit download` | file id, destination folder | Downloads one of your own files |
| `eznit list` | | Lists your own files (`--me` is the same) |
| `eznit delete` | file id | Deletes a file |
| `eznit share` | file id, confirmation | Makes a file public to the instance |
| `eznit unshare` | file id | Makes it private again (same as `share --off`) |

> [!IMPORTANT]
> Everything that reads or writes your own rows needs the session from `login`.
> Only these need none: `signup`, `login`, and the **public** catalogue (`template
> list`, `template show`, `template apply` of a public template). Shared files can
> be read anonymously through the API and the web UI, but `eznit download` always
> needs a session.

```sh
eznit signup
eznit login
eznit upload      # file path: ./notes.txt
eznit list        # note the id
eznit download    # id: <id>   destination folder: ./out
```

## 🧩 Templates

| Command | Flags | What it does |
| --- | --- | --- |
| `eznit template publish [folder]` | `--public`, `--commands <id>=<path>` (repeatable or comma-separated) | Publishes a folder. `--commands` pulls a library command in. |
| `eznit template list` | `--kind <kind>`, `--mine` | The public catalogue plus your own. `--mine` shows only yours. |
| `eznit template show [id]` | | One template: its files and variables |
| `eznit template apply [id]` | `--target <dir>`, `--force` | Writes a template into a folder |
| `eznit template apply --from-file <bundle>` | `--target <dir>`, `--force` | Writes an exported bundle, with no server or session |
| `eznit template export [id]` | `-o <file>` (default `template.eznit`), `--force` | Writes a template to one file |
| `eznit template init` | `--kind <kind>`, `--target <dir>`, `--force` | Writes a starter locally, with no server |

**Kinds:** `opencode` `codex` `copilot` `cursor` `windsurf` `generic` (default)
`bash` `zsh` `fish` `powershell`. The shell kinds write one commented snippet
(`shell/bashrc.sh`, `shell/zshrc.sh`, `shell/config.fish`, `shell/profile.ps1`)
that does nothing until you uncomment an example.

<details>
<summary><b>📦 Offline bundles in detail</b></summary>

`template export` writes the whole template (files, binary bytes and variable
prompts) into one JSON file (`format: 1`). A command the template pulls in is
written at its target path, so the bundle is a snapshot to apply, not a publish
request. `apply --from-file` checks the same path rules and limits as the server
and asks for the variables on the machine that writes the files. A private
template needs a session to export; a public one does not.

</details>

**Limits** (the server enforces them): 256 KiB per text file, 4 MiB of text per
template, 10 MiB per binary file, and 32 MiB per template in total. All limits are on
[Templates and skills](https://github.com/fiwon123/eznit/wiki/Templates-and-Skills#-limits).

## 📑 Commands library

A command is a file kept once in your library and pulled into templates by id, so
updating it in one place updates every template that uses it.

| Command | Flags | What it does |
| --- | --- | --- |
| `eznit command add <file>` | `--name <name>`, `--not-executable`, `--replace` | Keeps a script in your library |
| `eznit command list` | | Lists your library |
| `eznit command show <name-or-id>` | | Shows one command |
| `eznit command remove <name-or-id>` | | Removes one. It is refused while a template still uses it. |
| `eznit command export --crower <file>` | `--force` | Writes the library to a crower TOML pack |
| `eznit command import --crower <file>` | `--replace` | Reads a crower TOML pack into the library |

```sh
eznit command add scripts/fmt.sh --name fmt
eznit template publish . --commands <command-id>=scripts/fmt.sh
```

<details>
<summary><b>🔁 How a crower pack maps</b></summary>

The pack is [crower](https://github.com/fiwon123/crower)'s TOML: one table per
command, keyed by name. eznit stores a script, so the mapping is narrow and
reported:

- on **export**, the script becomes `Exec`;
- on **import**, `Steps` win over `Exec` when present (that is how crower runs
  them); otherwise `Exec` becomes the script;
- aliases, env, shell, cwd and timeout have no eznit column, so the pack keeps
  them, the import names them, and nothing is guessed;
- a binary command cannot be exported, and it is named rather than dropped;
- import adds; `--replace` updates a name that is already there.

</details>

## 🐙 Git hosts

List the repositories a token can reach and clone one, on GitHub, GitLab or
Bitbucket Cloud.

| Host | `--provider` | Token needs | API base override |
| --- | --- | --- | --- |
| GitHub | `github` (default) | `repo` scope | `EZNIT_GITHUB_API` (Enterprise) |
| GitLab | `gitlab` | `read_api` (+ `read_repository` to clone) | `EZNIT_GITLAB_API` (instance root) |
| Bitbucket Cloud | `bitbucket` | repository / workspace read | `EZNIT_BITBUCKET_API` |

| Command | Flags | What it does |
| --- | --- | --- |
| `eznit repo auth` | `--provider`, `--token`, `--token-file`, `--remove` | Stores a token (asks for it by default). `--remove` forgets that host's token. |
| `eznit repo list` | `--provider` | Lists the repositories the token can reach |
| `eznit repo clone <owner/name>` | `--provider`, `--dir <path>` | Clones into `<dir>/<provider>/<owner>/<repo>` |

> [!TIP]
> **The token stays on your machine.** It is kept in `~/.eznit/config.json`, and
> the eznit API never sees it. The clone hands it to `git` through the child
> process's environment (`http.extraheader`). It is never on the command line, in
> the URL, or in the clone's `.git/config`.

A GitLab project is addressed by its full path (`group/subgroup/project`),
Bitbucket by `workspace/repository`. Bitbucket has no "all my repositories"
endpoint, so the listing walks each workspace the token can see.
`eznit logout` keeps the host token; `eznit repo auth --remove` removes it.

## 🔄 Updates

| Command | Flags | What it does |
| --- | --- | --- |
| `eznit update` | `--yes` | Installs the newest release |
| `eznit update --check` | | Says whether a newer release exists |
| `eznit update --install-version <tag>` | `--yes` | Installs a specific release, older ones included |
| `eznit update --revert` | `--yes` | Puts back the version the last install saved |

It downloads the platform's archive, `eznit_<tag>_<os>_<arch>`, and checks it
against the release's `checksums.txt` **before** replacing anything. The replaced
binary is kept beside the new one. The window's **About** panel has the same actions,
plus **Restart now**, and it installs `eznit-gui` from the same archive.

> [!NOTE]
> Only HTTPS is accepted, including redirects. The checksum protects the download
> in transit. It is not a signature, so it trusts GitHub's release storage.

## 🧾 The config file

`~/.eznit/config.json` (mode `0600`) is shared by the CLI and the window. Each key
is written on its own, and none erases another.

| Key | Holds |
| --- | --- |
| `token` | the session token |
| `github_token` · `gitlab_token` · `bitbucket_token` | a git-host token |
| `api_url` | the server the window talks to |
| `api_local` | whether that server is this machine |
| `server_dir` | the folder **Settings → Local server** drives |

`EZNIT_CONFIG` points it somewhere else (a test, a profile).

## 🌱 CLI environment variables

| Variable | Default | Meaning |
| --- | --- | --- |
| `CLI_API_HOST` | `http://localhost` | API host |
| `CLI_API_PORT` | `4000` | API port |
| `CLI_DOWNLOADS` | `./downloads/` | where `download` writes when you give no folder |
| `CLI_LOGS` | `./logs/` | CLI log folder |
| `EZNIT_CONFIG` | `~/.eznit/config.json` | the config file |
| `EZNIT_REPOS` | `~/eznit` | clone root when `repo clone` has no `--dir` |
| `EZNIT_GITHUB_API` | `https://api.github.com` | GitHub Enterprise API base |
| `EZNIT_GITLAB_API` | `https://gitlab.com` | GitLab instance root (`/api/v4` is added) |
| `EZNIT_BITBUCKET_API` | `https://api.bitbucket.org/2.0` | Bitbucket API base |

The CLI also loads `.env.local` and `.env` from the working directory, so a
development checkout can point it at another API with the same files the server
reads.

## 👉 Next

| | |
| --- | --- |
| 🧩 [Templates and skills](https://github.com/fiwon123/eznit/wiki/Templates-and-Skills) | the concepts behind the template commands |
| ⚙️ [Configuration](https://github.com/fiwon123/eznit/wiki/Configuration) | the server's variables and secrets |
| 🧯 [Troubleshooting](https://github.com/fiwon123/eznit/wiki/Troubleshooting) | when a command refuses |
