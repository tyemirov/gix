#!/usr/bin/env python3
"""Plan and apply the reviewed Gix license fleet rollout."""

from __future__ import annotations

import argparse
import concurrent.futures
import dataclasses
import json
import pathlib
import shutil
import subprocess
import sys
import tempfile
from collections.abc import Iterable, Mapping, Sequence


REPOSITORY_ROOT = pathlib.Path(__file__).resolve().parents[2]
DEFAULT_MANIFEST_PATH = REPOSITORY_ROOT / "configs" / "licensing" / "fleet.json"
DEFAULT_WORKFLOW_PATH = REPOSITORY_ROOT / "configs" / "license-rollout.yaml"
DEFAULT_GIX_PATH = REPOSITORY_ROOT / "bin" / "gix"

OWNERS = ("MarcoPoloResearchLab", "tyemirov")
PROFILE_OWNERS = {
    "mprlab-proprietary": "MarcoPoloResearchLab",
    "polyform-noncommercial": "tyemirov",
}
PROFILE_VARIABLES = {
    "mprlab-proprietary": {
        "license_template": "proprietary",
        "license_contact": "legal@mprlab.com",
        "license_year": "2026",
    },
    "polyform-noncommercial": {
        "license_template": "polyform-noncommercial",
        "license_contact": "legal@mprlab.com",
        "license_year": "2026",
    },
}
ALLOWED_LICENSE_HOLDERS = frozenset(
    ("Marco Polo Research Lab LLC", "Vadym Temirov")
)
ALLOWED_DISPOSITIONS = frozenset(("apply", "review"))
ALLOWED_VISIBILITIES = frozenset(("INTERNAL", "PRIVATE", "PUBLIC"))
LICENSE_FILE_NAMES = (
    "COMMERCIAL_LICENSE",
    "COMMERCIAL_LICENSE.md",
    "COMMERCIAL_LICENSE.txt",
    "COPYING",
    "COPYING.md",
    "LICENCE",
    "LICENCE.md",
    "LICENCE.txt",
    "LICENSE",
    "LICENSE.md",
    "LICENSE.txt",
    "MIT-LICENSE",
    "MIT-LICENSE.txt",
    "NOTICE",
    "NOTICE.md",
    "NOTICE.txt",
    "UNLICENSE",
)
LICENSE_FILE_NAME_SET = frozenset(
    candidate.upper() for candidate in LICENSE_FILE_NAMES
)
SPARSE_CHECKOUT_PATHS = tuple(f"/{name}" for name in LICENSE_FILE_NAMES)
AUTOMATION_BRANCH_PREFIX = "automation/license/"
DEFAULT_PARALLELISM = 8
DEFAULT_WORKFLOW_WORKERS = 4
INDIVIDUAL_COMMAND_TIMEOUT_SECONDS = 30
WORKFLOW_TIMEOUT_SECONDS = 350


class RolloutError(RuntimeError):
    """Raised when the rollout cannot prove its safety contract."""


@dataclasses.dataclass(frozen=True)
class RepositoryRecord:
    repository: str
    profile: str
    disposition: str
    default_branch: str
    visibility: str
    license_holder: str
    license_files: Mapping[str, str]
    reason: str


@dataclasses.dataclass(frozen=True)
class Manifest:
    schema_version: int
    snapshot_date: str
    repositories: tuple[RepositoryRecord, ...]

    @property
    def apply_count(self) -> int:
        return sum(record.disposition == "apply" for record in self.repositories)

    @property
    def review_count(self) -> int:
        return sum(record.disposition == "review" for record in self.repositories)


@dataclasses.dataclass(frozen=True)
class LiveRepository:
    repository: str
    default_branch: str
    visibility: str
    license_files: Mapping[str, str]


@dataclasses.dataclass(frozen=True)
class RolloutPlan:
    manifest: Manifest
    live_inventory: Mapping[str, LiveRepository]

    @property
    def apply_records(self) -> tuple[RepositoryRecord, ...]:
        return tuple(
            record
            for record in self.manifest.repositories
            if record.disposition == "apply"
        )

    @property
    def review_records(self) -> tuple[RepositoryRecord, ...]:
        return tuple(
            record
            for record in self.manifest.repositories
            if record.disposition == "review"
        )

    @property
    def apply_count(self) -> int:
        return len(self.apply_records)

    @property
    def review_count(self) -> int:
        return len(self.review_records)


