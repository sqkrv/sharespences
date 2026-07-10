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

For native development, start only the database and run the binary against it:

```sh
docker compose up -d db
export DATABASE_URL=postgres://sharespences:sharespences@localhost:5432/sharespences
go run ./cmd/sharespences migrate
go run ./cmd/sharespences seed
go run ./cmd/sharespences serve   # web/: npm run dev proxies to :8080
```

## Acknowledgements

- [MCC codes and descriptions](https://mcc-codes.ru)