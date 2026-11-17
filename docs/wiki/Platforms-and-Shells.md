<sub>🏠 [Home](https://github.com/fiwon123/eznit/wiki) › 📖 Reference › 🐧 Platforms and shells</sub>

# 🐧 Platforms and shells

**What runs where, and what a template carries between machines.** The archives
cover Linux, Windows and macOS on amd64 and arm64. A template is plain files, so
you choose the shell or editor it configures.

## 📦 Released archives

| Platform | Archive | Verified how |
| --- | --- | --- |
| 🐧 Linux amd64 | `eznit_<tag>_linux_amd64.tar.gz` | ✅ built; the window is started in CI |
| 🐧 Linux arm64 | `eznit_<tag>_linux_arm64.tar.gz` | ⚪ built, never executed on ARM |
| 🪟 Windows amd64 | `eznit_<tag>_windows_amd64.zip` | ✅ CLI and window started in CI |
| 🪟 Windows arm64 | `eznit_<tag>_windows_arm64.zip` | ⚪ built, never executed on ARM |
| 🍎 macOS amd64 | `eznit_<tag>_darwin_amd64.tar.gz` | ⚪ Mach-O checked, never executed |
| 🍎 macOS arm64 | `eznit_<tag>_darwin_arm64.tar.gz` | ⚪ Mach-O checked, never executed |

One archive per platform holds the whole product: the client, its alias, the
server, the window, the migrations and the local stack. Every release has
`checksums.txt`. See [Releases](https://github.com/fiwon123/eznit/wiki/Releases)
for what each gate runs.

## 🧩 The three programs

| Program | What it is | Runs on |
| --- | --- | --- |
| ⚙️ `eznit` / `ez` | the CLI **and** the server (`eznit serve`) | every platform; `CGO_ENABLED=0` |
| 🖥️ `eznit-gui` | the desktop window (Fyne) | every platform, with a display and OpenGL |
| 🐳 the Compose stack | `db`, `migrate`, `api`, `web` | Linux containers (Docker Desktop on Windows and macOS) |

### 🖥️ The window

| Part | Does |
| --- | --- |
| **Explore** | public templates and shared files of the connected server, with no account needed |
| **Workspace** *(signed in)* | your files and templates: upload, publish, change visibility |
| **Settings → Server** | which server the window talks to: **Use this machine** or any address, **Test**, **Connect** |
| **Settings → Local server** | starts and stops the `eznit serve` beside the window, shows its log, edits its `.env` and password |
| **Settings → Reset** | erases the local server's data, after a confirmation |
| **About** | the version, updates, **Restart now** |

File and folder choices open **the operating system's own dialog**: Explorer on
Windows, the system panel on macOS, and zenity / qarma / matedialog on Linux. When
none is available, the window uses its built-in dialog instead.

> [!NOTE]
> **Linux runtime:** the window needs `libGL.so.1` and `libwayland-client.so.0`
> (`libgl1 libwayland-client0`) and glibc **2.34** or newer. It is linked inside a
> Debian 12 image, so Debian 12 and Ubuntu 22.04 run it. For the native file
> dialog, install `zenity` (most desktops already have it).

### ⚙️ The server

The product stack is Linux containers. No CI job starts the stack itself, on any
OS: "it runs there" rests on the Docker images, not on a gate. The server also
runs directly on the host (`./eznit serve`) with PostgreSQL reachable through
`DB_*`. That is the release-bundle path, and it is tested on Linux only.

## 🐚 Shells

eznit never detects or edits your shell. A template is a folder of files, written
exactly where you ask:

| Shell | `--kind` | Starter file | Load it from |
| --- | --- | --- | --- |
| bash | `bash` | `shell/bashrc.sh` | `~/.bashrc` (`source`) |
| zsh | `zsh` | `shell/zshrc.sh` | `~/.zshrc` (`source`) |
| fish | `fish` | `shell/config.fish` | `~/.config/fish/` (link or copy) |
| PowerShell | `powershell` | `shell/profile.ps1` | `$PROFILE` (dot-source) |

Each starter is a commented snippet: the project identity in a header, an example
of how to source it, and an example alias or function. **It does nothing until you
uncomment it.** A tool that edits `~/.bashrc` unasked is a tool people stop running.

> [!TIP]
> Applying a template **never** edits a file in your home directory unless you
> point `--target` there, and it never overwrites an existing file without
> `--force`.

## ↔️ What survives a round trip

| | Linux / macOS | Windows |
| --- | --- | --- |
| 📂 **Paths** | stored relative, with `/` | written with `\`: `shell\profile.ps1` |
| 📝 **Text** | UTF-8, line endings as published | the same, with no CRLF normalisation |
| ⚡ **Executable bit** | applied as mode `0755` | Windows has none; call the script through its interpreter |
| 🖼️ **Binary files** | byte for byte | byte for byte |
| 🔗 **Symlinks** | skipped at publish time | skipped at publish time |

## 🧭 Not proven

- The macOS archives are never executed on a Mac.
- The ARM64 binaries are never executed on ARM hardware.
- On Windows, `serve` is only asked for its help. The API itself is never started
  there.
- The browser UI has no automated test on any platform.

The README's
[What is not proven](https://github.com/fiwon123/eznit#%EF%B8%8F-what-is-not-proven)
has the full list.

## 👉 Next

| | |
| --- | --- |
| 🚀 [Getting Started](https://github.com/fiwon123/eznit/wiki/Getting-Started) | run it on your platform |
| 🧩 [Templates and skills](https://github.com/fiwon123/eznit/wiki/Templates-and-Skills) | what a template can hold |
| 🧯 [Troubleshooting](https://github.com/fiwon123/eznit/wiki/Troubleshooting#-window-startup) | the window will not start |
