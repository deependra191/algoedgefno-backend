// Package buildinfo exposes build-time metadata injected via -ldflags -X at image build time.
// Override at build time with:
//
//	go build -ldflags "-X github.com/deependra191/algoedgefno-backend/internal/buildinfo.AppVersion=1.2.3 \
//	  -X github.com/deependra191/algoedgefno-backend/internal/buildinfo.CommitSHA=$(git rev-parse HEAD) \
//	  -X github.com/deependra191/algoedgefno-backend/internal/buildinfo.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
//	  ./cmd/server
package buildinfo

// AppVersion is the semantic version of the application.
var AppVersion = "0.1.0"

// CommitSHA is the git commit SHA injected at build time.
var CommitSHA = "unknown"

// BuildTime is the UTC build timestamp injected at build time.
var BuildTime = "unknown"
