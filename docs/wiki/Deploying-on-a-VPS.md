# 🖥️ Deploying on a VPS

How to run the Compose stack on a server you rent, and how to update it, back it up,
move it and change its password **without losing data**.

> **Pre-1.0 and unhardened.** Read the security status in the
> [README](https://github.com/fiwon123/eznit#-security-status) first. A VPS is the
> internet; treat this as a test deployment until that section says otherwise.

## 🧠 The one thing to understand: where the data lives

Containers are disposable. The data is not in them. It is in two Docker **named
volumes**:

| Volume | Holds | Lost if… |
| --- | --- | --- |
| `postgres-data` | the database: accounts, file records, templates, sessions | the volume is removed |
| `api-data` | the uploaded **bytes** (`/data/uploads` in the `api` container) | the volume is removed |

A file record in the database points at bytes in `api-data`. **Back up and restore
the two together.** A database without its uploads lists files that are gone.

Rebuilding, recreating, restarting or upgrading a container leaves its volume alone.
Only these commands delete data:

| Command | Effect |
| --- | --- |
| `docker compose down -v` / `make docker_down_v` | removes **every** volume of the project |
| `make dev-reset` | the same, for the dev loop |
| `docker volume rm …` / `docker volume prune` / `docker system prune --volumes` | removes the volumes named or unused |
| the desktop window's **Settings → Reset** | empties the database and the uploads of its local folder |

Plain `docker compose down`, `stop`, `restart`, `up -d --build` and a VPS reboot are
all safe.

## 🚀 First deploy

On the VPS, with Docker and the Compose plugin installed:

```sh
git clone https://github.com/fiwon123/eznit.git /srv/eznit
cd /srv/eznit
git checkout v0.38.0            # a release tag, not a moving branch
make setup                      # creates .env and secrets/db_password.txt
```

1. **Set a strong database password before the first start.** Put it in
   `secrets/db_password.txt` (`chmod 600`). Postgres applies it **only when it
   creates the volume**; changing it later is a separate procedure (below).
2. **Keep the ports off the internet.** In `.env`:

   ```sh
   API_BIND=127.0.0.1
   WEB_BIND=127.0.0.1
   TRUST_PROXY=true             # safe only because the API is on loopback
   ```

   `DB_BIND` is already `127.0.0.1` by default. Docker's firewall rules bypass `ufw`,
   so the bind address, not the firewall, is what keeps a published port private.
3. **Start it in the background:**

   ```sh
   docker compose up -d --build db migrate api web
   docker compose ps               # api "healthy", migrate "exited (0)"
   ```

4. **Put HTTPS in front.** The `web` service already proxies `/v1` to the API, so one
   site is enough. With [Caddy](https://caddyserver.com/), which fetches the
   certificate itself:

   ```
   eznit.example.com {
       reverse_proxy 127.0.0.1:5173
   }
   ```

5. **Create your account straight away.** The first account to sign up becomes the
   instance's **admin**, so sign up yourself before you share the address.

## 🔄 Updating without touching the data

```sh
cd /srv/eznit
make backup                     # always: it turns any mistake into a restore
git fetch --tags && git checkout v0.38.1
docker compose up -d --build db migrate api web
```

`up -d --build` rebuilds the images and recreates only the containers that changed.
The volumes stay as they are. `migrate` applies only the migrations the database does
not have yet. Migrations are append-only, so an update adds to the schema and never
rewrites what is there.

**Never run a down migration on a deployment you care about.** A down migration drops
columns and tables. To go back to an older release, restore the backup you took
before the update.

## 💾 Backups

```sh
make backup                     # → backups/<UTC timestamp>/database.dump + uploads.tgz
```

`scripts/backup.sh` does the same without `make`. It needs the stack running.

- `database.dump` is a `pg_dump` custom-format dump made inside the `db` container,
  so the password is never read on the host. It is portable across Postgres versions
  and across machines.
- `uploads.tgz` is the contents of the `api-data` uploads folder, with file ownership
  kept.

**A backup on the same disk is not a backup.** Copy it off the machine, then run it on
a schedule. For example, `crontab -e`:

```cron
# 03:00 daily: back up, copy off the machine, keep two weeks locally
0 3 * * * cd /srv/eznit && make backup >> backups/backup.log 2>&1 && rclone copy backups remote:eznit-backups
30 3 * * * find /srv/eznit/backups -mindepth 1 -maxdepth 1 -type d -mtime +14 -exec rm -rf {} +
```

**Restore one once, on purpose, before you need it.** A backup you have never
restored is a guess.

## ♻️ Restoring, moving, rebuilding

```sh
make restore BACKUP=backups/2026-11-17T030000Z
```

The restore:

1. checks the folder holds both files, before touching anything;
2. takes a **safety backup** of the current data into `backups/before-restore/`, and
   stops if that fails;
3. stops `api`, empties the database's schema and loads the dump;
4. empties the uploads volume and unpacks the archive;
5. runs `migrate`, so a backup from an older release is brought up to this release's
   schema;
6. starts `api` again (also when a step fails, and then it says so).

It **replaces** the current data. That is why it takes the safety backup first.

**Moving to another server:** do a first deploy there (same release, its own
password), `docker compose up -d` once so the volumes exist, copy the backup folder
over, then run `make restore BACKUP=…`.

**Rebuilding the database from zero** (a corrupted volume, or a new major Postgres
version, which cannot read an older version's data folder):

```sh
make backup
docker compose stop api
docker compose rm -sf db
docker volume rm eznit_postgres-data   # the project name, from `name:` in docker-compose.yml
docker compose up -d db migrate api web
make restore BACKUP=backups/<the one you just took>
```

## 🔐 Changing the database password

Editing `secrets/db_password.txt` alone **breaks the deployment.** Postgres stored the
old password when it created the volume, and the API and `migrate` would start sending
the new one. Change it in the database first, then in the file:

```sh
docker compose exec db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
#   ALTER USER root PASSWORD 'the-new-password';
#   \q
# put the same value in secrets/db_password.txt (chmod 600), then:
docker compose up -d --force-recreate api
```

`psql` inside the container connects over the local socket, which needs no password.
That is also the way back in if the file and the database ever disagree.

## 👤 Admin and access

- **The first account is the admin.** Only an admin may list, change or delete other
  accounts. No route grants the flag. Promote another account by hand:

  ```sh
  docker compose exec db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "UPDATE users SET is_admin = true WHERE email = '\''someone@example.com'\'';"'
  ```

- **The database user is a superuser.** `root` here is the Postgres user the image
  creates, and the API connects as it. That is acceptable while the database answers
  only on loopback and the Compose network. Never set `DB_BIND=0.0.0.0` on a VPS.
- **The server itself:** SSH keys only (`PasswordAuthentication no`,
  `PermitRootLogin no`), a normal user with `sudo` for daily work, unattended security
  updates, and a firewall that allows 22, 80 and 443.
- **Secrets stay out of git.** `.env` and `secrets/db_password.txt` are gitignored and
  must be mode `600`. Keep a copy of the password with the backups, in a password
  manager rather than next to the dump. A restore into a new deployment uses that
  deployment's own password, so the dump alone does not need it.

## 📦 The desktop window's local server

The release bundle's **Local server** runs the API as a process, not as the `api`
container. Its uploads are in the folder that `API_UPLOADS` names (`./uploads` by
default), not in a volume. To back that up, dump the database the same way
(`docker compose exec -T db sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > database.dump`)
and copy the uploads folder. `make backup` and `make restore` are for the Compose
deployment above.
