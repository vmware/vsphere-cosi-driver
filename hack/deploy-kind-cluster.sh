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


# deploy-kind-cluster.sh: build and deploy the vSphere COSI driver (plus the upstream COSI
# controller and sidecar) into a shared local kind cluster, and tear the driver down again.

set -o errexit
set -o nounset
set -o pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"

CLUSTER_NAME="cosi-cluster"
COSI_ROOT="${ROOT}/../container-object-storage-interface"
TEARDOWN=false
DELETE_CLUSTER=false

# Pinned: a mutable tag such as :latest is never re-pulled once a copy is on the node, so
# developers would drift onto different builds. The coollabsio fork is used because the
# upstream minio/minio images are no longer published. Use a tag, not a digest (see render_wrapper).
MINIO_IMAGE="${MINIO_IMAGE:-ghcr.io/coollabsio/minio:RELEASE.2025-10-15T17-29-55Z}"
# Optional `docker save` tarball, used when MINIO_IMAGE is not in the local Docker and
# cannot be pulled (public registries are blocked on some networks).
MINIO_IMAGE_TAR="${MINIO_IMAGE_TAR:-}"

# Upstream COSI components deployed alongside the driver. Their image tags are derived from
# the path lists below. The controller is placed as on Supervisor: Deployment cosi-controller
# in kube-system.
CONTROLLER_NS="kube-system"
CONTROLLER_DEPLOY="cosi-controller"
UPSTREAM_CONTROLLER_DEPLOY="container-object-storage-controller"
UPSTREAM_CONTROLLER_NS="container-object-storage-system"
CONTROLLER_BASE_IMAGE="gcr.io/k8s-staging-sig-storage/objectstorage-controller"
CONTROLLER_IMAGE="cosi-controller"
SIDECAR_IMAGE="cosi-provisioner-sidecar"
COSI_SHARED_PATHS=(client internal proto vendor go.mod go.sum)
CONTROLLER_PATHS=(controller "${COSI_SHARED_PATHS[@]}")
SIDECAR_PATHS=(sidecar "${COSI_SHARED_PATHS[@]}")

SIDECAR_WANT=""

DRIVER_NS="vmware-system-cosi"
DRIVER_DEPLOY="vsphere-cosi-controller"
DRIVER_IMAGE="vsphere-cosi"
DRIVER_PATHS=(cmd pkg go.mod go.sum images/driver/Dockerfile.dev)
BACKEND_SECRET="vsphere-cosi-s3-backend"
MINIO_NS="minio"
WRAPPER_DIR="${ROOT}/.build/deploy-kind-cluster"

log() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
skip() { printf '\033[1;32m ok\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mwarn:\033[0m %s\n' "$*" >&2; }
die() {
  printf '\033[1;31merror:\033[0m %s\n' "$*" >&2
  exit 1
}

# run_quiet <command...>: hides the command's output unless it fails.
run_quiet() {
  local out
  if ! out="$("$@" 2>&1)"; then
    printf '%s\n' "${out}" >&2
    return 1
  fi
}

usage() {
  cat <<USAGE
Usage: hack/deploy-kind-cluster.sh [options]
  -c, --cluster NAME     kind cluster name         (default: cosi-cluster)
  -r, --cosi-root PATH   upstream COSI repo root   (default: ../container-object-storage-interface)
  -t, --teardown         remove the vSphere driver (its namespace, including the backend Secret)
                         and MinIO, then exit
      --delete-cluster   with -t, also delete the kind cluster (other drivers on it are lost)
  -h, --help             show this help
USAGE
}

parse_args() {
  while (($#)); do
    case "$1" in
      -c | --cluster) CLUSTER_NAME="${2:?$1 requires a value}"; shift 2 ;;
      -r | --cosi-root) COSI_ROOT="${2:?$1 requires a value}"; shift 2 ;;
      -t | --teardown) TEARDOWN=true; shift ;;
      --delete-cluster) DELETE_CLUSTER=true; shift ;;
      -h | --help) usage; exit 0 ;;
      *) usage >&2; die "unknown option: $1" ;;
    esac
  done
  # Normalize when the directory exists; a missing default is reported when it is used.
  if [[ -d "${COSI_ROOT}" ]]; then
    COSI_ROOT="$(cd -- "${COSI_ROOT}" && pwd)"
  fi
}

