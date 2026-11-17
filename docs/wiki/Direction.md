<sub>🏠 [Home](https://github.com/fiwon123/eznit/wiki) › 🧭 Direction</sub>

# 🧭 Direction

**This page is the one place a promise is allowed.** Planned work is marked
**not implemented**, with its issue, and what has landed is listed with the
release that shipped it. The issues are the source of truth for both.

> [!NOTE]
> **The principle:** one file or one command per step, and the offline paths need no
> account. A setup tool that needs a server to hand a file to the machine next to it
> is solving the wrong problem.

## 🔭 Planned

| | Status | Issue |
| --- | --- | --- |
| 📱 **Device-to-device over [cthrone](https://github.com/fiwon123/cthrone)**: send a template bundle to a second machine (a laptop, a tablet, a machine on the same network) with no account and no server on either side | 🟡 not implemented: the most work of anything here | none yet |

> [!TIP]
> If a feature would change how you use eznit today,
> [open an issue](https://github.com/fiwon123/eznit/issues/new) and say so. This
> is a direction, not a schedule.

## ✅ Shipped

### 🛡️ Operating it

| Release | What |
| --- | --- |
| **v0.38.0** | **Deploying on a VPS**: `make backup` / `make restore` (database **and** uploads, with a safety backup before every restore), the database bound to `127.0.0.1` by default, `API_BIND` / `WEB_BIND` for a reverse proxy ([#388](https://github.com/fiwon123/eznit/issues/388)) |
| **v0.38.0** | The window uses **the operating system's file and folder dialogs**, with the built-in one as a fallback ([#392](https://github.com/fiwon123/eznit/issues/392)); **Reset** says how to reset another folder ([#390](https://github.com/fiwon123/eznit/issues/390)) |
| **v0.37.2** | Start and Reset refuse a database **another stack** holds on the port, instead of failing with `exit status 1` ([#385](https://github.com/fiwon123/eznit/issues/385)) |
| **v0.37.1** | Settings → Local server keeps the page margins, and the folder gets its own row ([#382](https://github.com/fiwon123/eznit/issues/382)) |

### 🖥️ The desktop window

| Release | What |
| --- | --- |
| **v0.37.0** | **Explore** and **Workspace** tabs (Workspace once signed in); **Log in / Log out**, **Settings** and **About** on the right; Settings holds **Server**, **Local server** and **Reset** ([#379](https://github.com/fiwon123/eznit/issues/379)) |
| **v0.36.0** | A template's files as a folder tree with a preview, file previews before download, Private ↔ Public after a confirmation ([#376](https://github.com/fiwon123/eznit/issues/376)) |
| **v0.35.0** | Tabs regrouped, with a Templates / Files / All filter, whole-row selection, Download on every row ([#373](https://github.com/fiwon123/eznit/issues/373)) |
| **v0.34.0** | **Publish template…** from a folder ([#367](https://github.com/fiwon123/eznit/issues/367)); **shared files** listed with Download, with no account needed ([#368](https://github.com/fiwon123/eznit/issues/368)) |
| **v0.26.0** | Templates in the window: browse, export the `.eznit` bundle ([#260](https://github.com/fiwon123/eznit/issues/260)), apply into a folder with the CLI's rules ([#259](https://github.com/fiwon123/eznit/issues/259)) |
| **v0.25.0** | The window **names its server** and is the one place to change it; a new host signs you out ([#248](https://github.com/fiwon123/eznit/issues/248)) |

### 🧩 Templates and the CLI

| Release | What |
| --- | --- |
| **v0.18.0** | **GitHub, GitLab and Bitbucket**: list and clone with a token that never reaches the API ([#188](https://github.com/fiwon123/eznit/issues/188)) |
| **v0.17.0** | **macOS archives**, amd64 and arm64, verified as Mach-O ([#187](https://github.com/fiwon123/eznit/issues/187)) |
| **v0.16.0** | **Shell starters**: `template init --kind bash\|zsh\|fish\|powershell` ([#186](https://github.com/fiwon123/eznit/issues/186)) |
| **v0.15.0** | **Crower packs**: `command export/import --crower` ([#185](https://github.com/fiwon123/eznit/issues/185)) |
| **v0.14.0** | **Offline bundles**: `template export` and `apply --from-file`, with no server and no account ([#184](https://github.com/fiwon123/eznit/issues/184)) |
| earlier | `template init` offline starters; a command library pulled into templates by id; git-host tokens kept per host on the CLI machine |

## 👉 Next

| | |
| --- | --- |
| 🗺️ [Roadmap board](https://github.com/users/fiwon123/projects/12) | planned work, live |
| 📦 [Releases](https://github.com/fiwon123/eznit/releases) | every version and its notes |
| 🏠 [Home](https://github.com/fiwon123/eznit/wiki) | back to the start |
