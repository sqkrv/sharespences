Sharespences
===

Sharespences is a comprehensive finance management app that tracks expenses, manages subscriptions, budgets for group trips, assists with cashbacks, and provides MCC information.

## Running with Docker

The stack needs a database password before it will start. There is no
default: this same file set is the production deployment descriptor, so a
default would be a credential in a public repository. Once, in a fresh
checkout:

```sh
printf 'POSTGRES_PASSWORD=%s\n' "$(openssl rand -hex 24)" > .env
```

`.env.example` documents that variable and the optional ones. A missing or
empty value fails the command by name rather than starting on a shared value.
Keep it URL-safe — it is interpolated into the connection string, where `@ :
/ ? #` and `%` corrupt the DSN instead of failing.

```sh
docker compose up --build
```

brings up PostGIS, applies migrations, loads seed data and serves the app
(API + embedded SPA) on <http://localhost:8080>. The OpenAPI document is at
`/openapi.json`; `DOCS=true` additionally serves the interactive reference at
`/docs`, which is off by default because it loads Stoplight Elements from
unpkg.com. Uploaded attachments and the database persist in named volumes.

Set `APP_PORT` (env or a `.env` file) to publish on a different host port,
e.g. `APP_PORT=9090 docker compose up` serves on <http://localhost:9090>.

The session cookie carries `Secure` by default, which browsers only return
over HTTPS — Safari will not store it at all over plain http, and sign-in then
fails with nothing visible to explain why. On a local http stack, opt out
per-machine rather than editing the compose file:

```bash
echo COOKIE_SECURE=false >> .env
```

The database port is **not** published to the host — the app, migrate and
seed containers reach it over the internal network — so the stack runs on a
server that already has its own Postgres on 5432. For native development,
`docker-compose.override.yaml` (gitignored, present in this checkout)
publishes it on `127.0.0.1:5432` so you can point the binary at it:

```sh
docker compose up -d db   # override publishes 127.0.0.1:5432 locally
set -a && . ./.env && set +a   # the password chosen above
export DATABASE_URL=postgres://sharespences:$POSTGRES_PASSWORD@localhost:5432/sharespences
go run ./cmd/sharespences migrate
go run ./cmd/sharespences seed
go run ./cmd/sharespences serve   # web/: npm run dev proxies to :8080
```

If 5432 is taken locally too, change the host side in the override (e.g.
`"127.0.0.1:5433:5432"`) and the `DATABASE_URL` port to match.

### Filling a dev database

`cmd/devdata` generates a realistic cashback history for one user — bank
clients, cards, offer periods, menu rows drawn from each bank's seeded
catalog, dated selections and partner offers:

```sh
go run ./cmd/devdata -user <username> -months 18
```

It writes through `internal/cashback.Service`, not raw SQL, so slot limits,
period-overlap and selection-date rules apply exactly as they do to a real
user. Re-running is the intended workflow: existing periods are detected and
skipped, so the same command next month adds only next month. `-dry-run`
reports what it would write; `-seed` makes generation reproducible.

It refuses a non-loopback `DATABASE_URL` without `-confirm`, and it is not
part of the server image (the Dockerfile builds `./cmd/sharespences` only).
The rows it writes are indistinguishable from hand-entered ones — do not
point it at production.

## Deploying

The session cookie is marked `Secure`, so **the app must be served over
HTTPS** — behind a TLS-terminating reverse proxy is the expected shape. The
one escape hatch is `COOKIE_SECURE=false`, which `docker-compose.yaml` sets
because that stack serves plain http on localhost (Safari refuses to store a
Secure cookie there, and login would fail silently). A real deployment must
not set it.

A deployment supplies `POSTGRES_PASSWORD` through its own untracked `.env`,
generated on the server and never committed. The database reads it when the
volume is first initialized and ignores it afterwards, so changing it later
means an `ALTER ROLE` inside the running cluster as well as an `.env` edit —
the two have to move together or the app cannot authenticate.

The stack keeps itself up. `db`, `app` and `admin` carry `restart:
unless-stopped`, so they come back from a crash and after a host reboot,
while a stop taken deliberately for maintenance stays stopped across one —
`always` would instead bring the app back up against a database an upgrade
had only half finished. `migrate` and `seed` opt out explicitly: they are
one-shots, and the rest of the stack waits on them reporting completion.
Restarts follow process exit, so a container whose healthcheck reports
unhealthy while its process keeps running is not one of them.

Builds identify themselves. Pass the version at build time and it is served
at `GET /api/v1/version` and shown in «Сервисы» → «О приложении»:

```sh
docker build --build-arg VERSION="$(git describe --tags --always --dirty)" -t sharespences .
```

Versions are CalVer `vYYYY.M.N` with an unpadded month (`v2026.7.1`); an
unstamped build reports `dev`. API compatibility lives in the URL path
(`/api/v1`), never in the tag.

## Screenshot recognizer (optional)

The cashback period form can prefill itself from bank-app screenshots.
The feature is **off by default** — without configuration the endpoints
answer 503 and manual entry works as before. It needs a vision model
behind one of two backends:

| Variable            | Meaning                                              | Default                       |
|---------------------|------------------------------------------------------|-------------------------------|
| `VISION_BACKEND`    | `ollama` \| `anthropic`; empty = feature off         | *(empty)*                     |
| `VISION_MODEL`      | model name                                           | `qwen3-vl:4b` / `claude-opus-5` |
| `OLLAMA_HOST`       | Ollama base URL (ollama backend)                     | `http://localhost:11434`      |
| `ANTHROPIC_API_KEY` | API key (anthropic backend)                          | —                             |

Local-first setup: install [Ollama](https://ollama.com), `ollama pull
qwen3-vl:4b` (fits a 6 GB GPU), then `VISION_BACKEND=ollama`. Under
compose the variables pass through from the environment or `.env`; the
app container reaches an Ollama on the Docker host via the pre-mapped
`host.docker.internal` (`OLLAMA_HOST=http://host.docker.internal:11434`).
Expect roughly 2–3 minutes per screenshot on a small GPU — recognition
runs as a background job with a progress poll. On Pascal-era NVIDIA
cards set `OLLAMA_FLASH_ATTENTION=1` in the Ollama service environment,
or every request dies with a CUDA out-of-memory error.

## Acknowledgements

Bank logos and other third-party material, with sources and trademark notice:
[ACKNOWLEDGEMENTS.md](ACKNOWLEDGEMENTS.md).

- [MCC codes and descriptions](https://mcc-codes.ru)