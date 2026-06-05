# Gateway Repository Setup & Push Checklist

Operator runbook for initializing the **Gateway** local Git repository, wiring it to
the remote `Gateway_Repository`, and publishing a **secret-free** initial push.

The Gateway is a **Go** service (Gin, sqlx, go-redis), cloned from the API and
reduced to the admin + health surface. It listens on **port 8081**.

> Scope: this document covers only repository configuration and the first push.
> Building, deploying, and reverse-proxy setup are covered by the Gateway Docker
> stack files (`Dockerfile`, `docker-compose.yml`) and the nginx server-block
> configs under `gateway/scripts/`.

- **Remote (`Gateway_Repository`):** `https://github.com/GTDGit/GTD_GATEWAY.git`
- **Local project root:** `Gerbang/gateway` (run every command below from here)
- **VPS deployment directory:** `/root/gateway`, mirroring the existing `/root/api`
  and `/root/admin` layout.
- **Current deployment target:** `dev-gateway.gtd.co.id` (development), whose nginx
  server block proxies to the Gateway container on `127.0.0.1:8081`.
  Production (`gateway.gtd.co.id`) is authored now and activated later.

> Credential rule (Requirements 15.4, 15.5): every credential in this document is
> referenced by its **environment-variable name only** (for example `DB_PASSWORD`,
> `JWT_SECRET`, `REDIS_PASSWORD`). Never write a real Secret_Value into this file,
> into any tracked file, or into a commit.

---

## 0. Prerequisites

- `git` installed and available on `PATH`.
- Go 1.23+ installed (`go version`) for the local build sanity check.
- Write access to `https://github.com/GTDGit/GTD_GATEWAY.git` (HTTPS credential or PAT).
- A terminal opened in the Gateway project root `Gerbang/gateway`.
- The Gateway Go source and Docker config already present in this directory.

Confirm you are in the right directory before doing anything else:

```bash
# Should print a path ending in /gateway (or \gateway on Windows)
pwd
ls -a
```

You should see at minimum: `go.mod`, `go.sum`, `Dockerfile`, `docker-compose.yml`,
`.dockerignore`, `.gitignore`, `.env.example`, the Go source folders
(`cmd/`, `internal/`, `pkg/`), and `scripts/` (including the nginx confs).

Optional quick build sanity check before publishing (Go, not npm):

```bash
# Must exit 0; this is the Go equivalent of an npm build check.
go build ./...
```

---

## 1. Initialize the local repository (if not already a repo)

```bash
# Only needed if Gerbang/gateway is not already a Git repository.
git rev-parse --is-inside-work-tree 2>/dev/null || git init
```

- If `git init` runs, it creates a fresh `.git/` in `Gerbang/gateway`.
- If the command reports `true`, the directory is already a repo — skip `git init`.

Optionally set the default branch name to `main`:

```bash
git branch -M main
```

---

## 2. Configure the `origin` remote (Requirement 6.2)

Point the local `origin` remote at the `Gateway_Repository`.

```bash
# Add origin if it does not exist yet:
git remote add origin https://github.com/GTDGit/GTD_GATEWAY.git

# If origin already exists but points elsewhere, update it instead:
git remote set-url origin https://github.com/GTDGit/GTD_GATEWAY.git

# Verify it resolves to the Gateway_Repository:
git remote -v
```

Expected output:

```
origin  https://github.com/GTDGit/GTD_GATEWAY.git (fetch)
origin  https://github.com/GTDGit/GTD_GATEWAY.git (push)
```

---

## 3. Confirm `.gitignore` covers all env files and secrets (Requirements 6.1, 6.4, 15.1)

Before staging anything, verify the ignore rules so no secret-bearing file can be
tracked. The committed `.gitignore` must exclude env files, local secrets, and Go
build output while keeping `.env.example` tracked.

```bash
# These should each print the file path = IGNORED (good):
git check-ignore -v .env
git check-ignore -v .env.local
git check-ignore -v .env.development.local
git check-ignore -v bin
git check-ignore -v gtd_gateway.exe

# This MUST print nothing (i.e. .env.example is NOT ignored and will be pushed):
git check-ignore -v .env.example || echo ".env.example is tracked (correct)"
```

The `.gitignore` in this directory excludes env files (`.env`, `.env.local`,
`.env.*.local`), local secrets (`keys/`, certs, `*.pem`), and Go build output
(`bin/`, `*.exe`, `*.test`, `*.out`). `.env.example` stays tracked. If any of the
`git check-ignore` checks above behave differently, fix `.gitignore` before
continuing.

---

## 4. What the initial push MUST include (Requirement 6.3)

The first push publishes everything needed to build and run the Gateway, and nothing
secret. Concretely:

**Include (tracked):**
- All Gateway **Go** source: `cmd/` (entrypoint `cmd/api`), `internal/`, `pkg/`.
- Go module manifests: `go.mod`, `go.sum`.
- Docker configuration: `Dockerfile` (Go multi-stage), `docker-compose.yml`,
  `.dockerignore`.
- Reverse-proxy configs: `scripts/nginx-dev-gateway.gtd.co.id.conf`,
  `scripts/nginx-gateway.gtd.co.id.conf` (and any other `scripts/nginx-*.conf`).
- Helper scripts under `scripts/` that contain no secrets.
- Environment template: `.env.example` (placeholders only).
- Repository hygiene: `.gitignore` (and `.gitattributes`).
- This deliverable: `REPO_SETUP.md`.

**Never include (must stay untracked):**
- Real environment files: `.env`, `.env.local`, `.env.*.local`.
- Any private keys or certificates: `*.pem` (for example the SSH key
  `devtdg.pem` or any RDS CA bundle copied locally) and the `certs/` and `keys/`
  directories — see Requirement 15.6.
- Compiled binaries / build output: `bin/`, `*.exe`, the compiled `gtd_gateway`
  binary, `*.test`, `*.out`.
- Any file containing a real Secret_Value (a real value for `DB_PASSWORD`,
  `JWT_SECRET`, `REDIS_PASSWORD`, any provider `*_CLIENT_SECRET` / `*_API_KEY` /
  `*_SECRET` / `*_WEBHOOK_SECRET`, etc.).

---

## 5. Stage files and review (Requirement 6.4)

Stage the intended files, then inspect exactly what is staged before committing.

```bash
# Stage everything that is NOT ignored:
git add -A

# Review the staged file list — confirm only intended files are present and
# that NO real env file (.env / .env.local), NO *.pem, and NO binary appears here:
git status
```

If `git status` shows any `.env` (other than `.env.example`), any `*.pem`, any
binary, or any other secret-bearing file as staged, **stop** and unstage it:

```bash
git restore --staged .env .env.local   # adjust to the offending path(s)
```

---

## 6. Pre-push staged-file secret scan (Requirements 6.4, 15.4, 15.5)

Run this scan on the **staged** content (not the working tree) so it reflects exactly
what would be pushed. The goal is to catch any real Secret_Value before it leaves the
workstation.

### 6.1 Review the staged diff by eye

```bash
# Full staged diff — skim for anything that looks like a real credential value:
git diff --cached

# List just the staged file paths:
git diff --cached --name-only
```

### 6.2 Confirm no env/secret/key/binary files are staged

```bash
# Should print ONLY .env.example (never .env or .env.local):
git diff --cached --name-only | grep -E '(^|/)\.env' || echo "no env files staged"

# Should print nothing — no private keys or certs may be staged:
git diff --cached --name-only | grep -E '\.pem$' && echo "PEM STAGED — STOP" || echo "no .pem staged"

# Should print nothing — no compiled binaries may be staged:
git diff --cached --name-only | grep -E '(^|/)bin/|\.exe$|^gtd_gateway$' && echo "BINARY STAGED — STOP" || echo "no binaries staged"
```

### 6.3 Grep staged content for credential variable names with non-placeholder values

Scan the staged blob content for the credential variable **names** used by the
Gateway. The intent is to ensure these names only ever appear with placeholder
values (the `your_*_here` style) and never with a real Secret_Value.

```bash
# Surface every staged file that mentions a credential variable name:
git diff --cached -G \
  'DB_PASSWORD|JWT_SECRET|CALLBACK_SECRET|REDIS_PASSWORD|CLIENT_SECRET|API_KEY|API_SECRET|WEBHOOK_SECRET|WEBHOOK_TOKEN|SERVER_KEY|PRIVATE_KEY' \
  --name-only

# Show the actual staged lines for the same patterns so values can be eyeballed.
# Each hit MUST be a placeholder (your_..._here / xxxxx) or a comment, never a real value:
git diff --cached | grep -nE \
  'DB_PASSWORD|JWT_SECRET|CALLBACK_SECRET|REDIS_PASSWORD|CLIENT_SECRET|API_KEY|API_SECRET|WEBHOOK_SECRET|WEBHOOK_TOKEN|SERVER_KEY|PRIVATE_KEY' \
  || echo "no credential-variable references in staged content"
```

### 6.4 Grep for the shape of real secrets (DB/JWT/Redis passwords, provider keys, PEM blocks)

Look for value shapes that indicate a leaked secret rather than a placeholder. Any
hit here is a **stop-and-remediate** signal.

```bash
# PEM private-key blocks must never be staged:
git diff --cached | grep -nE 'BEGIN (RSA |EC |OPENSSL |)PRIVATE KEY' \
  && echo "PRIVATE KEY MATERIAL STAGED — STOP" || echo "no private-key blocks staged"

# Common provider key prefixes (sandbox or live) — review any hit:
git diff --cached | grep -nE 'SB-Mid-(server|client)-|xnd_(development|production)_|sk_(live|test)_|gb_(live|test)_' \
  && echo "PROVIDER KEY-LIKE STRING STAGED — review" || echo "no provider key prefixes staged"

# Long high-entropy assignments that look like a real DB_PASSWORD / JWT_SECRET /
# REDIS_PASSWORD rather than a your_..._here placeholder (24+ chars of base64-ish text):
git diff --cached | grep -nE '(PASSWORD|SECRET|KEY|TOKEN)[[:space:]]*[=:][[:space:]]*[A-Za-z0-9+/]{24,}' \
  | grep -vE 'your_|_here|xxxxx|placeholder|example' \
  && echo "POSSIBLE REAL SECRET STAGED — STOP and inspect" || echo "no high-entropy secret assignments staged"
```

