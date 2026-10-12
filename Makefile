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

.PHONY: clean
clean:
	rm -rf $(BIN_OUT)/*

.PHONY: check fmt vet boilerplate
check: fmt vet boilerplate

fmt:
	hack/check-format.sh

vet:
	hack/check-vet.sh

boilerplate:
	hack/check-boilerplate.sh

.PHONY: images
images:
	hack/release.sh
