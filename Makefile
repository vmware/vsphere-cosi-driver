# Copyright 2026 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

all: build

BUILD_OUT ?= .build
BIN_OUT ?= $(BUILD_OUT)/bin

GOOS ?= linux
GOARCH ?= amd64

LDFLAGS := -extldflags "-static" -w -s

COSI_BIN_NAME := vsphere-cosi
COSI_BIN := $(BIN_OUT)/$(COSI_BIN_NAME).$(GOOS)_$(GOARCH)

.PHONY: build
build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags '$(LDFLAGS)' -o $(COSI_BIN) ./cmd/$(COSI_BIN_NAME)

# Python e2e suite (pip install -r test/e2e/requirements.txt once). Pass pytest arguments
# with ARGS, e.g. make test-e2e ARGS="-k name", and pick a config with CONFIG.
CONFIG ?= test/e2e/config/config.yaml

.PHONY: test-e2e test-e2e-unit
test-e2e:
	pytest test/e2e --ignore=test/e2e/unit --config $(CONFIG) $(ARGS)

# The suite's own tests; no cluster needed.
test-e2e-unit:
	pytest test/e2e/unit -m unit $(ARGS)

# Deploy the Supervisor layout (driver, upstream COSI, MinIO) to a local kind cluster; see
# hack/deploy-kind-cluster.sh -h. Extra flags: make local-dev ARGS="-c other".
.PHONY: local-dev local-dev-teardown
local-dev:
	hack/deploy-kind-cluster.sh $(ARGS)

local-dev-teardown:
	hack/deploy-kind-cluster.sh -t $(ARGS)

.PHONY: clean
clean:
	rm -rf $(BIN_OUT)/*

# controller-gen generates deepcopy code, CRDs and RBAC from the kubebuilder markers in Go.
# It is installed to bin/ and reinstalled only when the pinned version changes.
CONTROLLER_TOOLS_VERSION ?= v0.23.0
CONTROLLER_GEN := bin/controller-gen
CONTROLLER_GEN_STAMP := bin/.controller-gen-$(CONTROLLER_TOOLS_VERSION)

$(CONTROLLER_GEN_STAMP):
	GOBIN=$(CURDIR)/bin go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)
	touch $@

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN_STAMP)

# Writes zz_generated.deepcopy.go, the CRDs in config/crd and the operator RBAC in
# manifests/operator/rbac. Commit the output; make check fails when it is stale.
.PHONY: manifests
manifests: controller-gen
	$(CONTROLLER_GEN) object:headerFile=hack/boilerplate.go.txt,year=2026 paths=./api/... paths=./internal/...
	$(CONTROLLER_GEN) crd paths=./api/... output:crd:dir=config/crd
	$(CONTROLLER_GEN) rbac:roleName=object-store-operator paths=./internal/... paths=./cmd/... output:rbac:dir=manifests/operator/rbac

.PHONY: check fmt vet boilerplate manifests-drift
check: fmt vet boilerplate manifests-drift

manifests-drift: manifests
	@git diff --exit-code -- api internal config manifests/operator || \
		{ echo "FAIL: generated files are stale; commit the output of make manifests"; exit 1; }
	@test -z "$$(git ls-files --others --exclude-standard -- api internal config manifests/operator)" || \
		{ echo "FAIL: untracked generated files; git add the output of make manifests"; exit 1; }

fmt:
	hack/check-format.sh

vet:
	hack/check-vet.sh

boilerplate:
	hack/check-boilerplate.sh

.PHONY: images
images:
	hack/release.sh