> Note on `.env.example`: it intentionally contains the credential variable **names**
> with placeholder values (`your_redis_password_here`, `your_jwt_secret_here`,
> `SB-Mid-server-xxxxx`, `xnd_development_xxxxx`). Those placeholders are expected and
> safe. The scan above is designed to pass on placeholders and flag only real values.

### 6.5 If the scan flags anything

1. **Do not commit or push.**
2. Unstage the offending file: `git restore --staged <path>`.
3. If the secret was only in the working tree, move it into an ignored env file
   (`.env` / `.env.local`) and reference it by variable name only.
4. Re-confirm `.gitignore` covers that path (Section 3), then re-run the scan.
5. If a real Secret_Value was ever committed locally, treat it as exposed:
   remove it from history before pushing and **rotate** that credential
   (for example issue a new value for `DB_PASSWORD` / `JWT_SECRET` /
   `REDIS_PASSWORD`).

---

## 7. Commit and push (Requirement 6.3)

Only after Sections 5 and 6 are clean:

```bash
git commit -m "chore: initial Gateway service (Go source, Docker, env template)"

# First push sets up the upstream tracking branch:
git push -u origin main
```

On success the `Gateway_Repository` contains all Gateway Go source, the Docker
configuration, the nginx confs, and `.env.example` — and no Secret_Value.

---

## 8. Post-push verification

```bash
# Confirm the remote branch exists and matches local:
git ls-remote --heads origin

# Sanity check the pushed tree does NOT contain real env files, keys, or binaries:
git ls-files | grep -E '(^|/)\.env(\.|$)' | grep -v '\.env\.example' \
  && echo "UNEXPECTED env file tracked — investigate" || echo "only .env.example tracked"
git ls-files | grep -E '\.pem$' && echo "UNEXPECTED .pem tracked — investigate" || echo "no .pem tracked"
git ls-files | grep -E '(^|/)bin/|\.exe$' && echo "UNEXPECTED binary tracked — investigate" || echo "no binaries tracked"

# Confirm the Go module + entrypoint are present in the pushed tree:
git ls-files | grep -E '^go\.(mod|sum)$'
git ls-files | grep -E '^cmd/api/'
```

- On GitHub, open the pushed repository and confirm `.env.example` is present while
  `.env` / `.env.local` are absent.
- Optionally confirm a fresh clone builds: `go build ./...` exits 0.

---

## 9. Deployment note (VPS layout)

The Gateway is deployed on the shared VPS from `/root/gateway`, mirroring the
existing `/root/api` and `/root/admin` directories. The current target is the
development host `dev-gateway.gtd.co.id`, whose nginx server block proxies to the
Gateway container on `127.0.0.1:8081` (see
`scripts/nginx-dev-gateway.gtd.co.id.conf`). The production host
`gateway.gtd.co.id` is configured for later activation
(`scripts/nginx-gateway.gtd.co.id.conf`).

On the VPS, deployment pulls from the `Gateway_Repository`; real secrets are supplied
on the host via an untracked `.env` derived from `.env.example`, never from the
repository. The SSH key `devtdg.pem` and any RDS CA bundle stay on their respective
hosts and are **never** copied into the repository or into a Docker image
(Requirement 15.6).

---

## Quick reference — ordered command sequence

```bash
# 0. From Gerbang/gateway — optional build sanity check:
go build ./...

# 1. Init (if needed) + default branch:
git rev-parse --is-inside-work-tree 2>/dev/null || git init
git branch -M main

# 2. Wire the remote:
git remote add origin https://github.com/GTDGit/GTD_GATEWAY.git   # or: git remote set-url origin <url>
git remote -v

# 3. Verify ignore coverage:
git check-ignore -v .env .env.local bin gtd_gateway.exe
git check-ignore -v .env.example || echo ".env.example tracked (correct)"

# 4. Stage + review:
git add -A
git status

# 5. Pre-push secret scan (must come back clean):
git diff --cached --name-only | grep -E '(^|/)\.env' || echo "no env files staged"
git diff --cached --name-only | grep -E '\.pem$' && echo "STOP" || echo "no .pem staged"
git diff --cached --name-only | grep -E '(^|/)bin/|\.exe$' && echo "STOP" || echo "no binaries staged"
git diff --cached | grep -nE 'BEGIN (RSA |EC |OPENSSL |)PRIVATE KEY' && echo "STOP" || echo "no keys"
git diff --cached | grep -nE '(PASSWORD|SECRET|KEY|TOKEN)[[:space:]]*[=:][[:space:]]*[A-Za-z0-9+/]{24,}' \
  | grep -vE 'your_|_here|xxxxx|placeholder|example' && echo "STOP" || echo "clean"

# 6. Commit + push:
git commit -m "chore: initial Gateway service (Go source, Docker, env template)"
git push -u origin main
```
