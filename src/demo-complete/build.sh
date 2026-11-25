#!/bin/bash

# build.sh - Script de build optimise pour Nexterway FaaS Demo Frontend

set -e

if [ -z "$1" ]; then
  echo "[ERREUR] L'argument IMAGE_TAG est obligatoire."
  echo "Usage: ./build.sh <IMAGE_TAG>"
  echo "Exemple: ./build.sh v1.0.0"
  exit 1
fi

IMAGE_TAG=$1
IMAGE_NAME="${IMAGE_NAME:-herveleclerc/demo-frontend}"
BUILDER="${BUILDER:-kube}"
PLATFORM="${PLATFORM:-linux/amd64}"
BUILD_CACHE="${IMAGE_NAME}:buildcache"

echo "[BUILD] Lancement du build Docker avec Buildx (Builder: ${BUILDER}, Platform: ${PLATFORM})..."

docker buildx build \
  --builder="${BUILDER}" \
  --platform="${PLATFORM}" \
  -t "${IMAGE_NAME}:${IMAGE_TAG}" \
  --push \
  --cache-from "type=registry,ref=${BUILD_CACHE}" \
  --cache-to "type=registry,ref=${BUILD_CACHE},mode=max" \
  -f Dockerfile .

if [ $? -eq 0 ]; then
  echo "[OK] Build et push reussis : ${IMAGE_NAME}:${IMAGE_TAG}"
  
  echo "[INFO] Mise a jour des manifestes Kubernetes avec le tag ${IMAGE_TAG}..."
  
  # Mise à jour du Deployment Frontend
  DEPLOYMENT_FILE="k8s/frontend-deployment.yaml"
  if [ -f "${DEPLOYMENT_FILE}" ]; then
    # Sauvegarde de précaution avant modification
    cp "${DEPLOYMENT_FILE}" "${DEPLOYMENT_FILE}.bak"
    
    # Remplacement sécurisé et portable (compatible Linux et macOS)
    sed "s|image: .*demo-frontend:.*|image: ${IMAGE_NAME}:${IMAGE_TAG}|" "${DEPLOYMENT_FILE}" > "${DEPLOYMENT_FILE}.tmp" \
      && mv "${DEPLOYMENT_FILE}.tmp" "${DEPLOYMENT_FILE}"
    echo "   - ${DEPLOYMENT_FILE} mis a jour avec l'image ${IMAGE_NAME}:${IMAGE_TAG}."
  fi

  echo "[TERMINE] Vous pouvez deployer la nouvelle version avec : kubectl apply -k k8s/"
else
  echo "[ERREUR] Docker build failed."
  exit 1
fi