check_dependencies() {
  local missing=() tool
  for tool in kind docker kubectl git make go; do
    command -v "${tool}" >/dev/null 2>&1 || missing+=("${tool}")
  done
  if ((${#missing[@]})); then
    die "missing required tools: ${missing[*]}. On macOS: brew install kind kubectl (docker, go and make from their usual installers)."
  fi
}

# sha256_stdin: stdin -> "<hex>  -" (the `shasum -a 256` format) with whichever tool exists.
sha256_stdin() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum
  else
    shasum -a 256
  fi
}

# image_tag <repo-root> <path>...
#   clean:  <short sha of the last commit touching <path>...>
#   dirty:  <that sha>-dirty-<8 hex of the uncommitted content>
image_tag() {
  local root="$1"
  shift
  local sha dirty
  sha="$(git -C "${root}" log -1 --format=%h -- "$@")"
  [[ -n "${sha}" ]] || sha="$(git -C "${root}" rev-parse --short HEAD)"
  dirty="$(
    {
      git -C "${root}" diff HEAD -- "$@"
      git -C "${root}" ls-files -z --others --exclude-standard -- "$@" |
        while IFS= read -r -d '' f; do
          printf '%s ' "${f}"
          sha256_stdin <"${root}/${f}"
        done
    } | sha256_stdin | cut -c1-8
  )"
  # sha256 of empty input: nothing uncommitted in scope.
  if [[ "${dirty}" == "e3b0c442" ]]; then
    echo "${sha}"
  else
    echo "${sha}-dirty-${dirty}"
  fi
}

kc() { kubectl --context "kind-${CLUSTER_NAME}" "$@"; }

# The command output is captured first: `cmd | grep -q` under pipefail can report "not found"
# when grep exits early and the writer dies of SIGPIPE.
cluster_exists() {
  local clusters
  clusters="$(kind get clusters 2>/dev/null)" || return 1
  grep -qx "${CLUSTER_NAME}" <<<"${clusters}"
}

# image_in_cluster <image>: true if the kind node already has the image. Unqualified names
# are stored by the node as docker.io/library/<name>.
image_in_cluster() {
  local images
  images="$(docker exec "${CLUSTER_NAME}-control-plane" crictl images 2>/dev/null | awk '{print $1 ":" $2}')" || return 1
  grep -Fxq -e "$1" -e "docker.io/library/$1" <<<"${images}"
}

# node_arch: the CPU architecture of the kind node (Go naming), so the driver binary matches.
node_arch() {
  local machine
  machine="$(docker exec "${CLUSTER_NAME}-control-plane" uname -m)" || die "cannot read the architecture of ${CLUSTER_NAME}-control-plane"
  case "${machine}" in
    aarch64 | arm64) echo arm64 ;;
    x86_64 | amd64) echo amd64 ;;
    *) die "unsupported kind node architecture: ${machine}" ;;
  esac
}

load_image() {
  local image="$1"
  if ! run_quiet kind load docker-image "${image}" --name "${CLUSTER_NAME}"; then
    warn "'kind load' failed; falling back to importing via the node's ctr"
    # shellcheck source=/dev/null
    source "${COSI_ROOT}/hack/kind-helpers.sh"
    KIND_CLUSTER_NAME="${CLUSTER_NAME}" run_quiet load_image_into_cluster "${image}"
  fi
}

# deployed_image <namespace> <deployment> <container>; empty if not deployed.
deployed_image() {
  kc -n "$1" get deploy "$2" \
    -o "jsonpath={.spec.template.spec.containers[?(@.name==\"$3\")].image}" 2>/dev/null || true
}

# Creates the cluster if it is missing.
setup_cluster() {
  if cluster_exists; then
    skip "kind cluster '${CLUSTER_NAME}' exists"
  else
    log "creating kind cluster '${CLUSTER_NAME}'"
    kind create cluster --name "${CLUSTER_NAME}"
  fi
}

# Makes MINIO_IMAGE available in the cluster node.
prepare_minio_image() {
  if image_in_cluster "${MINIO_IMAGE}"; then
    skip "MinIO image in cluster (${MINIO_IMAGE})"
    return
  fi
  if ! docker image inspect "${MINIO_IMAGE}" >/dev/null 2>&1; then
    if [[ -n "${MINIO_IMAGE_TAR}" ]]; then
      log "loading MinIO image from ${MINIO_IMAGE_TAR}"
      run_quiet docker load -i "${MINIO_IMAGE_TAR}" || die "cannot load ${MINIO_IMAGE_TAR}"
    else
      log "pulling ${MINIO_IMAGE}"
      run_quiet docker pull "${MINIO_IMAGE}" ||
        die "cannot pull ${MINIO_IMAGE}. Set MINIO_IMAGE to a reachable image, or MINIO_IMAGE_TAR to a 'docker save' tarball."
    fi
  fi
  log "loading ${MINIO_IMAGE} into kind"
  load_image "${MINIO_IMAGE}"
}

