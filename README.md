## Migrations

```sh
cd sql/schema
goose postgres <connection_string> up
```

## Queries

```sh
cd .
sqlc generate
```

```sh
go run .
go vet
```