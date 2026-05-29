FROM --platform=$BUILDPLATFORM golang:1.25-bookworm AS build

WORKDIR /src

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG APP_VERSION=0.1.0
ARG COMMIT_SHA=unknown
ARG BUILD_TIME=unknown

COPY go.mod go.sum ./
RUN go mod download

# Copy only the Go source the build needs. Deliberately NOT `COPY . .`: a broad
# context copy can pull credential-like files (Firebase/GCP service-account
# JSONs, etc.) into the builder layer/cache even though they never reach the
# runtime image. cmd/ and internal/ are the only Go source trees; migrations/
# and scripts/ are copied explicitly into the runtime stage below. If a new
# top-level source dir is added, copy it here explicitly rather than reverting
# to `COPY . .` (a CI lint enforces this).
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
      -ldflags="-s -w -X github.com/deependra191/algoedgefno-backend/internal/buildinfo.AppVersion=${APP_VERSION} -X github.com/deependra191/algoedgefno-backend/internal/buildinfo.CommitSHA=${COMMIT_SHA} -X github.com/deependra191/algoedgefno-backend/internal/buildinfo.BuildTime=${BUILD_TIME}" \
      -o /out/server ./cmd/server && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
      -ldflags="-s -w -X github.com/deependra191/algoedgefno-backend/internal/buildinfo.AppVersion=${APP_VERSION} -X github.com/deependra191/algoedgefno-backend/internal/buildinfo.CommitSHA=${COMMIT_SHA} -X github.com/deependra191/algoedgefno-backend/internal/buildinfo.BuildTime=${BUILD_TIME}" \
      -o /out/sync ./cmd/sync && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
      -ldflags="-s -w" \
      -o /out/firebase-token ./cmd/firebase-token && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
      -ldflags="-s -w" \
      -o /out/setup-firebase-test-users ./cmd/setup-firebase-test-users && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
      -ldflags="-s -w" \
      -o /out/teardown-firebase-test-users ./cmd/teardown-firebase-test-users && \
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
COPY --from=build /out/firebase-token /app/firebase-token
COPY --from=build /out/setup-firebase-test-users /app/setup-firebase-test-users
COPY --from=build /out/teardown-firebase-test-users /app/teardown-firebase-test-users
COPY --from=build /out/migrate /app/migrate
COPY migrations /app/migrations
COPY scripts /app/scripts

USER appuser

EXPOSE 8080

CMD ["/app/server"]