deploy_controller() {
  local tag want have
  tag="$(image_tag "${COSI_ROOT}" "${CONTROLLER_PATHS[@]}")"
  want="${CONTROLLER_IMAGE}:${tag}"
  have="$(deployed_image "${CONTROLLER_NS}" "${CONTROLLER_DEPLOY}" objectstorage-controller)"
  if [[ "${have}" == "${want}" ]]; then
    skip "controller up to date (${want})"
    return
  fi
  log "controller: ${have:-<not deployed>} -> ${want}"
  run_quiet make -C "${COSI_ROOT}" build.controller CONTROLLER_TAG="${want}"
  load_image "${want}"
  render_controller_wrapper "${want}"
  # A new tag changes the Deployment spec, so apply alone rolls the pod. The upstream
  # kustomization lives outside this repo, hence the load restrictor.
  kubectl kustomize --load-restrictor LoadRestrictionsNone "${WRAPPER_DIR}/controller" | kc apply -f -
  kc -n "${CONTROLLER_NS}" rollout status "deploy/${CONTROLLER_DEPLOY}" --timeout=180s
}

# Builds and loads the sidecar image if the node lacks it, and sets SIDECAR_WANT.
deploy_sidecar() {
  SIDECAR_WANT="${SIDECAR_IMAGE}:$(image_tag "${COSI_ROOT}" "${SIDECAR_PATHS[@]}")"
  if image_in_cluster "${SIDECAR_WANT}"; then
    skip "sidecar image in cluster (${SIDECAR_WANT})"
    return
  fi
  log "sidecar: building ${SIDECAR_WANT}"
  run_quiet make -C "${COSI_ROOT}" build.sidecar SIDECAR_TAG="${SIDECAR_WANT}"
  load_image "${SIDECAR_WANT}"
}

