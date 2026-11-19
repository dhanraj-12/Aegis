#!/usr/bin/env bash
set -e

# Usage: ./docker_push.sh <your-dockerhub-username>
DOCKER_USER="$1"

if [ -z "$DOCKER_USER" ]; then
    echo "Usage: ./docker_push.sh <dockerhub-username>"
    echo "Example: ./docker_push.sh dhanraj12"
    exit 1
fi

echo "=========================================================="
echo " Tagging and Pushing Aegis Images to Docker Hub: $DOCKER_USER"
echo "=========================================================="

echo "[-] Tagging images..."
docker tag aegis:latest "${DOCKER_USER}/aegis:latest"
docker tag aegis-backend:latest "${DOCKER_USER}/aegis-backend:latest"
docker tag aegis-traditional-proxy:latest "${DOCKER_USER}/aegis-traditional-proxy:latest"

echo "[-] Pushing ${DOCKER_USER}/aegis:latest..."
docker push "${DOCKER_USER}/aegis:latest"

echo "[-] Pushing ${DOCKER_USER}/aegis-backend:latest..."
docker push "${DOCKER_USER}/aegis-backend:latest"

echo "[-] Pushing ${DOCKER_USER}/aegis-traditional-proxy:latest..."
docker push "${DOCKER_USER}/aegis-traditional-proxy:latest"

echo ""
echo "=========================================================="
echo " All 3 images successfully pushed to Docker Hub!"
echo "   - ${DOCKER_USER}/aegis:latest"
echo "   - ${DOCKER_USER}/aegis-backend:latest"
echo "   - ${DOCKER_USER}/aegis-traditional-proxy:latest"
echo "=========================================================="
