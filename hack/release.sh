#!/bin/bash

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

set -o errexit
set -o nounset
set -o pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

readonly IMAGE_NAME=vsphere-cosi

if [[ "$(git rev-parse --abbrev-ref HEAD)" =~ "main" ]]; then
  VERSION="$(git log -1 --format=%h)"
else
  VERSION="$(git describe --always 2>/dev/null)"
fi

ARCH=amd64

readonly BASE_IMAGE=photon:5.0

echo "building ${IMAGE_NAME}:${VERSION} for linux"
docker buildx build \
  --platform "linux/${ARCH}" \
  --output "type=docker" \
  --file images/driver/Dockerfile \
  --tag "${IMAGE_NAME}-linux-${ARCH}:${VERSION}" \
  --build-arg "BASE_IMAGE=${BASE_IMAGE}" \
  .
