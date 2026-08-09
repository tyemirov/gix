#!/usr/bin/env bash
set -euo pipefail

[[ $# -eq 0 ]] || { echo "error: make deploy accepts no arguments" >&2; exit 1; }
remote="origin"
branch="gh-pages"
version=""
url="https://gix.mprlab.com/"

required_commands=(awk cat cp curl find gh git head mkdir mktemp python3 rm shasum sleep tar)
for command_name in "${required_commands[@]}"; do
  command -v "${command_name}" >/dev/null 2>&1 || { echo "error: ${command_name} is required" >&2; exit 1; }
done

git_directory="$(git rev-parse --absolute-git-dir)"
prepared_manifest="${git_directory}/mprlab-release/manifest.json"
[[ -f "${prepared_manifest}" ]] || { echo "error: locally prepared release manifest is missing; run make release" >&2; exit 1; }
if [[ -z "${version}" ]]; then
  version="$(git tag --points-at HEAD --list 'v*' --sort=-version:refname | head -n 1)"
fi
[[ -n "${version}" ]] || { echo "error: no exact release tag at HEAD; run make release" >&2; exit 1; }

temporary_directory="$(mktemp -d)"
trap 'rm -rf "${temporary_directory}"' EXIT
download_directory="${temporary_directory}/download"
site_directory="${temporary_directory}/site"
checkout_directory="${temporary_directory}/checkout"
mkdir -p "${download_directory}" "${site_directory}"
gh release download "${version}" --pattern manifest.json --pattern pages.tar.gz --dir "${download_directory}"
archive="${download_directory}/pages.tar.gz"
downloaded_manifest="${download_directory}/manifest.json"
prepared_manifest_sha256="$(shasum -a 256 "${prepared_manifest}" | awk '{print $1}')"
downloaded_manifest_sha256="$(shasum -a 256 "${downloaded_manifest}" | awk '{print $1}')"
[[ "${downloaded_manifest_sha256}" == "${prepared_manifest_sha256}" ]] || { echo "error: published release manifest does not match the locally prepared release" >&2; exit 1; }
release_values="$(python3 - "${downloaded_manifest}" "${version}" <<'PY'
import json
import sys

manifest = json.load(open(sys.argv[1], encoding="utf-8"))
if manifest.get("schema_version") != 3 or manifest.get("artifact_kind") != "mprlab.release":
    raise SystemExit("published release manifest has an invalid contract")
if manifest.get("version") != sys.argv[2]:
    raise SystemExit("published release manifest has the wrong version")
asset = next((item for item in manifest["payloads"] if item["path"] == "payloads/release-assets/pages.tar.gz"), None)
if asset is None:
    raise SystemExit("published release has no Pages payload; run make release and make publish")
print(manifest["release_commit"])
print(manifest["source_commit"])
print(asset["sha256"])
PY
)"
release_commit="${release_values%%$'\n'*}"
remaining_values="${release_values#*$'\n'}"
source_commit="${remaining_values%%$'\n'*}"
expected_sha256="${remaining_values#*$'\n'}"
remote_tag_commit="$(git ls-remote --tags "${remote}" "refs/tags/${version}^{}" | awk 'NR == 1 {print $1}')"
if [[ -z "${remote_tag_commit}" ]]; then
  remote_tag_commit="$(git ls-remote --tags "${remote}" "refs/tags/${version}" | awk 'NR == 1 {print $1}')"
fi
[[ "${remote_tag_commit}" == "${release_commit}" ]] || { echo "error: published release manifest does not match remote tag ${version}" >&2; exit 1; }
actual_sha256="$(shasum -a 256 "${archive}" | awk '{print $1}')"
[[ "${actual_sha256}" == "${expected_sha256}" ]] || { echo "error: published Pages asset does not match make release" >&2; exit 1; }
python3 - "${archive}" <<'PY'
import pathlib
import sys
import tarfile

with tarfile.open(sys.argv[1], "r:gz") as archive:
    for member in archive.getmembers():
        path = pathlib.PurePosixPath(member.name)
        if path.is_absolute() or ".." in path.parts or member.issym() or member.islnk():
            raise SystemExit(f"unsafe Pages archive member: {member.name}")
PY
tar -xzf "${archive}" -C "${site_directory}"
python3 - "${site_directory}/.mprlab-release.json" "${version}" "${source_commit}" <<'PY'
import json
import sys

marker_path, version, source_commit = sys.argv[1:]
try:
    marker = json.load(open(marker_path, encoding="utf-8"))
except (OSError, json.JSONDecodeError):
    raise SystemExit(f"published Pages marker is invalid for source {source_commit}")
if marker.get("schema_version") != 1:
    raise SystemExit(f"published Pages marker has an invalid schema for source {source_commit}")
if marker.get("release_version") != version:
    raise SystemExit(f"published Pages marker has the wrong version for source {source_commit}")
if marker.get("source_commit") != source_commit:
    raise SystemExit(f"published Pages marker has the wrong source; expected source {source_commit}")
PY

remote_url="$(git remote get-url "${remote}")"
git clone --no-checkout "${remote_url}" "${checkout_directory}" >/dev/null
if git -C "${checkout_directory}" show-ref --verify --quiet "refs/remotes/origin/${branch}"; then
  git -C "${checkout_directory}" checkout -B "${branch}" "origin/${branch}" >/dev/null
else
  git -C "${checkout_directory}" checkout --orphan "${branch}" >/dev/null
fi
find "${checkout_directory}" -mindepth 1 -maxdepth 1 ! -name .git -exec rm -rf {} +
cp -R "${site_directory}"/. "${checkout_directory}/"
git -C "${checkout_directory}" add -A
pages_branch_changed="false"
if ! git -C "${checkout_directory}" diff --cached --quiet; then
  git -C "${checkout_directory}" -c user.name="MPR Lab Pages Deployer" -c user.email="pages-deployer@mprlab.invalid" commit -m "Deploy Pages for ${version}" >/dev/null
  git -C "${checkout_directory}" push origin "HEAD:refs/heads/${branch}"
  pages_branch_changed="true"
else
  echo "Pages branch already contains ${version} from source ${source_commit}."
fi
pages_commit="$(git -C "${checkout_directory}" rev-parse HEAD)"

pages_configuration_changed="false"
pages_configuration_path="${temporary_directory}/pages-configuration.json"
pages_configuration_error_path="${temporary_directory}/pages-configuration-error.txt"
if gh api repos/{owner}/{repo}/pages >"${pages_configuration_path}" 2>"${pages_configuration_error_path}"; then
    pages_configuration_state="$(python3 - "${pages_configuration_path}" "${branch}" <<'PY'
import json
import sys

configuration_path, expected_branch = sys.argv[1:]
try:
    configuration = json.load(open(configuration_path, encoding="utf-8"))
except (OSError, json.JSONDecodeError) as error:
    raise SystemExit(f"GitHub Pages configuration response is invalid: {error}")

source = configuration.get("source")
matches = (
    configuration.get("build_type") == "legacy"
    and isinstance(source, dict)
    and source.get("branch") == expected_branch
    and source.get("path") == "/"
    and configuration.get("https_enforced") is True
)
print("current" if matches else "update")
PY
)"
    if [[ "${pages_configuration_state}" == "update" ]]; then
      if ! gh api --method PUT repos/{owner}/{repo}/pages -f build_type=legacy -f "source[branch]=${branch}" -f 'source[path]=/' -F https_enforced=true >/dev/null; then
        echo "error: failed to update GitHub Pages legacy source to ${branch}:/" >&2
        exit 1
      fi
      pages_configuration_changed="true"
      echo "Updated GitHub Pages legacy source to ${branch}:/."
    fi
  elif python3 - "${pages_configuration_error_path}" <<'PY'
