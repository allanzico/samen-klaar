.DEFAULT_GOAL := help

# Optional values may be passed as make deploy-prod APP_IMAGE=registry/image@sha256:...
ifneq ($(APP_IMAGE),)
export APP_IMAGE
endif
ifneq ($(DOMAIN),)
export DOMAIN
endif
ifneq ($(ACME_EMAIL),)
export ACME_EMAIL
endif
ifneq ($(IMAGE_REPO),)
export IMAGE_REPO
endif
ifneq ($(PLATFORM),)
export PLATFORM
endif

.PHONY: help init-local init-prod deploy-local deploy-prod infra-local validate-local validate-prod migrate-local migrate-prod status-local status-prod logs-local logs-prod stop-local stop-prod backup-local backup-prod rollback-prod publish check

help:
	@echo 'make deploy-local                  Build and deploy local containers (localhost:8090)'
	@echo 'make init-prod DOMAIN=app.example.com ACME_EMAIL=you@example.com'
	@echo 'make deploy-prod APP_IMAGE=registry/samen-klaar@sha256:...  Deploy on the production server'
	@echo 'make publish IMAGE_REPO=registry/samen-klaar              Build and push an immutable release'
	@echo 'make infra-local                   Start PostgreSQL only'
	@echo 'make {validate,migrate,status,logs,stop,backup}-{local,prod}'
	@echo 'make rollback-prod                 Restore the previous app image'
	@echo 'make check                         Run Go and frontend checks'
	@echo 'make check-flow                    Run workflow integration tests with disposable PostgreSQL'
	@echo 'make seed-local                    Seed local test accounts, offers and availability'
	@echo 'make import-seed-auth0              Import their passwords into the configured Auth0 tenant'
	@echo 'make benchmark-local               Measure real AI routing and directory matching'

init-local init-prod deploy-local deploy-prod infra-local validate-local validate-prod migrate-local migrate-prod status-local status-prod logs-local logs-prod stop-local stop-prod backup-local backup-prod:
	@./docker/deploy.sh $(word 1,$(subst -, ,$@)) $(word 2,$(subst -, ,$@))

rollback-prod:
	@./docker/deploy.sh rollback prod

publish:
	@./docker/publish.sh

check:
	docker build --file docker/Dockerfile --tag samen-klaar-check:local .
	python3 docker/deploy_test.py

.PHONY: check-flow
check-flow:
	@sh ./docker/verify-flow.sh

.PHONY: seed-local benchmark-local import-seed-auth0
seed-local benchmark-local import-seed-auth0:
	@sh ./docker/data.sh $@
