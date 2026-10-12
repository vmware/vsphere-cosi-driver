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

BOILERPLATE="hack/boilerplate.go.txt"
HEADER_LINES="$(wc -l < "${BOILERPLATE}")"

any_failure=false

for file in $(git ls-files -- '*.go'); do
  if ! head -n "${HEADER_LINES}" "${file}" \
      | sed -E '2s/^Copyright [0-9]{4} /Copyright YEAR /' \
      | cmp -s - "${BOILERPLATE}"; then
    echo "FAIL: ${file} -- missing or malformed license header (see ${BOILERPLATE})"
    any_failure=true
  fi
done

if ${any_failure}; then
  exit 1
fi
