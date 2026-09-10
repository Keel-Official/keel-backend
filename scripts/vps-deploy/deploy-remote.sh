#!/usr/bin/env bash
#
# deploy-remote.sh
#
# Runs ON THE HOST. Puts one published image live and nothing else.
#
# WHY THIS IS A FILE AND NOT TEN LINES OF YAML. Al has to be able to run the
# deploy by hand, on the box, when Actions is down or when the last deploy left
# something half applied. A deploy path that exists only inside a workflow is a
# deploy path nobody can execute or read. So the workflow's job is to reach the
# host and call this; the deploy itself is here.
#
#   bash scripts/vps-deploy/deploy-remote.sh <image-tag>
#
# THE VERSION THAT RUNS IS THE VERSION BEING DEPLOYED. The workflow checks the
# commit out on the host first and then calls this script from that checkout, so
# a change to this file ships with the commit that made it rather than one deploy
# later.
#
# WHAT IT DOES NOT DO:
#
#   - It does not migrate. `scripts/migrate.sh` is run by hand, for the reason
#     section 5 of RUNBOOK.md gives: an automatic migration is a deploy that
#     cannot be rolled back by pointing at the previous image, because the schema
#     has already moved.
#   - It does not write .env beyond the one image tag line. Every other value in
#     there is Al's and was set once, in section 3.4 of the runbook.
#   - It does not check that the deployment WORKS. That is a request from outside
#     the box, and the caller makes it: the workflow, or section 3.8 by hand.

set -euo pipefail

cd "$(dirname "$0")/../.."

COMPOSE_FILE_PATH="docker-compose.prod.yml"
TAG="${1:-}"

if [ -z "$TAG" ]; then
  echo "deploy: usage: bash scripts/vps-deploy/deploy-remote.sh <image-tag>" >&2
  echo "        normally a commit SHA. \`edge\` is the newest build of main." >&2
  exit 1
fi

if [ ! -f .env ]; then
  echo "deploy: no .env beside $COMPOSE_FILE_PATH in $(pwd)" >&2
  echo "        This is first-time setup and it is not this script's job." >&2
  echo "        See scripts/vps-deploy/RUNBOOK.md section 3.4." >&2
  exit 1
fi

# THE .env FILE IS THE RECORD OF WHAT IS LIVE, which is why the tag is written
# there rather than passed on the command line. A tag passed only to
# `docker compose up` would leave the file saying `edge` while something else
# ran, and the next person to type `docker compose up -d` by hand would silently
# change the running version. Rollback is then editing one line here.
if grep -q '^KEEL_IMAGE_TAG=' .env; then
  previous=$(grep '^KEEL_IMAGE_TAG=' .env | head -1 | cut -d= -f2-)
  # A temporary file and a move, so an interrupted write cannot leave a .env
  # with no image tag in it, which is a file `docker compose` refuses outright.
  sed "s|^KEEL_IMAGE_TAG=.*|KEEL_IMAGE_TAG=${TAG}|" .env > .env.next
  chmod --reference=.env .env.next 2>/dev/null || chmod 600 .env.next
  mv .env.next .env
else
  previous="(none recorded)"
  printf 'KEEL_IMAGE_TAG=%s\n' "$TAG" >> .env
fi

echo "deploy: was ${previous}, now ${TAG}"

docker compose -f "$COMPOSE_FILE_PATH" pull
docker compose -f "$COMPOSE_FILE_PATH" up -d
docker compose -f "$COMPOSE_FILE_PATH" ps

# Dangling and untagged layers, plus published images older than two weeks. A
# rollback further back than that re-pulls from ghcr.io, which works because
# every build is published under its commit SHA and none of them is deleted.
docker image prune -af --filter "until=336h" >/dev/null 2>&1 || true

echo "deploy: ${TAG} is up. Nothing here has checked that it SERVES: that is a"
echo "        request from outside the box. See RUNBOOK.md section 3.8."
