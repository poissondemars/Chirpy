## Migrations

```sh
(cd sql/schema && goose postgres $CONNECTION up)
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