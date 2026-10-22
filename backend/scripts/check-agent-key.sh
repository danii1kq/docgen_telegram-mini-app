#!/bin/sh
# Verifies that Cline picks up the opencode-go key from OPENCODE_API_KEY when the
# stored provider config is not available. The key is read from the container's
# own providers.json, so it never lands in the terminal output.
set -e

SETTINGS=/data/cline/settings/providers.json
BACKUP=/tmp/providers.json.bak

KEY=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["providers"]["opencode-go"]["settings"]["apiKey"])' "$SETTINGS")
if [ -z "$KEY" ]; then
  echo "no opencode-go key in $SETTINGS" >&2
  exit 2
fi

cp "$SETTINGS" "$BACKUP"
rm -f "$SETTINGS"

OPENCODE_API_KEY="$KEY" cline --json --auto-approve true --timeout 90 \
  -P opencode-go -m deepseek-v4.1-flash --thinking high \
  --config /data/cline --data-dir /data/cline \
  "Ответь ровно одним словом: ok" || STATUS=$?

mv "$BACKUP" "$SETTINGS"
exit ${STATUS:-0}
