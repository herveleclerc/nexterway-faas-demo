# Configuration des variables
IMAGE_TAG      ?= latest
IMAGE_NAME     ?= herveleclerc/demo-frontend
IMAGE_URI      := $(IMAGE_NAME):$(IMAGE_TAG)
KUBECTL        ?= kubectl
NAMESPACE      ?= default

.PHONY: all help build test buildx docker-build docker-push deploy deploy-db undeploy status logs clean

all: build

help:
	@echo "Commandes disponibles pour demo-complete :"
	@echo "  make build         - Compile localement le serveur frontend Go"
	@echo "  make test          - Exécute les vérifications et tests unitaires Go"
	@echo "  make buildx        - Buildx optimisé (builder kube + push + mise à jour k8s) via build.sh"
	@echo "  make docker-build  - Construit l'image conteneur locale du frontend"
	@echo "  make docker-push   - Pousse l'image sur le registre d'images"
	@echo "  make deploy-db     - Déploie le cluster PostgreSQL CloudNativePG (1 instance)"
	@echo "  make deploy        - Déploie l'ensemble de l'application via Kustomize"
	@echo "  make undeploy      - Supprime les ressources déployées"
	@echo "  make status        - Affiche le statut du cluster CNPG et des pods"
	@echo "  make logs          - Affiche les logs du pod frontend"
	@echo "  make clean         - Supprime les binaires locaux"

build:
	@echo "==> Compilation du frontend Go..."
	go build -v -o bin/frontend ./cmd/frontend
	@echo "==> Binaire prêt : bin/frontend"

test:
	@echo "==> Exécution des tests Go..."
	go vet ./...

docker-build:
	@echo "==> Construction de l'image Docker $(IMAGE_URI)..."
	docker build -t $(IMAGE_URI) .

docker-push:
	@echo "==> Publication de l'image $(IMAGE_URI)..."
	docker push $(IMAGE_URI)

buildx:
	@echo "==> Lancement du build optimisé Buildx avec le tag $(IMAGE_TAG)..."
	./build.sh $(IMAGE_TAG)

deploy-db:
	@echo "==> Déploiement du cluster PostgreSQL CloudNativePG..."
	$(KUBECTL) apply -f k8s/cnpg-cluster.yaml -n $(NAMESPACE)
	@echo "==> En attente que le cluster PostgreSQL soit prêt..."
	@echo "Vérifiez avec : $(KUBECTL) get cluster.postgresql.cnpg.io pg-demo -n $(NAMESPACE) -w"

deploy:
	@echo "==> Déploiement complet via Kustomize dans le namespace $(NAMESPACE)..."
	$(KUBECTL) apply -k k8s/ -n $(NAMESPACE)

undeploy:
	@echo "==> Suppression des ressources déployées..."
	$(KUBECTL) delete -k k8s/ -n $(NAMESPACE) --ignore-not-found

status:
	@echo "==> Statut PostgreSQL CloudNativePG :"
	-$(KUBECTL) get cluster.postgresql.cnpg.io pg-demo -n $(NAMESPACE)
	@echo "\n==> Pods applicatifs :"
	-$(KUBECTL) get pods -l app.kubernetes.io/part-of=demo-complete -n $(NAMESPACE)
	@echo "\n==> Services :"
	-$(KUBECTL) get svc -l app.kubernetes.io/part-of=demo-complete -n $(NAMESPACE)

logs:
	$(KUBECTL) logs -l app.kubernetes.io/name=demo-frontend -n $(NAMESPACE) -f

clean:
	rm -rf bin/
