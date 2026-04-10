run:
    air
sqlc:
    sqlc generate
test:
    @echo "Testing..."
    go test ./... -v
migrateup:
    @echo "Full migrate up..."
    goose up
migratedown:
    @echo "Full migrate down..."
    goose down-to 0
