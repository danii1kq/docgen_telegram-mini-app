# SruNal KZ — Telegram-first image: Go API + bot + worker and the built Mini App.
#
# Build from the repository root:
#   docker build -f deploy/Dockerfile -t srunal-tgbot .
# Or via compose (see deploy/docker-compose.yml), which is the normal path.

# ---------------------------------------------------------------- Mini App ---
FROM node:22-bookworm-slim AS web
WORKDIR /web
COPY miniapp/package.json ./
RUN npm install --no-audit --no-fund
COPY miniapp/ ./
RUN npm run build

# ------------------------------------------------------------- Go binaries ---
FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/cmd ./cmd
COPY backend/internal ./internal
COPY backend/skills ./skills
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o /out/srunal-api ./cmd/api \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o /out/srunal-bot ./cmd/bot

# ---------------------------------------------------------------- runtime ----
FROM node:22-bookworm-slim

# Toolchain of the document-generation agent (Cline + skills).
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      python3 python3-pip python3-venv pandoc ca-certificates curl tzdata unzip \
 && rm -rf /var/lib/apt/lists/*

RUN pip3 install --no-cache-dir --break-system-packages python-docx python-pptx lxml
# The agent CLI is pinned and never self-updates: a background update inside a
# running container replaces the binary under live workers (we hit exactly that
# in a test run: the symlink disappeared mid-generation).
ARG CLINE_VERSION=3.0.73
RUN npm install -g "cline@${CLINE_VERSION}" && cline --version

WORKDIR /app
COPY --from=build /out/srunal-api /app/bin/srunal-api
COPY --from=build /out/srunal-bot /app/bin/srunal-bot
COPY --from=build /src/skills /app/skills
COPY --from=web /web/dist /app/miniapp

ENV APP_ENV=prod \
    HTTP_ADDR=:8080 \
    STORAGE_DIR=/data/storage \
    GENERATOR_DIR=/data/generator \
    SKILL_SOURCE_DIR=/app/skills \
    MINIAPP_DIR=/app/miniapp \
    GENERATOR_MODE=agent \
    GENERATOR_COMMAND=cline \
    GENERATOR_PROVIDER=opencode-go \
    GENERATOR_MODEL=deepseek-v4.1-flash \
    GENERATOR_THINKING=high \
    GENERATOR_CONFIG_DIR=/data/cline \
    GENERATOR_DATA_DIR=/data/cline \
    CLINE_NO_AUTO_UPDATE=1 \
    NPM_CONFIG_UPDATE_NOTIFIER=false \
    NO_COLOR=1 \
    TZ=Asia/Almaty

VOLUME ["/data"]
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=25s --retries=3 \
  CMD curl -fsS http://127.0.0.1:8080/api/health || exit 1

ENTRYPOINT ["/app/bin/srunal-api"]
