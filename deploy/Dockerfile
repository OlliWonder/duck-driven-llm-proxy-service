# Build stage
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -o /bin/server ./cmd/server

# Runtime stage
FROM alpine:3.20
RUN adduser -D -u 10001 app
COPY --from=build /bin/server /usr/local/bin/server
COPY --chmod=0555 deploy/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
USER app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