@dataclasses.dataclass(frozen=True)
class PullRequestState:
    record: RepositoryRecord
    existing_url: str


class CommandRunner:
    """Runs subprocesses under the repository timeout contract."""

    def __init__(self) -> None:
        timeout_executable = shutil.which("timeout")
        if timeout_executable is None:
            raise RolloutError("GNU timeout is required")
        self.timeout_executable = timeout_executable

    def run(
        self,
        arguments: Sequence[str | pathlib.Path],
        *,
        cwd: pathlib.Path | None = None,
        timeout_seconds: int = INDIVIDUAL_COMMAND_TIMEOUT_SECONDS,
        check: bool = True,
        stream: bool = False,
    ) -> subprocess.CompletedProcess[str]:
        normalized_arguments = [str(argument) for argument in arguments]
        command = [
            self.timeout_executable,
            "-k",
            f"{timeout_seconds}s",
            "-s",
            "SIGKILL",
            f"{timeout_seconds}s",
            *normalized_arguments,
        ]
        result = subprocess.run(
            command,
            cwd=cwd,
            text=True,
            stdout=None if stream else subprocess.PIPE,
            stderr=None if stream else subprocess.PIPE,
            check=False,
        )
        if check and result.returncode != 0:
            error_text = (result.stderr or result.stdout or "").strip()
            detail = f": {error_text}" if error_text else ""
            raise RolloutError(
                f"command failed ({result.returncode}): "
                f"{' '.join(normalized_arguments)}{detail}"
            )
        return result

    def run_json(
        self,
        arguments: Sequence[str | pathlib.Path],
        *,
        cwd: pathlib.Path | None = None,
    ) -> object:
        result = self.run(arguments, cwd=cwd)
        try:
            return json.loads(result.stdout)
        except json.JSONDecodeError as error:
            raise RolloutError(
                f"command returned invalid JSON: {' '.join(map(str, arguments))}"
            ) from error


class GitHubInspector:
    """Reads the live non-fork, non-archived source fleet."""

    def __init__(
        self,
        runner: CommandRunner,
        *,
        parallelism: int = DEFAULT_PARALLELISM,
    ) -> None:
        self.runner = runner
        self.parallelism = parallelism

    def inspect(self) -> Mapping[str, LiveRepository]:
        metadata_rows: list[dict[str, object]] = []
        for owner in OWNERS:
            raw_rows = self.runner.run_json(
                [
                    "gh",
                    "repo",
                    "list",
                    owner,
                    "--limit",
                    "1000",
                    "--source",
                    "--no-archived",
                    "--json",
                    "nameWithOwner,defaultBranchRef,visibility,isFork,isArchived",
                ]
            )
            if not isinstance(raw_rows, list):
                raise RolloutError(f"GitHub repository list for {owner} was not a list")
            metadata_rows.extend(raw_rows)

        live_repositories: dict[str, LiveRepository] = {}
        with concurrent.futures.ThreadPoolExecutor(
            max_workers=self.parallelism
        ) as executor:
            futures = {
                executor.submit(self._inspect_repository, row): row
                for row in metadata_rows
            }
            for future in concurrent.futures.as_completed(futures):
                repository = future.result()
                if repository.repository in live_repositories:
                    raise RolloutError(
                        f"duplicate live repository: {repository.repository}"
                    )
                live_repositories[repository.repository] = repository
        return dict(sorted(live_repositories.items()))

    def _inspect_repository(self, row: Mapping[str, object]) -> LiveRepository:
        if bool(row.get("isFork")) or bool(row.get("isArchived")):
            raise RolloutError("GitHub source query returned a fork or archived repository")

        repository = require_string(row, "nameWithOwner")
        visibility = require_string(row, "visibility")
        default_branch_value = row.get("defaultBranchRef")
        if default_branch_value is None:
            default_branch = ""
        elif isinstance(default_branch_value, dict):
            default_branch = require_string(default_branch_value, "name")
        else:
            raise RolloutError(
                f"{repository}: defaultBranchRef had an unexpected shape"
            )

        license_files: Mapping[str, str]
        if default_branch == "":
            license_files = {}
        else:
            raw_contents = self.runner.run_json(
                [
                    "gh",
                    "api",
                    "--method",
                    "GET",
                    f"repos/{repository}/contents",
                    "-f",
                    f"ref={default_branch}",
                ]
            )
            if not isinstance(raw_contents, list):
                raise RolloutError(f"{repository}: root contents were not a list")
            discovered_files: dict[str, str] = {}
            for entry in raw_contents:
                if not isinstance(entry, dict) or entry.get("type") == "dir":
                    continue
                name = require_string(entry, "name")
                if not is_license_contract_file(name):
                    continue
                discovered_files[name] = require_string(entry, "sha")
            license_files = dict(sorted(discovered_files.items()))

        return LiveRepository(
            repository=repository,
            default_branch=default_branch,
            visibility=visibility,
            license_files=license_files,
        )


def require_string(values: Mapping[str, object], key: str) -> str:
    value = values.get(key)
    if not isinstance(value, str):
        raise RolloutError(f"{key} must be a string")
    return value.strip()


def is_license_contract_file(name: str) -> bool:
    return name.upper() in LICENSE_FILE_NAME_SET


def load_manifest(path: str | pathlib.Path) -> Manifest:
    manifest_path = pathlib.Path(path)
    try:
        raw_manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    except FileNotFoundError as error:
        raise RolloutError(f"license manifest not found: {manifest_path}") from error
    except json.JSONDecodeError as error:
        raise RolloutError(f"license manifest is invalid JSON: {manifest_path}") from error

    if not isinstance(raw_manifest, dict):
        raise RolloutError("license manifest must be an object")
    schema_version = raw_manifest.get("schema_version")
    if schema_version != 1:
        raise RolloutError("license manifest schema_version must be 1")
    snapshot_date = require_string(raw_manifest, "snapshot_date")
    raw_repositories = raw_manifest.get("repositories")
    if not isinstance(raw_repositories, list) or not raw_repositories:
        raise RolloutError("license manifest repositories must be a non-empty list")

    records: list[RepositoryRecord] = []
    seen_repositories: set[str] = set()
    for raw_record in raw_repositories:
        if not isinstance(raw_record, dict):
            raise RolloutError("license manifest repository entry must be an object")
        record = parse_manifest_record(raw_record)
        if record.repository in seen_repositories:
            raise RolloutError(
                f"duplicate manifest repository: {record.repository}"
            )
        seen_repositories.add(record.repository)
        records.append(record)

    return Manifest(
        schema_version=schema_version,
        snapshot_date=snapshot_date,
        repositories=tuple(sorted(records, key=lambda record: record.repository.lower())),
    )


def parse_manifest_record(raw_record: Mapping[str, object]) -> RepositoryRecord:
    repository = require_string(raw_record, "repository")
    repository_parts = repository.split("/")
    if len(repository_parts) != 2 or not all(repository_parts):
        raise RolloutError(f"invalid repository identity: {repository}")
    owner = repository_parts[0]

    profile = require_string(raw_record, "profile")
    expected_owner = PROFILE_OWNERS.get(profile)
    if expected_owner is None:
        raise RolloutError(f"{repository}: unsupported profile {profile}")
    if owner != expected_owner:
        raise RolloutError(f"profile {profile} requires owner {expected_owner}")

    disposition = require_string(raw_record, "disposition")
    if disposition not in ALLOWED_DISPOSITIONS:
        raise RolloutError(f"{repository}: invalid disposition {disposition}")

    default_branch = require_string(raw_record, "default_branch")
    if disposition == "apply" and default_branch == "":
        raise RolloutError(f"{repository}: apply disposition requires a default branch")

    visibility = require_string(raw_record, "visibility")
    if visibility not in ALLOWED_VISIBILITIES:
        raise RolloutError(f"{repository}: invalid visibility {visibility}")

    license_holder = require_string(raw_record, "license_holder")
    if license_holder not in ALLOWED_LICENSE_HOLDERS:
        raise RolloutError(
            f"{repository}: unsupported license holder {license_holder}"
        )
    if (
        owner == "MarcoPoloResearchLab"
        and license_holder != "Marco Polo Research Lab LLC"
    ):
        raise RolloutError(
            f"{repository}: organization repositories require "
            "Marco Polo Research Lab LLC as license holder"
        )

    raw_license_files = raw_record.get("license_files")
    if not isinstance(raw_license_files, dict):
        raise RolloutError(f"{repository}: license_files must be an object")
    license_files: dict[str, str] = {}
    for raw_name, raw_sha in raw_license_files.items():
        if not isinstance(raw_name, str) or not is_license_contract_file(raw_name):
            raise RolloutError(f"{repository}: unsupported license file {raw_name}")
        if not isinstance(raw_sha, str) or raw_sha.strip() == "":
            raise RolloutError(f"{repository}: license file SHA is required")
        license_files[raw_name] = raw_sha.strip()

    reason = require_string(raw_record, "reason")
    if reason == "":
        raise RolloutError(f"{repository}: disposition reason is required")

    return RepositoryRecord(
        repository=repository,
        profile=profile,
        disposition=disposition,
        default_branch=default_branch,
        visibility=visibility,
        license_holder=license_holder,
        license_files=dict(sorted(license_files.items())),
        reason=reason,
    )


