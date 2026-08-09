#!/usr/bin/env bash
set -euo pipefail

[[ $# -eq 0 ]] || { echo "error: make release accepts no arguments" >&2; exit 1; }

helper="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/release_helper.py"
ci_timeout="350"
artifact_targets="release-artifacts pages-artifact"

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

select_release() {
  python3 -c '
import json
import re
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    data = json.load(handle)
semver_bump = sys.argv[2]
info = data.get("version_info") or {}
detected_scheme = info.get("scheme_guess") or "none"
if detected_scheme in ("none", "semver", "mixed"):
    effective_scheme = "semver"
elif detected_scheme == "calver":
    effective_scheme = "calver"
else:
    raise SystemExit(f"release version scheme is unsupported: {detected_scheme}")

def select_semver(latest):
    if not latest:
        return "v1.0.0"
    if semver_bump not in ("patch", "minor", "major"):
        raise SystemExit("autonomous SemVer decision must be patch, minor, or major")
    match = re.match(r"^(v?)(\d+)\.(\d+)\.(\d+)(?:[-+].*)?$", latest)
    if not match:
        raise SystemExit(f"latest SemVer tag is invalid: {latest}")
    prefix, major, minor, patch = match.groups()
    major, minor, patch = int(major), int(minor), int(patch)
    if semver_bump == "major":
        major, minor, patch = major + 1, 0, 0
    elif semver_bump == "minor":
        minor, patch = minor + 1, 0
    else:
        patch += 1
    selected_prefix = prefix or "v"
    return f"{selected_prefix}{major}.{minor}.{patch}"

if effective_scheme == "semver":
    selected = select_semver(info.get("latest_semver_tag") or "")
elif effective_scheme == "calver":
    candidate = info.get("calver_candidate") or {}
    if candidate.get("ok") is not True:
        raise SystemExit("CalVer candidate is not valid for this release timestamp")
    selected = info.get("next_calver") or ""
if effective_scheme == "calver":
    boundary = info.get("latest_calver_tag") or ""
elif effective_scheme == "semver":
    boundary = info.get("latest_semver_tag") or ""

if not selected:
    raise SystemExit("release version selection returned an empty version")
print(selected)
print(boundary)
print(effective_scheme)
' "$1" "$2"
}

decide_semver_bump() {
  local boundary="$1"
  local decision_output decision_values
  if ! decision_output="$(go run . message semver "${boundary}")"; then
    echo "error: autonomous SemVer decision failed" >&2
    exit 1
  fi
  decision_values="$(python3 -c '
import json
import sys

matches = []
for line in sys.stdin.read().splitlines():
    try:
        value = json.loads(line)
    except json.JSONDecodeError:
        continue
    if isinstance(value, dict) and set(value) == {"bump", "reason", "deterministic_floor"}:
        matches.append(value)
if len(matches) != 1:
    raise SystemExit("release decision command did not return exactly one decision")
decision = matches[0]
if decision["bump"] not in ("patch", "minor", "major"):
    raise SystemExit("release decision command returned an invalid bump")
if not isinstance(decision["reason"], str) or not decision["reason"].strip():
    raise SystemExit("release decision command returned an empty reason")
print(decision["bump"])
print(decision["reason"].strip())
' <<<"${decision_output}")" || {
    echo "error: autonomous SemVer decision output is invalid" >&2
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
  if ! "${helper}" preflight --local --release-timestamp "${release_timestamp}" >"${preflight_json}"; then
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
detected_scheme="$(json_value "${preflight_json}" "version_info.scheme_guess")"
semver_bump=""
semver_reason=""
case "${detected_scheme}" in
  none|semver|mixed)
    boundary_tag="$(json_value "${preflight_json}" "version_info.latest_semver_tag")"
    if [[ -n "${boundary_tag}" ]]; then
      echo "==> [release] Deciding the SemVer successor"
      decision_values="$(decide_semver_bump "${boundary_tag}")"
      semver_bump="$(sed -n '1p' <<<"${decision_values}")"
      semver_reason="$(sed -n '2p' <<<"${decision_values}")"
      echo "semver_decision=${semver_bump}"
      echo "semver_reason=${semver_reason}"
    fi
    ;;
  calver) ;;
  *) echo "error: release version scheme is unsupported: ${detected_scheme}" >&2; exit 1 ;;
esac
selection="$(select_release "${preflight_json}" "${semver_bump}")"
next_version="$(sed -n '1p' <<<"${selection}")"
boundary_tag="$(sed -n '2p' <<<"${selection}")"
effective_scheme="$(sed -n '3p' <<<"${selection}")"
echo "version_scheme=${effective_scheme}"
echo "next_version=${next_version}"
echo "changelog_boundary=${boundary_tag:-<none>}"

artifact_dir="$(git rev-parse --git-path mprlab-release)"
if [[ "${artifact_dir}" != /* ]]; then
  artifact_dir="${repo_root}/${artifact_dir}"
fi
candidate_artifact_dir="$(mktemp -d "$(dirname "${artifact_dir}")/mprlab-release-candidate.XXXXXX")"

"${helper}" initialize-release-artifact \
  --version "${next_version}" \
  --source-commit "${source_commit}" \
  --release-timestamp "${release_timestamp}" \
  --artifact-dir "${candidate_artifact_dir}"

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
rechecked_selection="$(select_release "${preflight_json}" "${semver_bump}")"
[[ "${rechecked_selection}" == "${selection}" ]] || { echo "error: autonomous release selection changed while preparing artifacts" >&2; exit 1; }

echo "==> [release] Preparing ${next_version} from local Git history"
notes_args=(generate-notes --version "${next_version}" --release-date "${release_date}")
if [[ -n "${boundary_tag}" ]]; then
  notes_args+=(--since-tag "${boundary_tag}")
fi
"${helper}" "${notes_args[@]}" | tee "${notes_file}"
"${helper}" insert-changelog --notes-file "${notes_file}"

git add CHANGELOG.md
if git diff --cached --quiet -- CHANGELOG.md; then
  echo "error: CHANGELOG.md has no staged release changes" >&2
  exit 1
fi
staged_files="$(git diff --cached --name-only)"
if [[ "${staged_files}" != "CHANGELOG.md" ]]; then
  echo "error: release commit may contain only CHANGELOG.md" >&2
  printf '%s\n' "${staged_files}" >&2
  exit 1
fi

git commit -m "Release ${next_version}"
release_commit="$(git rev-parse HEAD)"
release_tag="${next_version}"
git tag -a "${next_version}" -m "Release ${next_version}" "${release_commit}"
release_tag_created="true"
"${helper}" write-release-artifact \
  --version "${next_version}" \
  --source-commit "${source_commit}" \
  --release-commit "${release_commit}" \
  --notes-file "${notes_file}" \
  --default-branch "${default_branch}" \
  --release-timestamp "${release_timestamp}" \
  --artifact-dir "${candidate_artifact_dir}"
"${helper}" verify-release-artifact --artifact-dir "${candidate_artifact_dir}"
"${helper}" promote-release-artifact --artifact-dir "${candidate_artifact_dir}"
release_promoted="true"
candidate_artifact_dir=""

echo "Prepared ${next_version} at ${release_commit}. Run make publish to publish it."
