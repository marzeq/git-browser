# git-browser

Minimal web UI for browsing local Git repositories.

`git-browser` scans a directory for Git repositories directly under the configured root or one level below it in organization/user folders, and serves a small read-only interface for:

- listing repositories
- browsing trees and files at any branch, tag, or commit
- viewing repository and file commit history
- listing branches
- showing the top-level `README` file inline
- rendering inline `$...$` and display `$$...$$` math in Markdown with bundled KaTeX assets
- copying configured SSH and/or HTTPS clone URLs from the page header

Clone URLs are optional. Configure them with:

- `-clone-ssh-prefix=git@example.com:repos/`
- `-clone-https-prefix=https://example.com/repos/`

The repository name is appended directly to each configured prefix.

## Single-image Gitolite appliance

The included container image runs both services needed for a small Git host:

- OpenSSH and Gitolite on port 22 for authenticated clone, fetch, and push
- `git-browser` on port 8080 for read-only browsing
- one persistent `/var/lib/gitolite` volume for repositories, Gitolite configuration, authorized keys, logs, and SSH host keys

Gitolite runs as the unprivileged `git` account. Only the SSH daemon and the
small service supervisor run as root. Password login, root login, forwarding,
user-controlled SSH environments, tunnels, and user SSH startup files are
disabled.

### Start with Docker Compose

Create a dedicated administrator key if you do not already have one:

```sh
ssh-keygen -t ed25519 -f admin -C gitolite-admin
```

The private key stays on the administrator's machine. `admin.pub` is mounted as
a Docker secret and is only used when initializing a completely empty volume.

```sh
docker compose up -d --build
docker compose ps
git clone ssh://git@localhost:2222/gitolite-admin
```

The default deployment publishes SSH on all host interfaces at port 2222, but
publishes the unauthenticated web UI only on `127.0.0.1:8080`. Open
<http://127.0.0.1:8080> locally or put an authenticating reverse proxy in front
of it.

Set these values in the environment before starting Compose when the defaults
do not match the deployment:

| Variable | Default | Purpose |
| --- | --- | --- |
| `GITOLITE_ADMIN_NAME` | `admin` | Gitolite identity assigned to the first public key |
| `GITOLITE_SSH_PORT` | `2222` | Host port mapped to SSH |
| `GIT_BROWSER_HTTP_PORT` | `8080` | Loopback-only host port mapped to the browser |
| `GIT_BROWSER_CLONE_SSH_PREFIX` | `ssh://git@localhost:2222/` | Prefix shown in clone commands |

For a remote host, the clone prefix normally looks like
`ssh://git@git.example.com:2222/` or, when using port 22,
`git@git.example.com:`.

### Start with Docker directly

```sh
docker build -t git-browser-gitolite .
docker volume create gitolite-data
docker run -d \
  --name git \
  --restart unless-stopped \
  --pids-limit 128 \
  --read-only \
  --security-opt no-new-privileges \
  --tmpfs /run:size=16m,mode=0755 \
  --tmpfs /tmp:size=64m,mode=1777 \
  -p 2222:22 \
  -p 127.0.0.1:8080:8080 \
  -e GIT_BROWSER_CLONE_SSH_PREFIX=ssh://git@localhost:2222/ \
  -v gitolite-data:/var/lib/gitolite \
  -v "$PWD/admin.pub:/run/secrets/gitolite_admin_key:ro" \
  git-browser-gitolite
```

### Persistence and restarts

Initialization is idempotent:

- An empty volume requires a valid administrator public key and creates the
  `gitolite-admin` repository exactly once.
- An initialized volume is preserved. Each boot refreshes the bundled Gitolite
  executable, generated configuration, and hooks without replacing repositories
  or administrator configuration.
- Partial Gitolite state causes startup to fail rather than overwriting data.
- SSH host keys are stored in the same volume, so replacing the container does
  not change the server fingerprint.

Back up the `gitolite-data` volume as the `git` account or while the container
is stopped. Restoring that single volume restores repositories, access rules,
keys, audit logs, and the server's SSH identity.

The image uses uid/gid 1000 for the `git` account. Named Docker volumes work
without additional configuration. For an existing bind mount, set ownership to
1000:1000 before the first start. As an explicit recovery option,
`GITOLITE_FIX_PERMISSIONS=true` repairs ownership below the state directory once;
remove it after the container starts successfully.

Additional runtime variables are available when using the image directly:

| Variable | Default | Purpose |
| --- | --- | --- |
| `GITOLITE_ADMIN_KEY_FILE` | `/run/secrets/gitolite_admin_key` | First-boot public-key secret |
| `GIT_BROWSER_LISTEN` | `0.0.0.0:8080` | Browser listen address inside the container |
| `GIT_BROWSER_CLONE_HTTPS_PREFIX` | empty | Optional HTTPS clone prefix shown in the UI |
| `GIT_BROWSER_HIDE` | empty | Semicolon-separated repositories hidden from the UI |
| `GITOLITE_FIX_PERMISSIONS` | `false` | Repair state ownership before startup |

### Web authorization boundary

Gitolite permissions protect Git operations over SSH. The browser is deliberately
read-only but does **not** apply Gitolite's per-user access rules: every repository
it discovers is visible over HTTP. Keep port 8080 private, hide sensitive
repositories with `GIT_BROWSER_HIDE`, or enforce authentication and authorization
at a reverse proxy before exposing it to a network.
