# Current Operator version
VERSION ?= 0.0.1
# Default bundle image tag
BUNDLE_IMG ?= controller-bundle:$(VERSION)
# Options for 'bundle-build'
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# Certificate dir of Webhook
TEST_WEBHOOK_CERTS_DIR ?= /tmp/k8s-webhook-server/serving-certs

# Image URL to use all building/pushing image targets
IMG ?= controller:latest
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true,preserveUnknownFields=false"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: manager

# Run tests
ENVTEST_ASSETS_DIR=$(shell pwd)/testbin
test: generate fmt vet manifests
	mkdir -p ${ENVTEST_ASSETS_DIR}
	test -f ${ENVTEST_ASSETS_DIR}/setup-envtest.sh || curl -sSLo ${ENVTEST_ASSETS_DIR}/setup-envtest.sh https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/v0.7.0/hack/setup-envtest.sh
	source ${ENVTEST_ASSETS_DIR}/setup-envtest.sh; fetch_envtest_tools $(ENVTEST_ASSETS_DIR); setup_envtest_env $(ENVTEST_ASSETS_DIR); go test ./... -coverprofile cover.out

# Run e2e tests
E2E_ASSETS_DIR=$(shell pwd)/tests
e2e: generate fmt vet manifests
	$(E2E_ASSETS_DIR)/run_tests.sh

# Generate local debug certificate
HACK_DIR=$(shell pwd)/hack
cert: $(TEST_WEBHOOK_CERTS_DIR) $(TEST_WEBHOOK_CERTS_DIR)/tls.crt

$(TEST_WEBHOOK_CERTS_DIR):
	mkdir -p $(TEST_WEBHOOK_CERTS_DIR)

$(TEST_WEBHOOK_CERTS_DIR)/ca.key:
	openssl genrsa -out $(TEST_WEBHOOK_CERTS_DIR)/ca.key 1024

$(TEST_WEBHOOK_CERTS_DIR)/ca.crt: $(TEST_WEBHOOK_CERTS_DIR)/ca.key
	openssl req -x509 -new -nodes -key $(TEST_WEBHOOK_CERTS_DIR)/ca.key -subj "/C=CN/ST=Shanghai/O=daisy/CN=webhook" -sha256 -days 3650 -out $(TEST_WEBHOOK_CERTS_DIR)/ca.crt

$(TEST_WEBHOOK_CERTS_DIR)/tls.key:
	openssl genrsa -out $(TEST_WEBHOOK_CERTS_DIR)/tls.key 1024

$(TEST_WEBHOOK_CERTS_DIR)/tls.csr: $(TEST_WEBHOOK_CERTS_DIR)/tls.key
	openssl req -new -sha256 -key $(TEST_WEBHOOK_CERTS_DIR)/tls.key \
		-subj "/C=CN/ST=Shanghai/O=daisy/CN=host.docker.internal" \
   		-config config/debug/req.cnf \
   		-out $(TEST_WEBHOOK_CERTS_DIR)/tls.csr

$(TEST_WEBHOOK_CERTS_DIR)/tls.crt: $(TEST_WEBHOOK_CERTS_DIR)/tls.csr $(TEST_WEBHOOK_CERTS_DIR)/ca.crt $(TEST_WEBHOOK_CERTS_DIR)/ca.key
	openssl x509 -req -in $(TEST_WEBHOOK_CERTS_DIR)/tls.csr -CA $(TEST_WEBHOOK_CERTS_DIR)/ca.crt -CAkey $(TEST_WEBHOOK_CERTS_DIR)/ca.key \
	-extfile config/debug/san.cnf \
	-CAcreateserial -out $(TEST_WEBHOOK_CERTS_DIR)/tls.crt -days 3650 -sha256

#Setup Kind environment
kind:
	$(HACK_DIR)/patch-kind.sh
	$(HACK_DIR)/install-certmgr.sh

#Delete Kind environment
HACK_DIR=$(shell pwd)/hack
unkind:
	$(HACK_DIR)/delete-kind.sh

#register crds & webhook for local debug
debug: manifests kustomize
	$(HACK_DIR)/write-runtime-patches.sh
	$(KUSTOMIZE)  build config/debug | kubectl apply -f -

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run ./main.go

# Install CRDs into a cluster
install: manifests kustomize
	($(KUSTOMIZE)  build config/crd | kubectl replace -f -) || ($(KUSTOMIZE)  build config/crd | kubectl create -f -)

# Uninstall CRDs from a cluster
uninstall: manifests kustomize
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests kustomize
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl create -f -
yamla: manifests kustomize
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default >abc
# Copy webhook certs to local for debug
copy-running-certs:
	kubectl get secret webhook-server-cert -n daisy-operator-system -o json | sed 's/ca.crt/cacrt/; s/tls.crt/tlscrt/; s/tls.key/tlskey/' > secrets.json
	mkdir -p ./.running-keys
	jq -r .data.cacrt < secrets.json | base64 –decode > ./.running-keys/ca.crt
	jq -r .data.tlscrt < secrets.json | base64 –decode > ./.running-keys/tls.crt
	jq -r .data.tlskey < secrets.json | base64 –decode > ./.running-keys/tls.key
	mkdir -p /tmp/k8s-webhook-server/serving-certs
	cp ./.running-keys/* /tmp/k8s-webhook-server/serving-certs/
	rm secrets.json

# UnDeploy controller from the configured Kubernetes cluster in ~/.kube/config
undeploy:
	$(KUSTOMIZE) build config/default | kubectl delete -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen kustomize
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	$(KUSTOMIZE) build config/crd -o config/crd/tmp/crd.yaml

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Build the docker image
docker-build: test
	docker build -t ${IMG} .

# Push the docker image
docker-push:
	docker push ${IMG}

# Download controller-gen locally if necessary
CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen:
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.4.1)

# Download kustomize locally if necessary
KUSTOMIZE = $(shell pwd)/bin/kustomize
kustomize:
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v3@v3.9.1)

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

# Generate bundle manifests and metadata, then validate generated files.
.PHONY: bundle
bundle: manifests kustomize
	operator-sdk generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | operator-sdk generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	operator-sdk bundle validate ./bundle

# Build the bundle image.
.PHONY: bundle-build
bundle-build:
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .
