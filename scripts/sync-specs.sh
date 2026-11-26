#!/usr/bin/env bash
# Copyright (c) 2026, WSO2 LLC. (http://www.wso2.com).
#
# WSO2 LLC. licenses this file to you under the Apache License,
# Version 2.0 (the "License"); you may not use this file except
# in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations
# under the License.

set -euo pipefail

if [[ $# -lt 1 || $# -gt 2 ]]; then
  echo "Usage: $0 <commit> [is_version]" >&2
  exit 1
fi

COMMIT="$1"
IS_VERSION="${2:-7.3.0}"
REPO="wso2/docs-is"
ROOT_SRC_PATH="en/identity-server/${IS_VERSION}/docs/apis/restapis"
ORG_SRC_PATH="en/identity-server/${IS_VERSION}/docs/apis/organization-apis/restapis"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
TARGET_DIR="${REPO_ROOT}/api-specs/wso2is/${IS_VERSION}"

sync_dir() {
  local src_path="$1"
  local dest_dir="$2"
  local api_url="https://api.github.com/repos/${REPO}/contents/${src_path}?ref=${COMMIT}"

  rm -rf "${dest_dir}"
  mkdir -p "${dest_dir}"

  local listing
  listing="$(curl -fsSL -H "Accept: application/vnd.github+json" "${api_url}")"

  local count=0
  while IFS=$'\t' read -r name download_url; do
    [[ -z "${name}" ]] && continue
    local raw_url="https://raw.githubusercontent.com/${REPO}/${COMMIT}/${src_path}/${name}"
    curl -fsSL "${raw_url}" -o "${dest_dir}/${name}"
    count=$((count + 1))
  done < <(python3 -c '
import json, sys
items = json.load(sys.stdin)
if not isinstance(items, list):
    sys.stderr.write("unexpected GitHub API response\n")
    sys.exit(1)
for item in items:
    if item.get("type") == "file" and (item["name"].endswith(".yaml") or item["name"].endswith(".yml")):
        name = item["name"]
        dl = item.get("download_url", "")
        print(f"{name}\t{dl}")
' <<< "${listing}")

  if [[ ${count} -eq 0 ]]; then
    echo "error: no spec files found at ${src_path} (${COMMIT})" >&2
    exit 1
  fi
  echo "Downloaded ${count} files to ${dest_dir}"
}

sync_dir "${ROOT_SRC_PATH}" "${TARGET_DIR}/root"
sync_dir "${ORG_SRC_PATH}" "${TARGET_DIR}/org"

cat > "${TARGET_DIR}/SOURCE" <<EOF
repository: https://github.com/${REPO}
commit: ${COMMIT}
root_path: ${ROOT_SRC_PATH}
org_path: ${ORG_SRC_PATH}
EOF

echo "Updated ${TARGET_DIR}/SOURCE"
