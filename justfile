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
composeup:
    @echo "docker compose up..."
    docker-compose -f ./infra/docker-compose.yml up -d
composedown:
    @echo "docker compose down..."
    docker-compose -f ./infra/docker-compose.yml down -v
composestop:
    @echo "docker stopping services..."
    docker-compose stop
