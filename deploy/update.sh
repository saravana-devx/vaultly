#!/bin/sh
# Deploy the latest code on the server:  sh deploy/update.sh
set -eu
git pull --ff-only
docker compose -f docker-compose.prod.yml up -d --build
docker compose -f docker-compose.prod.yml ps
