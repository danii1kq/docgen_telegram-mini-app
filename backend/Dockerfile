# SruNal KZ backend image.
#
# The API spawns the document-generation agent (Cline CLI) as a child process, so
# the runtime image carries the whole toolchain the skills need:
#   * the Go binaries (api + bot)
#   * Node 22 + Cline CLI            -> the AI agent
#   * Python 3 + python-docx/pptx    -> the document builders used by the skills
#   * pandoc                         -> conversions the skills may use
#   * the global skills (satbayev-gost, pptx) baked in at /app/skills
#
# Build:  docker build -t srunal-api .
# Run:    docker compose up -d          (see docker-compose.yml)

# ---------------------------------------------------------------- build stage
FROM golang:1.27-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY skills ./skills
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o /out/srunal-api ./cmd/api \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o /out/srunal-bot ./cmd/bot

# -------------------------------------------------------------- runtime stage
FROM node:22-bookworm-slim

# Toolchain of the generation agent.
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      python3 python3-pip python3-venv pandoc ca-certificates curl tzdata unzip \
 && rm -rf /var/lib/apt/lists/*

# Document libraries the skills use (Debian 12 needs the PEP 668 flag).
RUN pip3 install --no-cache-dir --break-system-packages python-docx python-pptx lxml

# The AI agent CLI.
RUN npm install -g cline@latest && cline --version

WORKDIR /app
COPY --from=build /out/srunal-api /app/bin/srunal-api
COPY --from=build /out/srunal-bot /app/bin/srunal-bot
COPY --from=build /src/skills /app/skills

# Sensible container defaults; everything can be overridden from the environment.
ENV APP_ENV=prod \
    HTTP_ADDR=:8080 \
    STORAGE_DIR=/data/storage \
    GENERATOR_DIR=/data/generator \
    SKILL_SOURCE_DIR=/app/skills \
    GENERATOR_MODE=agent \
    GENERATOR_COMMAND=cline \
    GENERATOR_PROVIDER=opencode-go \
    GENERATOR_MODEL=deepseek-v4.1-flash \
    GENERATOR_THINKING=high \
    GENERATOR_CONFIG_DIR=/data/cline \
    GENERATOR_DATA_DIR=/data/cline \
    TZ=Asia/Almaty

VOLUME ["/data"]
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=25s --retries=3 \
  CMD curl -fsS http://127.0.0.1:8080/api/health || exit 1

# The bot runs inside the API process; migrations and the demo seed are applied
# on startup (SEED_ON_START).
ENTRYPOINT ["/app/bin/srunal-api"]
