#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
: "${IMAGE_REPO:?Set IMAGE_REPO to the destination registry/repository}"
printf '%s\n' "$IMAGE_REPO" | grep -Eq '^[a-zA-Z0-9./:_-]+$' || { echo 'Invalid IMAGE_REPO' >&2; exit 1; }
[ -z "$(git status --porcelain)" ] || { echo 'Commit the release source before publishing; the working tree must be clean.' >&2; exit 1; }
sh ./docker/verify-flow.sh
revision=$(git rev-parse HEAD)
mkdir -p .deploy
umask 077
metadata=$(mktemp .deploy/build-metadata.XXXXXX)
trap 'rm -f "$metadata"' EXIT
trap 'exit 130' HUP INT TERM
docker buildx build --file docker/Dockerfile --platform "${PLATFORM:-linux/amd64}" --tag "$IMAGE_REPO:$revision" \
  --label "org.opencontainers.image.revision=$revision" --metadata-file "$metadata" --push .
metadata_path="$(pwd)/$metadata"
digest=$(docker run --rm --network none --read-only --cap-drop ALL --security-opt no-new-privileges \
  --mount "type=bind,src=$metadata_path,dst=/run/build-metadata.json,readonly" \
  node:22-alpine \
  node --input-type=module -e 'import fs from "node:fs"; const m=JSON.parse(fs.readFileSync("/run/build-metadata.json","utf8")); if(!/^sha256:[a-f0-9]{64}$/.test(m["containerimage.digest"])) process.exit(1); console.log(m["containerimage.digest"]);')
printf 'APP_IMAGE=%s@%s\n' "$IMAGE_REPO" "$digest" > .deploy/release.env
echo "Release published: $IMAGE_REPO@$digest"
echo 'Copy the APP_IMAGE value from .deploy/release.env into the server .deploy/prod.env, then run make deploy-prod there.'
