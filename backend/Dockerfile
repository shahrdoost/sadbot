# ---- build + test stage ----
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# gate: vet + unit tests must pass before binaries are built
# (dating flow_test skips without DATABASE_URL, so this is safe in the build container)
RUN go vet ./...
RUN go test ./...
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags='-s -w' -o /out/sadbot ./cmd/sadbot
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags='-s -w' -o /out/migrate ./cmd/migrate

# ---- runtime stage ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata postgresql-client
WORKDIR /app
COPY --from=build /out/sadbot /app/sadbot
COPY --from=build /out/migrate /app/migrate
COPY migrations /app/migrations
COPY deploy/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh
EXPOSE 8080
ENTRYPOINT ["/app/entrypoint.sh"]
