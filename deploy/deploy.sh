#!/usr/bin/env bash
# Ship the MVP to $DEPLOY_HOST:/opt/credit-and-bundle (Docker on the host, a reverse proxy in front of 127.0.0.1:18094).
set -euo pipefail
cd "$(dirname "$0")"
HOST=${DEPLOY_HOST:?set DEPLOY_HOST to the ssh host}
DIR=/opt/credit-and-bundle
[ -f .env.deploy ] || { echo "PG_PASSWORD=$(openssl rand -hex 16)" > .env.deploy; chmod 600 .env.deploy; }
# fallback SVGs embedded in the binary (shown when the PlantUML server is down)
../internal/web/diagrams/render.sh
(cd .. && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o deploy/bct ./cmd/server)
ssh "$HOST" "sudo mkdir -p $DIR && sudo chown \$(id -u):\$(id -g) $DIR && chmod 700 $DIR"
rsync -a bct Dockerfile docker-compose.yml "$HOST:$DIR/"
rsync -a .env.deploy "$HOST:$DIR/.env"
ssh "$HOST" "cd $DIR && docker compose up -d --build --remove-orphans && for i in \$(seq 1 30); do curl -fsS -o /dev/null http://127.0.0.1:18094/ && echo up && exit 0; sleep 1; done; docker compose logs --tail 40 app; exit 1"