# Writes a kustomization that places the upstream controller in kube-system as
# cosi-controller, with the per-build image. The upstream manifests are reached through a
# symlink because kustomize does not accept absolute resource paths.
render_controller_wrapper() { # <controller-image-ref>
  local dir="${WRAPPER_DIR}/controller"
  mkdir -p "${dir}"
  ln -sfn "${COSI_ROOT}" "${dir}/upstream"
  cat >"${dir}/kustomization.yaml" <<KUSTOMIZATION
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - upstream/client/config/crd
  - upstream/controller
namespace: ${CONTROLLER_NS}
images:
  - name: ${CONTROLLER_BASE_IMAGE}
    newName: ${1%:*}
    newTag: ${1##*:}
patches:
  - target:
      kind: Deployment
      name: ${UPSTREAM_CONTROLLER_DEPLOY}
    patch: |-
      - op: replace
        path: /metadata/name
        value: ${CONTROLLER_DEPLOY}
      - op: add
        path: /spec/template/spec/containers/0/imagePullPolicy
        value: IfNotPresent
  # Patches run before the namespace override, so this drops upstream's own Namespace
  # object; ${CONTROLLER_NS} already exists.
  - patch: |-
      \$patch: delete
      apiVersion: v1
      kind: Namespace
      metadata:
        name: ${UPSTREAM_CONTROLLER_NS}
KUSTOMIZATION
}

# Writes a kustomization that applies hack/kind with the per-build image tags. The images
# are set by container name because the Supervisor manifest uses <placeholder> tags that the
# kustomize images transformer does not match.
render_wrapper() { # <driver-image-ref>
  local minio_name="${MINIO_IMAGE%:*}" minio_tag="${MINIO_IMAGE##*:}"
  mkdir -p "${WRAPPER_DIR}"
  cat >"${WRAPPER_DIR}/kustomization.yaml" <<KUSTOMIZATION
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../../hack/kind
images:
  - name: minio
    newName: ${minio_name}
    newTag: ${minio_tag}
patches:
  - target:
      kind: Deployment
      name: ${DRIVER_DEPLOY}
    patch: |-
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: ${DRIVER_DEPLOY}
      spec:
        template:
          spec:
            containers:
              - name: objectstorage-provisioner-sidecar
                image: ${SIDECAR_WANT}
              - name: vsphere-cosi-controller
                image: ${1}
KUSTOMIZATION
}

# build_driver_image <image-ref>: builds the driver binary on the host, for the kind node's
# architecture, and wraps it in an image.
build_driver_image() {
  local arch
  arch="$(node_arch)"
  run_quiet make -C "${ROOT}" build GOOS=linux GOARCH="${arch}"
  run_quiet docker build -f "${ROOT}/images/driver/Dockerfile.dev" \
    --build-arg BINARY="vsphere-cosi.linux_${arch}" --tag "$1" "${ROOT}/.build/bin"
}

# Applies hack/kind (driver, sidecar, MinIO). Images are only built when missing; the apply
# always runs because it is idempotent and also picks up manifest changes.
deploy_driver() {
  local want have
  want="${DRIVER_IMAGE}:$(image_tag "${ROOT}" "${DRIVER_PATHS[@]}")"
  have="$(deployed_image "${DRIVER_NS}" "${DRIVER_DEPLOY}" vsphere-cosi-controller)"
  log "driver: ${have:-<not deployed>} -> ${want}"
  if ! image_in_cluster "${want}"; then
    build_driver_image "${want}"
    load_image "${want}"
  fi
  render_wrapper "${want}"
  kc apply -k "${WRAPPER_DIR}"
  kc -n "${MINIO_NS}" rollout status deploy/minio --timeout=180s
  kc -n "${DRIVER_NS}" rollout status "deploy/${DRIVER_DEPLOY}" --timeout=180s
}

# Applies the Secret holding the S3 backend credentials. It runs before the driver rolls
# out, so a driver that consumes the Secret finds it, and it is re-applied every run so
# changed values take effect. The AWS_* keys mirror the design's admin ConfigSecret; nothing
# in pkg/cosi reads them yet, so treat them as scaffolding, not a contract. The credentials
# must match MINIO_ROOT_USER/MINIO_ROOT_PASSWORD in hack/kind/minio.yaml.
configure_backend() {
  log "applying Secret ${BACKEND_SECRET} (MinIO endpoint, in-cluster)"
  kc create namespace "${DRIVER_NS}" --dry-run=client -o yaml | kc apply -f - >/dev/null
  kc -n "${DRIVER_NS}" create secret generic "${BACKEND_SECRET}" \
    --from-literal=AWS_ENDPOINT_URL="http://minio.${MINIO_NS}.svc.cluster.local:9000" \
    --from-literal=AWS_REGION="us-east-1" \
    --from-literal=AWS_ACCESS_KEY_ID="minioadmin" \
    --from-literal=AWS_SECRET_ACCESS_KEY="minioadmin123" \
    --dry-run=client -o yaml | kc apply -f - >/dev/null
}

# Removes only what this script created. The cluster, upstream controller and other
# drivers stay unless --delete-cluster is given.
teardown() {
  if ! cluster_exists; then
    skip "kind cluster '${CLUSTER_NAME}' not present"
    return
  fi
  log "removing the vSphere driver and MinIO"
  kc delete namespace "${DRIVER_NS}" "${MINIO_NS}" --ignore-not-found --wait=true
  kc delete clusterrolebinding vsphere-cosi-controller-binding --ignore-not-found
  kc delete clusterrole vsphere-cosi-controller-role --ignore-not-found
  if ${DELETE_CLUSTER}; then
    warn "deleting kind cluster '${CLUSTER_NAME}': any other drivers on it are lost"
    kind delete cluster --name "${CLUSTER_NAME}"
  fi
}

main() {
  parse_args "$@"
  check_dependencies
  unset KUBECONFIG
  if ${DELETE_CLUSTER} && ! ${TEARDOWN}; then
    die "--delete-cluster only makes sense with -t"
  fi
  if ${TEARDOWN}; then
    teardown
    exit 0
  fi
  [[ -d "${COSI_ROOT}" ]] || die "upstream COSI repo not found at ${COSI_ROOT} (use -r)"
  [[ "${MINIO_IMAGE##*/}" == *:* ]] || die "MINIO_IMAGE must include a tag: ${MINIO_IMAGE}"

  setup_cluster
  prepare_minio_image
  deploy_controller
  deploy_sidecar
  configure_backend
  deploy_driver
  log "done. kubectl context: kind-${CLUSTER_NAME}"
}

main "$@"