import pathlib
import re
import sys

message = pathlib.Path(sys.argv[1]).read_text(encoding="utf-8", errors="replace")
raise SystemExit(0 if re.search(r"\bHTTP 404\b", message) else 1)
PY
  then
    if ! gh api --method POST repos/{owner}/{repo}/pages -f build_type=legacy -f "source[branch]=${branch}" -f 'source[path]=/' -F https_enforced=true >/dev/null; then
      echo "error: failed to create GitHub Pages legacy source at ${branch}:/" >&2
      exit 1
    fi
    pages_configuration_changed="true"
    echo "Created GitHub Pages legacy source at ${branch}:/."
  else
    cat "${pages_configuration_error_path}" >&2
    echo "error: failed to inspect GitHub Pages configuration" >&2
    exit 1
fi

marker_url="${url%/}/.mprlab-release.json"
attempts="12"
delay_seconds="5"

  read_pages_build() {
    local builds_path="${temporary_directory}/pages-builds.json"
    local builds_error_path="${temporary_directory}/pages-builds-error.txt"
    if ! gh api "repos/{owner}/{repo}/pages/builds?per_page=100" >"${builds_path}" 2>"${builds_error_path}"; then
      cat "${builds_error_path}" >&2
      echo "error: failed to inspect GitHub Pages builds for commit ${pages_commit}" >&2
      return 1
    fi
    python3 - "${builds_path}" "${pages_commit}" <<'PY'
import json
import sys

builds_path, expected_commit = sys.argv[1:]
try:
    builds = json.load(open(builds_path, encoding="utf-8"))
except (OSError, json.JSONDecodeError) as error:
    raise SystemExit(f"GitHub Pages build response is invalid for commit {expected_commit}: {error}")
if not isinstance(builds, list):
    raise SystemExit(f"GitHub Pages build response is not a list for commit {expected_commit}")

matching = [build for build in builds if isinstance(build, dict) and build.get("commit") == expected_commit]
priority = {
    "built": 0,
    "building": 1,
    "queued": 2,
    "errored": 3,
}
matching.sort(key=lambda build: priority.get(build.get("status"), 3))
if not matching:
    print("missing")
    print("-")
    print("-")
    raise SystemExit(0)

build = matching[0]
status = build.get("status")
if not isinstance(status, str) or not status:
    raise SystemExit(f"GitHub Pages build has no status for commit {expected_commit}")
url = build.get("url") if isinstance(build.get("url"), str) and build.get("url") else "-"
error = build.get("error")
message = error.get("message") if isinstance(error, dict) else None
if not isinstance(message, str) or not message:
    message = "-"
print(status)
print(url.replace("\n", " "))
print(message.replace("\n", " "))
PY
  }

  parse_pages_build() {
    pages_build_status="${1%%$'\n'*}"
    local remaining_values="${1#*$'\n'}"
    pages_build_url="${remaining_values%%$'\n'*}"
    pages_build_error="${remaining_values#*$'\n'}"
  }

  pages_build_values="$(read_pages_build)"
  parse_pages_build "${pages_build_values}"
  ignored_terminal_build_url="-"
  if [[ "${pages_branch_changed}" == "true" || "${pages_configuration_changed}" == "true" ]]; then
    if [[ "${pages_branch_changed}" == "false" && "${pages_build_status}" == "errored" ]]; then
      ignored_terminal_build_url="${pages_build_url}"
    fi
  elif [[ "${pages_build_status}" == "missing" || "${pages_build_status}" == "errored" ]]; then
    ignored_terminal_build_url="${pages_build_url}"
    if ! gh api --method POST repos/{owner}/{repo}/pages/builds >/dev/null; then
      echo "error: failed to request GitHub Pages rebuild for commit ${pages_commit}" >&2
      exit 1
    fi
    echo "Requested one GitHub Pages rebuild for commit ${pages_commit}."
  elif [[ "${pages_build_status}" == "queued" || "${pages_build_status}" == "building" ]]; then
    echo "Reusing active GitHub Pages build for commit ${pages_commit}: ${pages_build_url}"
  elif [[ "${pages_build_status}" != "built" ]]; then
    echo "error: GitHub Pages returned unknown build status ${pages_build_status} for commit ${pages_commit}: ${pages_build_url}" >&2
    exit 1
  fi

  marker=""
  for ((attempt = 1; attempt <= attempts; attempt += 1)); do
    if [[ "${pages_build_status}" == "built" ]]; then
      marker="$(curl --fail --silent --show-error "${marker_url}" 2>/dev/null || true)"
      if python3 - "${version}" "${source_commit}" "${marker}" >/dev/null 2>&1 <<'PY'
