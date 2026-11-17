<sub>🏠 [Home](https://github.com/fiwon123/eznit/wiki) › 🛡️ Operate › 🖥️ Deploying on a VPS</sub>

# 🖥️ Deploying on a VPS

**Run the Compose stack on a server you rent, then update it, back it up, move it
and change its password without losing data.**

> [!WARNING]
> **Pre-1.0 and unhardened.** Read the
> [security status](https://github.com/fiwon123/eznit#-security-status) first. A
> VPS is on the internet, so treat this as a test deployment until that section
> says otherwise.

| ✅ Safe any time | ⛔ Deletes data |
| --- | --- |
| `docker compose up -d --build` | `docker compose down -v` / `make docker_down_v` |
| `docker compose restart` / `stop` / `down` | `make dev-reset` |
| rebuilding or recreating any container | `docker volume rm …` / `docker volume prune` |
| a VPS reboot | `docker system prune --volumes` |
| `make backup` | the window's **Settings → Reset** (its local folder) |

## 🧠 The one thing to understand: where the data lives

Containers are disposable, and the data is not in them. It lives in two Docker
**named volumes**, which outlive every container:

```mermaid
flowchart LR
    subgraph c["Containers: rebuilt on every update"]
        API["⚙️ api"]
        DB["🐘 db"]
        WEB["🌐 web"]
        MIG["🔁 migrate"]
    end
    subgraph v["Volumes: your data, kept"]
        PG[("postgres-data<br/>accounts · records · templates")]
        AD[("api-data<br/>uploaded bytes")]
    end
    DB --- PG
    API --- AD
    API --> DB
    MIG --> DB
    WEB --> API

    classDef ctr fill:#dbeafe,stroke:#2563eb,color:#1e3a8a
    classDef vol fill:#fef3c7,stroke:#d97706,color:#78350f
    class API,DB,WEB,MIG ctr
    class PG,AD vol
```

| Volume | Holds |
| --- | --- |
| 🐘 `postgres-data` | the database: accounts, file records, templates, sessions |
| 📁 `api-data` | the uploaded **bytes** (`/data/uploads` in the `api` container) |

> [!IMPORTANT]
> A file record in the database points at bytes in `api-data`. **Back up and
> restore the two together.** A database without its uploads lists files that are
> gone.

## 🚀 First deploy

On the VPS, with Docker and the Compose plugin installed:

```sh
git clone https://github.com/fiwon123/eznit.git /srv/eznit
cd /srv/eznit
git checkout v0.38.0            # a release tag, not a moving branch
make setup                      # creates .env and secrets/db_password.txt
```

### 1️⃣ Set a strong database password, before the first start

Put it in `secrets/db_password.txt` (`chmod 600`).

> [!CAUTION]
> Postgres applies the password **only when it creates the volume**. Changing the
> file later does not change the database. That is a separate procedure,
> [below](#-changing-the-database-password).

### 2️⃣ Keep the ports off the internet

In `.env`:

```sh
API_BIND=127.0.0.1
WEB_BIND=127.0.0.1
TRUST_PROXY=true             # safe only because the API is on loopback
```

`DB_BIND` is already `127.0.0.1` by default. Docker's firewall rules bypass
`ufw`, so the bind address, not the firewall, is what keeps a published port
private.

### 3️⃣ Start it in the background

```sh
docker compose up -d --build db migrate api web
docker compose ps               # api "healthy", migrate "exited (0)"
```

### 4️⃣ Put HTTPS in front

The `web` service already proxies `/v1` to the API, so one site is enough. With
[Caddy](https://caddyserver.com/), which fetches the certificate itself:

```caddyfile
eznit.example.com {
    reverse_proxy 127.0.0.1:5173
}
```

```mermaid
flowchart LR
    U["👤 Browser / CLI"] -->|"HTTPS :443"| C["🔒 Caddy"]
    C -->|"127.0.0.1:5173"| W["🌐 web (nginx)"]
    W -->|"/v1"| A["⚙️ api :4000"]
    A --> D[("🐘 db<br/>127.0.0.1:5432")]
    classDef pub fill:#fee2e2,stroke:#dc2626,color:#7f1d1d
    classDef priv fill:#dcfce7,stroke:#16a34a,color:#14532d
    class U,C pub
    class W,A,D priv
```

### 5️⃣ Create your account straight away

> [!IMPORTANT]
> **The first account to sign up becomes the instance's admin.** Sign up yourself
> before you share the address.

## 🔄 Updating without touching the data

```sh
cd /srv/eznit
make backup                     # always: it turns any mistake into a restore
git fetch --tags && git checkout v0.38.1
docker compose up -d --build db migrate api web
```

`up -d --build` rebuilds the images and recreates only the containers that changed.
The volumes are kept. `migrate` applies only the migrations the database does not
have yet. Migrations are **append-only**, so an update adds to the schema and never
rewrites what is there.

> [!WARNING]
> **Never run a down migration on a deployment you care about.** A down migration
> drops columns and tables. To go back to an older release, restore the backup you
> took before the update.

## 💾 Backups

```sh
make backup                     # → backups/<UTC timestamp>/database.dump + uploads.tgz
```

`scripts/backup.sh` does the same without `make`. The stack must be running.

| File | What it is |
| --- | --- |
| 🐘 `database.dump` | a `pg_dump` custom-format dump made **inside** the `db` container, so the password is never read on the host. It is portable across Postgres versions and machines. |
| 📁 `uploads.tgz` | the contents of the uploads volume, with file ownership kept |

> [!CAUTION]
> **A backup on the same disk is not a backup.** Copy it off the machine, on a
> schedule.

```cron
# crontab -e: back up at 03:00, copy it off the machine, keep two weeks locally
0 3 * * * cd /srv/eznit && make backup >> backups/backup.log 2>&1 && rclone copy backups remote:eznit-backups
30 3 * * * find /srv/eznit/backups -mindepth 1 -maxdepth 1 -type d -mtime +14 -exec rm -rf {} +
```

> [!TIP]
> **Restore one once, on purpose, before you need it.** A backup you have never
> restored is a guess.

## ♻️ Restoring, moving, rebuilding

```sh
make restore BACKUP=backups/2026-11-17T030000Z
```

```mermaid
flowchart TD
    A["🔎 Check the folder<br/>database.dump + uploads.tgz"] --> B["🛟 Safety backup<br/>backups/before-restore/"]
    B --> C["⏸️ Stop api"]
    C --> D["🐘 Empty the schema,<br/>load the dump"]
    D --> E["📁 Empty the uploads,<br/>unpack the archive"]
    E --> F["🔁 Run migrate<br/>(older backup → current schema)"]
    F --> G["▶️ Start api"]
    A -.->|"missing file"| X["⛔ Stop: nothing changed"]
    B -.->|"fails"| X

    classDef step fill:#dbeafe,stroke:#2563eb,color:#1e3a8a
    classDef safe fill:#dcfce7,stroke:#16a34a,color:#14532d
    classDef stop fill:#fee2e2,stroke:#dc2626,color:#7f1d1d
    class A,C,D,E,F step
    class B,G safe
    class X stop
```

The restore **replaces** the current data, which is why it takes a safety backup
first and stops if that fails. If a later step fails, it still starts `api` again
and says so.

### 🚚 Moving to another server

1. Do a [first deploy](#-first-deploy) there (same release, its own password).
2. `docker compose up -d` once, so the volumes exist.
3. Copy the backup folder over.
4. `make restore BACKUP=…`

### 🧱 Rebuilding the database from zero

Use this for a corrupted volume, or for a new **major** Postgres version, which cannot
read an older version's data folder:

```sh
make backup
docker compose stop api
docker compose rm -sf db
docker volume rm eznit_postgres-data   # <project>_postgres-data; the project is `name:` in docker-compose.yml
docker compose up -d db migrate api web
make restore BACKUP=backups/<the one you just took>
```

## 🔐 Changing the database password

> [!CAUTION]
> Editing `secrets/db_password.txt` alone **breaks the deployment.** Postgres
> stored the old password when it created the volume, so the API and `migrate`
> would start sending a password it does not know.

Change it in the database **first**, then in the file:

```sh
docker compose exec db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
#   ALTER USER root PASSWORD 'the-new-password';
#   \q
# put the same value in secrets/db_password.txt (chmod 600), then:
docker compose up -d --force-recreate api
```

`psql` inside the container connects over the local socket, which needs no
password. That is also the way back in if the file and the database ever disagree.

## 👤 Admin and access

| | |
| --- | --- |
| 👑 **The first account is the admin** | Only an admin may list, change or delete other accounts. No route grants the flag. |
| 🐘 **The database user is a superuser** | `root` is the user the Postgres image creates, and the API connects as it. That is acceptable only while the database answers on loopback and the Compose network. |
| 🔑 **The server itself** | SSH keys only (`PasswordAuthentication no`, `PermitRootLogin no`), a normal user with `sudo`, unattended security updates, and a firewall allowing 22, 80 and 443. |
| 🙈 **Secrets stay out of git** | `.env` and `secrets/db_password.txt` are gitignored and mode `600`. Keep the password in a password manager, not next to the dump. |

Promote another account to admin:

```sh
docker compose exec db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "UPDATE users SET is_admin = true WHERE email = '\''someone@example.com'\'';"'
```

> [!WARNING]
> Never set `DB_BIND=0.0.0.0` on a VPS. It would put a Postgres superuser on the
> internet.

## 📦 The desktop window's local server

The release bundle's **Local server** runs the API as a process, not as the `api`
container, so its uploads are in the folder `API_UPLOADS` names (`./uploads` by
default), not in a volume. To back it up, dump the database the same way and
copy that folder:

```sh
docker compose exec -T db sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > database.dump
```

`make backup` and `make restore` are for the Compose deployment above.

## 👉 Next

| | |
| --- | --- |
| ⚙️ [Configuration](https://github.com/fiwon123/eznit/wiki/Configuration#-ports-and-bind-addresses) | every bind address and variable |
| 🧯 [Troubleshooting](https://github.com/fiwon123/eznit/wiki/Troubleshooting) | when the database or a port refuses |
| 📦 [Releases](https://github.com/fiwon123/eznit/wiki/Releases) | which tag to deploy, and how to verify it |