def compare_inventory(
    manifest: Manifest,
    live_inventory: Mapping[str, LiveRepository],
) -> list[str]:
    errors: list[str] = []
    manifest_records = {
        record.repository: record for record in manifest.repositories
    }
    expected_names = set(manifest_records)
    live_names = set(live_inventory)

    for repository in sorted(expected_names - live_names, key=str.lower):
        errors.append(f"reviewed source repository is missing: {repository}")
    for repository in sorted(live_names - expected_names, key=str.lower):
        errors.append(f"unreviewed source repository: {repository}")

    for repository in sorted(expected_names & live_names, key=str.lower):
        expected = manifest_records[repository]
        live = live_inventory[repository]
        if live.default_branch != expected.default_branch:
            errors.append(
                f"{repository}: default branch changed "
                f"(expected {display_value(expected.default_branch)}; "
                f"found {display_value(live.default_branch)})"
            )
        if live.visibility != expected.visibility:
            errors.append(
                f"{repository}: visibility changed "
                f"(expected {expected.visibility}; found {live.visibility})"
            )
        if dict(live.license_files) != dict(expected.license_files):
            errors.append(
                f"{repository}: license files changed "
                f"(expected {describe_license_files(expected.license_files)}; "
                f"found {describe_license_files(live.license_files)})"
            )
    return errors


def display_value(value: str) -> str:
    return value if value else "<none>"


def describe_license_files(license_files: Mapping[str, str]) -> str:
    if not license_files:
        return "none"
    return ", ".join(
        f"{name}@{sha}" for name, sha in sorted(license_files.items())
    )


def build_plan(manifest: Manifest, inspector: object) -> RolloutPlan:
    inspect = getattr(inspector, "inspect", None)
    if not callable(inspect):
        raise RolloutError("live inventory inspector is invalid")
    live_inventory = inspect()
    inventory_errors = compare_inventory(manifest, live_inventory)
    if inventory_errors:
        details = "\n".join(f"- {error}" for error in inventory_errors)
        raise RolloutError(f"live license inventory drifted:\n{details}")
    return RolloutPlan(manifest=manifest, live_inventory=live_inventory)


def print_plan(plan: RolloutPlan) -> None:
    print(
        f"Verified {len(plan.manifest.repositories)} reviewed source repositories: "
        f"{plan.apply_count} ready for draft pull requests, "
        f"{plan.review_count} held for review."
    )
    profile_counts: dict[str, int] = {}
    for record in plan.apply_records:
        profile_counts[record.profile] = profile_counts.get(record.profile, 0) + 1
    for profile, count in sorted(profile_counts.items()):
        print(f"  {profile}: {count}")
    print("Review holds:")
    for record in plan.review_records:
        print(f"  {record.repository}: {record.reason}")
    print("Plan is read-only; no repositories, branches, or pull requests changed.")


def automation_branch(record: RepositoryRecord) -> str:
    return f"{AUTOMATION_BRANCH_PREFIX}{record.profile}"


