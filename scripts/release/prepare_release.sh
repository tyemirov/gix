#!/usr/bin/env bash
set -euo pipefail

[[ $# -eq 0 ]] || { echo "error: make release accepts no arguments" >&2; exit 1; }

helper="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/release_helper.py"
ci_timeout="350"
artifact_targets="release-artifacts pages-artifact"
release_policy=(semver --fixed-major 1)

command -v git >/dev/null 2>&1 || { echo "error: git is required" >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || { echo "error: python3 is required" >&2; exit 1; }

repo_root="$(git rev-parse --show-toplevel)"
cd "${repo_root}"

[[ -x "${helper}" ]] || { echo "error: release helper is not executable: ${helper}" >&2; exit 1; }

json_value() {
  python3 - "$1" "$2" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    value = json.load(handle)
for part in sys.argv[2].split("."):
    value = value.get(part) if isinstance(value, dict) else None
print("" if value is None else value)
PY
}

decide_release_version() {
  local source_commit="$1"
  local decision_output decision_values
  if ! decision_output="$(go run . release next "${release_policy[@]}" --format json)"; then
    echo "error: autonomous release version decision failed" >&2
    exit 1
  fi
  decision_values="$(printf '%s\n' "${decision_output}" | python3 -c '
import json
import sys

matches = []
for line in sys.stdin.read().splitlines():
    try:
        value = json.loads(line)
    except json.JSONDecodeError:
        continue
    if isinstance(value, dict) and value.get("contract") == "mprlab.version-decision/v2":
        matches.append(value)
if len(matches) != 1:
    raise SystemExit("release decision command did not return exactly one decision")
decision = matches[0]
policy = decision.get("policy")
if policy != {"scheme": "semver", "fixed_major": 1}:
    raise SystemExit("release decision command returned a different release policy")
if decision.get("source_commit") != sys.argv[1]:
    raise SystemExit("release decision command returned a different source commit")
if not isinstance(decision.get("next_version"), str) or not decision["next_version"]:
    raise SystemExit("release decision command returned an empty next version")
if not isinstance(decision["reason"], str) or not decision["reason"].strip():
    raise SystemExit("release decision command returned an empty reason")
print(decision["next_version"])
print(decision.get("boundary_tag") or "")
print(policy["scheme"])
print(decision["reason"].strip())
' "${source_commit}")" || {
    echo "error: autonomous release version decision output is invalid" >&2
    exit 1
  }
  printf '%s\n' "${decision_values}"
}

preflight_json="$(mktemp)"
notes_file="$(mktemp)"
candidate_artifact_dir=""
source_commit=""
default_branch=""
release_commit=""
release_tag=""
release_tag_created="false"
release_promoted="false"

rollback_release_commit() {
  local current_branch current_head tag_commit tag_object
  current_branch="$(git branch --show-current)"
  current_head="$(git rev-parse HEAD)"
  if [[ "${current_branch}" != "${default_branch}" || "${current_head}" != "${release_commit}" ]]; then
    echo "error: release rollback ownership changed; expected ${default_branch} at ${release_commit}, found ${current_branch:-<detached>} at ${current_head}" >&2
    return 1
  fi

  if [[ "${release_tag_created}" == "true" ]]; then
    tag_commit="$(git rev-parse --verify "refs/tags/${release_tag}^{commit}")"
    tag_object="$(git rev-parse --verify "refs/tags/${release_tag}")"
    if [[ "${tag_commit}" != "${release_commit}" ]]; then
      echo "error: release rollback does not own tag ${release_tag}" >&2
      return 1
    fi
    printf 'start\nupdate refs/heads/%s %s %s\ndelete refs/tags/%s %s\nprepare\ncommit\n' \
      "${default_branch}" "${source_commit}" "${release_commit}" "${release_tag}" "${tag_object}" |
      git update-ref --stdin
  else
    printf 'start\nupdate refs/heads/%s %s %s\nprepare\ncommit\n' \
      "${default_branch}" "${source_commit}" "${release_commit}" |
      git update-ref --stdin
  fi
  git restore --source "${source_commit}" --staged --worktree -- CHANGELOG.md
  echo "Restored ${default_branch} to ${source_commit} after release preparation failed." >&2
}

cleanup() {
  local exit_status="$?"
  if [[ "${exit_status}" -ne 0 && -n "${release_commit}" && "${release_promoted}" != "true" ]]; then
    rollback_release_commit || exit_status=1
  fi
  rm -f "${preflight_json}" "${notes_file}"
  if [[ -n "${candidate_artifact_dir}" && -d "${candidate_artifact_dir}" ]]; then
    rm -rf "${candidate_artifact_dir}"
  fi
  return "${exit_status}"
}
trap cleanup EXIT

release_timestamp="$(date +%Y-%m-%dT%H:%M:%S%z)"
release_date="${release_timestamp%%T*}"

run_local_preflight() {
  if ! "${helper}" preflight --local --scheme semver --fixed-major 1 >"${preflight_json}"; then
    cat "${preflight_json}"
    echo "error: local release preflight failed" >&2
    exit 1
  fi
  cat "${preflight_json}"
}

echo "==> [release] Checking local release state"
run_local_preflight
default_branch="$(json_value "${preflight_json}" "default_branch")"
source_commit="$(git rev-parse HEAD)"
exact_release_version="$(json_value "${preflight_json}" "version_info.exact_head_version_tag")"
if [[ -n "${exact_release_version}" ]]; then
  "${helper}" reuse-exact-release --version "${exact_release_version}" --default-branch "${default_branch}"
  exit 0
fi

echo "==> [release] Running make ci"
(
  unset MAKEFLAGS MAKELEVEL MAKEOVERRIDES MFLAGS
  timeout -k "${ci_timeout}s" -s SIGKILL "${ci_timeout}s" make ci
)

echo "==> [release] Rechecking local state after CI"
run_local_preflight
[[ "$(git rev-parse HEAD)" == "${source_commit}" ]] || { echo "error: HEAD changed while make ci was running" >&2; exit 1; }
echo "==> [release] Deciding the release version"
decision_values="$(decide_release_version "${source_commit}")"
next_version="$(sed -n '1p' <<<"${decision_values}")"
boundary_tag="$(sed -n '2p' <<<"${decision_values}")"
effective_scheme="$(sed -n '3p' <<<"${decision_values}")"
release_reason="$(sed -n '4p' <<<"${decision_values}")"
echo "version_scheme=${effective_scheme}"
echo "next_version=${next_version}"
echo "changelog_boundary=${boundary_tag:-<none>}"
echo "version_reason=${release_reason}"

artifact_dir="$(git rev-parse --git-path mprlab-release)"
if [[ "${artifact_dir}" != /* ]]; then
  artifact_dir="${repo_root}/${artifact_dir}"
fi
candidate_artifact_dir="$(mktemp -d "$(dirname "${artifact_dir}")/mprlab-release-candidate.XXXXXX")"

initialize_artifact_args=(
  initialize-release-artifact
  --version "${next_version}"
  --source-commit "${source_commit}"
  --release-timestamp "${release_timestamp}"
  --artifact-dir "${candidate_artifact_dir}"
)
"${helper}" "${initialize_artifact_args[@]}"

read -r -a artifact_target_list <<<"${artifact_targets}"
echo "==> [release] Preparing local artifacts: ${artifact_targets}"
RELEASE_VERSION="${next_version}" \
RELEASE_TIMESTAMP="${release_timestamp}" \
MOBILE_RELEASE_TIMESTAMP="${release_timestamp}" \
RELEASE_ARTIFACT_DIR="${candidate_artifact_dir}" \
make --no-print-directory "${artifact_target_list[@]}"
echo "==> [release] Rechecking local state after artifact preparation"
run_local_preflight
[[ "$(git rev-parse HEAD)" == "${source_commit}" ]] || { echo "error: HEAD changed while preparing release artifacts" >&2; exit 1; }

echo "==> [release] Preparing ${next_version} from local Git history"
notes_args=(generate-notes --version "${next_version}" --release-date "${release_date}")
if [[ -n "${boundary_tag}" ]]; then
  notes_args+=(--since-tag "${boundary_tag}")
fi
"${helper}" "${notes_args[@]}" | tee "${notes_file}"
"${helper}" insert-changelog --notes-file "${notes_file}"

release_metadata_files=(CHANGELOG.md)
git add -- "${release_metadata_files[@]}"
if git diff --cached --quiet -- "${release_metadata_files[@]}"; then
  echo "error: release metadata has no staged changes" >&2
  exit 1
fi
staged_files="$(git diff --cached --name-only | LC_ALL=C sort)"
expected_staged_files="$(printf '%s\n' "${release_metadata_files[@]}" | LC_ALL=C sort)"
if [[ "${staged_files}" != "${expected_staged_files}" ]]; then
  echo "error: release commit must contain the exact release metadata files" >&2
  printf '%s\n' "${staged_files}" >&2
  exit 1
fi

git commit -m "Release ${next_version}"
release_commit="$(git rev-parse HEAD)"
release_tag="${next_version}"
git tag -a "${next_version}" -m "Release ${next_version}" "${release_commit}"
release_tag_created="true"
write_artifact_args=(
  write-release-artifact
  --version "${next_version}"
  --source-commit "${source_commit}"
  --release-commit "${release_commit}"
  --notes-file "${notes_file}"
  --default-branch "${default_branch}"
  --release-timestamp "${release_timestamp}"
  --artifact-dir "${candidate_artifact_dir}"
)
"${helper}" "${write_artifact_args[@]}"
"${helper}" verify-release-artifact --artifact-dir "${candidate_artifact_dir}"
"${helper}" promote-release-artifact --artifact-dir "${candidate_artifact_dir}"
release_promoted="true"
candidate_artifact_dir=""

echo "Prepared ${next_version} at ${release_commit}. Run make publish to publish it."