import json
import sys

data = json.loads(sys.argv[3])
if data.get("schema_version") != 1:
    raise SystemExit(1)
if data.get("release_version") != sys.argv[1]:
    raise SystemExit(1)
if data.get("source_commit") != sys.argv[2]:
    raise SystemExit(1)
PY
      then
        echo "Verified ${url} at source ${source_commit}."
        echo "GitHub Pages build ${pages_build_url} published commit ${pages_commit}."
        exit 0
      fi
    elif [[ "${pages_build_status}" == "errored" && "${pages_build_url}" != "${ignored_terminal_build_url}" ]]; then
      echo "error: GitHub Pages build failed for commit ${pages_commit}: status=${pages_build_status} error=${pages_build_error} url=${pages_build_url}" >&2
      exit 1
    elif [[ ! "${pages_build_status}" =~ ^(missing|queued|building|errored)$ ]]; then
      echo "error: GitHub Pages returned unknown build status ${pages_build_status} for commit ${pages_commit}: ${pages_build_url}" >&2
      exit 1
    fi

    if (( attempt < attempts )); then
      sleep "${delay_seconds}"
      pages_build_values="$(read_pages_build)"
      parse_pages_build "${pages_build_values}"
    fi
  done
  if [[ "${pages_build_status}" == "built" ]]; then
    echo "error: GitHub Pages build ${pages_build_url} reached commit ${pages_commit}, but the public marker did not reach source ${source_commit}: ${marker_url}" >&2
  elif [[ "${pages_build_status}" == "errored" ]]; then
    echo "error: GitHub Pages build failed for commit ${pages_commit}: status=${pages_build_status} error=${pages_build_error} url=${pages_build_url}" >&2
  else
    echo "error: GitHub Pages build for commit ${pages_commit} remained ${pages_build_status} after ${attempts} checks: ${pages_build_url}" >&2
  fi
exit 1