def inspect_pull_request_state(
    runner: CommandRunner,
    record: RepositoryRecord,
) -> PullRequestState:
    branch = automation_branch(record)
    raw_pull_requests = runner.run_json(
        [
            "gh",
            "pr",
            "list",
            "--repo",
            record.repository,
            "--state",
            "open",
            "--head",
            branch,
            "--json",
            "number,url,headRefName,isDraft",
        ]
    )
    if not isinstance(raw_pull_requests, list):
        raise RolloutError(
            f"{record.repository}: pull-request query did not return a list"
        )
    if len(raw_pull_requests) > 1:
        raise RolloutError(
            f"{record.repository}: multiple open pull requests use {branch}"
        )
    if len(raw_pull_requests) == 1:
        pull_request = raw_pull_requests[0]
        if not isinstance(pull_request, dict):
            raise RolloutError(
                f"{record.repository}: pull-request result had an unexpected shape"
            )
        if pull_request.get("isDraft") is not True:
            raise RolloutError(
                f"{record.repository}: rollout pull request for {branch} is not a draft"
            )
        return PullRequestState(
            record=record,
            existing_url=require_string(pull_request, "url"),
        )

    remote_url = f"git@github.com:{record.repository}.git"
    branch_result = runner.run(
        [
            "git",
            "ls-remote",
            "--exit-code",
            "--heads",
            remote_url,
            f"refs/heads/{branch}",
        ],
        check=False,
    )
    if branch_result.returncode == 0:
        raise RolloutError(
            f"{record.repository}: remote branch {branch} exists without an open "
            "pull request; reconcile it before rerunning"
        )
    if branch_result.returncode != 2:
        error_text = (branch_result.stderr or branch_result.stdout or "").strip()
        raise RolloutError(
            f"{record.repository}: could not inspect remote branch {branch}: "
            f"{error_text}"
        )
    return PullRequestState(record=record, existing_url="")


def inspect_apply_states(
    runner: CommandRunner,
    records: Iterable[RepositoryRecord],
) -> tuple[PullRequestState, ...]:
    states: list[PullRequestState] = []
    with concurrent.futures.ThreadPoolExecutor(
        max_workers=DEFAULT_PARALLELISM
    ) as executor:
        futures = {
            executor.submit(inspect_pull_request_state, runner, record): record
            for record in records
        }
        for future in concurrent.futures.as_completed(futures):
            states.append(future.result())
    return tuple(sorted(states, key=lambda state: state.record.repository.lower()))


def prepare_sparse_clone(
    runner: CommandRunner,
    record: RepositoryRecord,
    temporary_root: pathlib.Path,
) -> pathlib.Path:
    destination = temporary_root / record.repository.replace("/", "--")
    runner.run(
        [
            "git",
            "clone",
            "--filter=blob:none",
            "--depth=1",
            "--sparse",
            "--no-checkout",
            "--branch",
            record.default_branch,
            f"git@github.com:{record.repository}.git",
            destination,
        ]
    )
    runner.run(
        [
            "git",
            "-C",
            destination,
            "sparse-checkout",
            "set",
            "--no-cone",
            *SPARSE_CHECKOUT_PATHS,
        ]
    )
    runner.run(
        ["git", "-C", destination, "checkout", record.default_branch]
    )
    return destination


def prepare_sparse_clones(
    runner: CommandRunner,
    records: Iterable[RepositoryRecord],
    temporary_root: pathlib.Path,
) -> Mapping[str, pathlib.Path]:
    clone_paths: dict[str, pathlib.Path] = {}
    with concurrent.futures.ThreadPoolExecutor(
        max_workers=DEFAULT_PARALLELISM
    ) as executor:
        futures = {
            executor.submit(
                prepare_sparse_clone,
                runner,
                record,
                temporary_root,
            ): record
            for record in records
        }
        for future in concurrent.futures.as_completed(futures):
            record = futures[future]
            clone_paths[record.repository] = future.result()
    return dict(sorted(clone_paths.items()))


def run_gix_workflow(
    runner: CommandRunner,
    *,
    gix_path: pathlib.Path,
    workflow_path: pathlib.Path,
    profile: str,
    license_holder: str,
    records: Iterable[RepositoryRecord],
    clone_paths: Mapping[str, pathlib.Path],
    workflow_workers: int,
) -> None:
    profile_variables = {
        **PROFILE_VARIABLES[profile],
        "license_profile": profile,
    }
    if profile == "mprlab-proprietary":
        profile_variables["license_company"] = license_holder
    elif profile == "polyform-noncommercial":
        profile_variables["license_licensor"] = license_holder
    else:
        raise RolloutError(f"unsupported rollout profile: {profile}")
    arguments: list[str | pathlib.Path] = [
        gix_path,
        "workflow",
        workflow_path,
        "--yes",
        "--workflow-workers",
        str(workflow_workers),
    ]
    for record in records:
        arguments.extend(("--roots", clone_paths[record.repository]))
    for key, value in sorted(profile_variables.items()):
        arguments.extend(("--var", f"{key}={value}"))
    runner.run(
        arguments,
        timeout_seconds=WORKFLOW_TIMEOUT_SECONDS,
        stream=True,
    )


