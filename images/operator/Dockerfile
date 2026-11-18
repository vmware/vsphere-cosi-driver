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

ARG BASE_IMAGE=photon:5.0

FROM docker.io/library/golang:1.26.7 AS builder

WORKDIR /build

COPY go.mod go.sum ./
COPY api/      api/
COPY cmd/      cmd/
COPY internal/ internal/

ENV CGO_ENABLED=0

RUN go build -ldflags="-w -s -extldflags=static" -o object-store-operator ./cmd/object-store-operator

FROM ${BASE_IMAGE}

COPY --from=builder /build/object-store-operator /bin/object-store-operator

ENTRYPOINT ["/bin/object-store-operator"]
