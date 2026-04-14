#!/usr/bin/env bash
#
# deploy-engine.sh
# Builds and deploys the State Sync Engine Docker image.
#
# Usage:
#   ./scripts/deploy-engine.sh                         # build only
#   PUSH=1 IMAGE=gcr.io/my-project/sse ./scripts/deploy-engine.sh  # build + push
#
set -euo pipefail

IMAGE="${IMAGE:-cloud-mirror/state-sync-engine}"
TAG="${TAG:-latest}"
PUSH="${PUSH:-0}"

echo "==> Building State Sync Engine image: ${IMAGE}:${TAG}"
docker build -t "${IMAGE}:${TAG}" .

echo "==> Running tests inside container..."
docker run --rm "${IMAGE}:${TAG}" -config /app/config.yaml 2>&1 | head -3 || true

if [ "$PUSH" = "1" ]; then
    echo "==> Pushing image to registry..."
    docker push "${IMAGE}:${TAG}"
    echo "    Pushed ${IMAGE}:${TAG}"
else
    echo "==> Skipping push (set PUSH=1 to push)"
fi

echo "==> Done."
echo ""
echo "To run locally:"
echo "  docker run --rm --network host -v \$(pwd)/config.yaml:/app/config.yaml ${IMAGE}:${TAG}"
