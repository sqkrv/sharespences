Sharespences
===

Sharespences is a comprehensive finance management app that tracks expenses, manages subscriptions, budgets for group trips, assists with cashbacks, and provides MCC information.

## Running with Docker

```sh
docker compose up --build
```

brings up PostGIS, applies migrations, loads seed data and serves the app
(API + embedded SPA) on <http://localhost:8080> — interactive API docs at
`/docs`. Uploaded attachments and the database persist in named volumes.

Set `APP_PORT` (env or a `.env` file) to publish on a different host port,
e.g. `APP_PORT=9090 docker compose up` serves on <http://localhost:9090>.

The database port is **not** published to the host — the app, migrate and
seed containers reach it over the internal network — so the stack runs on a
server that already has its own Postgres on 5432. For native development,
`docker-compose.override.yaml` (gitignored, present in this checkout)
publishes it on `127.0.0.1:5432` so you can point the binary at it:

```sh
docker compose up -d db   # override publishes 127.0.0.1:5432 locally
export DATABASE_URL=postgres://sharespences:sharespences@localhost:5432/sharespences
go run ./cmd/sharespences migrate
go run ./cmd/sharespences seed
go run ./cmd/sharespences serve   # web/: npm run dev proxies to :8080
```

If 5432 is taken locally too, change the host side in the override (e.g.
`"127.0.0.1:5433:5432"`) and the `DATABASE_URL` port to match.

## Acknowledgements

- [MCC codes and descriptions](https://mcc-codes.ru)