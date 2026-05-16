FROM --platform=$BUILDPLATFORM golang:1.25-bookworm AS build

WORKDIR /src

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG APP_VERSION=0.1.0
ARG COMMIT_SHA=unknown
ARG BUILD_TIME=unknown

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
      -ldflags="-s -w -X github.com/deependra191/algoedgefno-backend/internal/buildinfo.AppVersion=${APP_VERSION} -X github.com/deependra191/algoedgefno-backend/internal/buildinfo.CommitSHA=${COMMIT_SHA} -X github.com/deependra191/algoedgefno-backend/internal/buildinfo.BuildTime=${BUILD_TIME}" \
      -o /out/server ./cmd/server && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
      -ldflags="-s -w -X github.com/deependra191/algoedgefno-backend/internal/buildinfo.AppVersion=${APP_VERSION} -X github.com/deependra191/algoedgefno-backend/internal/buildinfo.CommitSHA=${COMMIT_SHA} -X github.com/deependra191/algoedgefno-backend/internal/buildinfo.BuildTime=${BUILD_TIME}" \
      -o /out/sync ./cmd/sync && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -tags postgres \
      -ldflags="-s -w" \
      -o /out/migrate github.com/golang-migrate/migrate/v4/cmd/migrate

FROM debian:bookworm-slim AS runtime

LABEL org.opencontainers.image.source="https://github.com/deependra191/algoedgefno-backend"
LABEL org.opencontainers.image.description="AlgoEdgeFno backend API"

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates tzdata && \
    rm -rf /var/lib/apt/lists/* && \
    useradd --uid 10001 --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin appuser

WORKDIR /app

COPY --from=build /out/server /app/server
COPY --from=build /out/sync /app/sync
COPY --from=build /out/migrate /app/migrate
COPY migrations /app/migrations

USER appuser

EXPOSE 8080

CMD ["/app/server"]
