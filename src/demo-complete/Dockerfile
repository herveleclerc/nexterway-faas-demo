# ── Étape 1 : Compilation du binaire Go multi-plateforme ──────────────
FROM golang:1.22-alpine AS builder

WORKDIR /src

# Copie de la définition du module et pré-téléchargement des dépendances
COPY go.mod ./
RUN go mod download

# Copie du code source complet
COPY . .

# Compilation statique ultra-légère sans CGO
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -extldflags '-static'" \
    -o /bin/frontend \
    ./cmd/frontend

# ── Étape 2 : Image d'exécution minimale et sécurisée (Distroless) ────
FROM gcr.io/distroless/static-debian12:nonroot

LABEL maintainer="Herve Leclerc <herve.leclerc@alterway.fr>"
LABEL org.opencontainers.image.title="Nexterway FaaS Demo Frontend"

WORKDIR /app

# Copie du binaire compilé depuis l'étape précédente
COPY --from=builder /bin/frontend /app/frontend

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/app/frontend"]