def verify_created_pull_requests(
    runner: CommandRunner,
    records: Iterable[RepositoryRecord],
) -> tuple[str, ...]:
    states = inspect_apply_states(runner, records)
    missing = [
        state.record.repository for state in states if state.existing_url == ""
    ]
    if missing:
        raise RolloutError(
            "workflow completed without an open draft pull request for: "
            + ", ".join(missing)
        )
    return tuple(state.existing_url for state in states)


def apply_plan(
    plan: RolloutPlan,
    *,
    runner: CommandRunner,
    gix_path: pathlib.Path,
    workflow_path: pathlib.Path,
    workflow_workers: int,
) -> None:
    if not gix_path.is_file():
        raise RolloutError(f"Gix binary not found: {gix_path}")
    if not workflow_path.is_file():
        raise RolloutError(f"license rollout workflow not found: {workflow_path}")
    if workflow_workers < 1:
        raise RolloutError("workflow workers must be positive")

    states = inspect_apply_states(runner, plan.apply_records)
    existing_states = tuple(state for state in states if state.existing_url)
    pending_records = tuple(
        state.record for state in states if state.existing_url == ""
    )
    for state in existing_states:
        print(
            f"Already prepared: {state.record.repository} "
            f"({state.existing_url})"
        )
    if not pending_records:
        print("All reviewed license pull requests are already open.")
        return

    temporary_root = pathlib.Path(
        tempfile.mkdtemp(prefix="gix-license-rollout-")
    ).resolve()
    try:
        clone_paths = prepare_sparse_clones(runner, pending_records, temporary_root)
        rollout_groups = sorted(
            {
                (record.profile, record.license_holder)
                for record in pending_records
            }
        )
        for profile, license_holder in rollout_groups:
            profile_records = tuple(
                record
                for record in pending_records
                if record.profile == profile
                and record.license_holder == license_holder
            )
            run_gix_workflow(
                runner,
                gix_path=gix_path,
                workflow_path=workflow_path,
                profile=profile,
                license_holder=license_holder,
                records=profile_records,
                clone_paths=clone_paths,
                workflow_workers=workflow_workers,
            )
        created_urls = verify_created_pull_requests(runner, pending_records)
    except Exception:
        print(
            f"Rollout workspace preserved for inspection: {temporary_root}",
            file=sys.stderr,
        )
        raise
    else:
        if (
            temporary_root.parent == pathlib.Path(tempfile.gettempdir()).resolve()
            and temporary_root.name.startswith("gix-license-rollout-")
        ):
            shutil.rmtree(temporary_root)
        else:
            raise RolloutError(
                f"refusing to remove unexpected temporary path: {temporary_root}"
            )

    print(
        f"Prepared {len(created_urls)} new draft pull requests; "
        f"{len(existing_states)} were already open."
    )


def build_argument_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description=(
            "Verify the frozen license inventory or create its reviewed draft "
            "pull requests."
        )
    )
    parser.add_argument("action", choices=("plan", "apply"))
    parser.add_argument(
        "--manifest",
        type=pathlib.Path,
        default=DEFAULT_MANIFEST_PATH,
    )
    parser.add_argument(
        "--workflow",
        type=pathlib.Path,
        default=DEFAULT_WORKFLOW_PATH,
    )
    parser.add_argument(
        "--gix",
        type=pathlib.Path,
        default=DEFAULT_GIX_PATH,
    )
    parser.add_argument(
        "--workflow-workers",
        type=int,
        default=DEFAULT_WORKFLOW_WORKERS,
    )
    return parser


def main(arguments: Sequence[str] | None = None) -> int:
    parser = build_argument_parser()
    options = parser.parse_args(arguments)
    try:
        manifest = load_manifest(options.manifest)
        runner = CommandRunner()
        inspector = GitHubInspector(runner)
        plan = build_plan(manifest, inspector)
        print_plan(plan)
        if options.action == "apply":
            apply_plan(
                plan,
                runner=runner,
                gix_path=options.gix.resolve(),
                workflow_path=options.workflow.resolve(),
                workflow_workers=options.workflow_workers,
            )
    except RolloutError as error:
        print(f"error: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
