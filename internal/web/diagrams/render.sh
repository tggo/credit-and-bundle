#!/usr/bin/env bash
# Renders the fallback SVGs with the same PlantUML server (and fonts) as production: docker compose up -d plantuml.
set -euo pipefail
cd "$(dirname "$0")"
URL=${PLANTUML_URL:-http://localhost:18095}
for f in charge grants states domains migration purchase flow_current flow_changed; do
  python3 - "$f" <<'PY' | curl -fsS -X POST -H "Content-Type: text/plain; charset=utf-8" --data-binary @- "$URL/svg" -o "$f.svg"
import sys
src = open(sys.argv[1] + ".puml").read().replace("!include skin.iuml", open("skin.iuml").read(), 1)
sys.stdout.write(src)
PY
done
