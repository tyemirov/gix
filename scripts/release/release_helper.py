#!/usr/bin/env python3
"""Deterministic helper for the repository-owned release workflow."""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import re
import shutil
import subprocess
import tempfile
import urllib.error
import urllib.request
from pathlib import Path, PurePosixPath
from typing import Any


SEMVER_TAG_RE = re.compile(r"^v?(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)(?:[-+][0-9A-Za-z.-]+)?$")
CALVER_TAG_RE = re.compile(
    r"^v?(?P<year>[1-9]\d)\.(?P<month_day>(?:0|[1-9]\d*))\.(?P<hhmmss>(?:0|[1-9]\d*))$"
)
# Older releases used YYYY.M.D.minutes; keep recognizing them for ordering.
LEGACY_CALVER_MINUTE_TAG_RE = re.compile(
    r"^v?(?P<year>\d{4})\.(?P<month>\d{1,2})\.(?P<day>\d{1,2})\.(?P<minutes>\d{1,4})$"
)
# Older releases also used YYYY.M.D.H[.m[.s]]; keep recognizing them for ordering.
LEGACY_CALVER_TAG_RE = re.compile(
    r"^v?(?P<year>\d{4})\.(?P<month>\d{1,2})\.(?P<day>\d{1,2})"
    r"(?:\.(?P<hour>\d{1,2})(?:\.(?P<minute>\d{1,2})(?:\.(?P<second>\d{1,2}))?)?)?$"
)
RELEASE_HEADING_RE = re.compile(
    r"^##\s+\[?(?:v?(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)(?:[-+][0-9A-Za-z.-]+)?|v?\d{4}\.\d{1,2}\.\d{1,2}(?:\.\d{1,4}(?:\.\d{1,2}(?:\.\d{1,2})?)?)?)\]?(?:[^\n]*)?$",
    re.MULTILINE,
)
RELEASE_REMOTE = "origin"
RELEASE_MANIFEST_SCHEMA = 3
GO_INSTALL_TRANSPORT_RE = re.compile(r"^v1\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)$")


class HelperError(Exception):
    def __init__(self, message: str, details: dict[str, Any] | None = None) -> None:
        super().__init__(message)
        self.details = details or {}


def run(command: list[str], cwd: Path | None = None, check: bool = True) -> subprocess.CompletedProcess[str]:
    proc = subprocess.run(
        command,
        cwd=str(cwd) if cwd else None,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if check and proc.returncode != 0:
        raise HelperError(
            f"command failed: {' '.join(command)}",
            {
                "command": command,
                "returncode": proc.returncode,
                "stdout": proc.stdout.strip(),
                "stderr": proc.stderr.strip(),
            },
        )
    return proc


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def emit(payload: dict[str, Any]) -> None:
    print(json.dumps(payload, indent=2, sort_keys=True))


def fail(message: str, details: dict[str, Any] | None = None) -> None:
    emit({"ok": False, "error": message, "details": details or {}})
    raise SystemExit(1)


def require_tools(names: list[str]) -> list[str]:
    return [name for name in names if shutil.which(name) is None]


def repo_root() -> Path:
    return Path(run(["git", "rev-parse", "--show-toplevel"]).stdout.strip())


def gh_json(command: list[str], cwd: Path) -> Any:
    return json.loads(run(command, cwd=cwd).stdout or "null")


def resolve_default_branch(cwd: Path, override: str | None = None) -> str:
    if override:
        return override

    gh_proc = run(["gh", "repo", "view", "--json", "defaultBranchRef"], cwd=cwd, check=False)
    if gh_proc.returncode == 0:
        data = json.loads(gh_proc.stdout)
        name = (data.get("defaultBranchRef") or {}).get("name")
        if name:
            return name

    remote_proc = run(["git", "remote", "show", "origin"], cwd=cwd)
    for line in remote_proc.stdout.splitlines():
        if "HEAD branch:" in line:
            return line.rsplit(":", 1)[1].strip()

    raise HelperError("could not resolve default branch")


def resolve_default_branch_local(cwd: Path, override: str | None = None) -> str:
    if override:
        return override

    remote_head = run(
        ["git", "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"],
        cwd=cwd,
        check=False,
    )
    if remote_head.returncode != 0:
        raise HelperError(
            "could not resolve the default branch from local refs",
            {"required_ref": "refs/remotes/origin/HEAD"},
        )
    ref_name = remote_head.stdout.strip()
    if not ref_name.startswith("origin/") or ref_name == "origin/":
        raise HelperError("local origin/HEAD ref is invalid", {"ref": ref_name})
    return ref_name.removeprefix("origin/")


def all_tags(cwd: Path) -> list[str]:
    return run(["git", "tag", "--sort=-version:refname"], cwd=cwd).stdout.splitlines()


def calver_match(tag: str) -> re.Match[str] | None:
    match = CALVER_TAG_RE.match(tag)
    if not match:
        return None
    month_day = int(match.group("month_day"))
    month = month_day // 100
    day = month_day % 100
    time_value = int(match.group("hhmmss"))
    if time_value > 235959:
        return None
    hhmmss = f"{time_value:06d}"
    hour = int(hhmmss[:2])
    minute = int(hhmmss[2:4])
    second = int(hhmmss[4:6])
    try:
        dt.date(2000 + int(match.group("year")), month, day)
    except ValueError:
        return None
    if not (0 <= hour <= 23 and 0 <= minute <= 59 and 0 <= second <= 59):
        return None
    return match


def legacy_calver_minute_match(tag: str) -> re.Match[str] | None:
    match = LEGACY_CALVER_MINUTE_TAG_RE.match(tag)
    if not match:
        return None
    try:
        dt.date(int(match.group("year")), int(match.group("month")), int(match.group("day")))
    except ValueError:
        return None
    if not 0 <= int(match.group("minutes")) <= 1439:
        return None
    return match


def legacy_calver_match(tag: str) -> re.Match[str] | None:
    match = LEGACY_CALVER_TAG_RE.match(tag)
    if not match:
        return None
    try:
        dt.date(int(match.group("year")), int(match.group("month")), int(match.group("day")))
    except ValueError:
        return None
    for name, upper in (("hour", 23), ("minute", 59), ("second", 59)):
        value = match.group(name)
        if value is not None and not 0 <= int(value) <= upper:
            return None
    return match


def tag_scheme(tag: str) -> str | None:
    if calver_match(tag) or legacy_calver_minute_match(tag) or legacy_calver_match(tag):
        return "calver"
    if SEMVER_TAG_RE.match(tag):
        return "semver"
    return None


def parse_release_timestamp(value: str | None, release_date: str | None = None) -> dt.datetime:
    if not value:
        if release_date:
            return dt.datetime.combine(parse_release_date(release_date), dt.time())
        return dt.datetime.now().astimezone()
    try:
        normalized = value.replace("Z", "+00:00")
        return dt.datetime.fromisoformat(normalized)
    except ValueError as exc:
        try:
            return dt.datetime.combine(dt.date.fromisoformat(value), dt.time())
        except ValueError:
            raise HelperError(
                "release timestamp must use ISO format such as YYYY-MM-DDTHH:MM:SS or YYYY-MM-DD",
                {"release_timestamp": value},
            ) from exc


def parse_release_date(value: str) -> dt.date:
    try:
        return dt.date.fromisoformat(value)
    except ValueError as exc:
        raise HelperError("release date must use YYYY-MM-DD format", {"release_date": value}) from exc


def version_info(cwd: Path) -> dict[str, Any]:
    tags = all_tags(cwd)
    exact_head_version_tags = [
        tag
        for tag in run(["git", "tag", "--points-at", "HEAD", "--sort=-version:refname"], cwd=cwd).stdout.splitlines()
        if tag_scheme(tag)
    ]
    exact_head_go_install_tag = None
    if len(exact_head_version_tags) > 1:
        release_configuration = cwd / ".mprlab" / "release.yml"
        release_configuration_text = (
            release_configuration.read_text(encoding="utf-8") if release_configuration.is_file() else ""
        )
        transport_tags = [tag for tag in exact_head_version_tags if GO_INSTALL_TRANSPORT_RE.match(tag)]
        product_tags = [tag for tag in exact_head_version_tags if tag not in transport_tags]
        if (
            not re.search(r"(?m)^go_install:\s*$", release_configuration_text)
            or len(transport_tags) != 1
            or len(product_tags) != 1
        ):
            raise HelperError(
                "HEAD has multiple release version tags",
                {"exact_head_version_tags": exact_head_version_tags},
            )
        exact_head_go_install_tag = transport_tags[0]
        exact_head_version_tag = product_tags[0]
    else:
        exact_head_version_tag = exact_head_version_tags[0] if exact_head_version_tags else None
    version_tags = [tag for tag in tags if tag_scheme(tag)]

    return {
        "exact_head_version_tag": exact_head_version_tag,
        "exact_head_version_scheme": tag_scheme(exact_head_version_tag) if exact_head_version_tag else None,
        "exact_head_go_install_tag": exact_head_go_install_tag,
        "version_tags": version_tags[:20],
    }


def detect_validation_candidates(cwd: Path) -> list[str]:
    candidates: list[str] = []

    makefile = cwd / "Makefile"
    if makefile.exists() and re.search(r"^ci\s*:", makefile.read_text(encoding="utf-8", errors="replace"), re.MULTILINE):
        candidates.append("make ci")

    package_json = cwd / "package.json"
    if package_json.exists():
        try:
            scripts = json.loads(package_json.read_text(encoding="utf-8")).get("scripts", {})
        except json.JSONDecodeError:
            scripts = {}
        runner = "npm"
        if (cwd / "pnpm-lock.yaml").exists():
            runner = "pnpm"
        elif (cwd / "yarn.lock").exists():
            runner = "yarn"
        for script_name in ("ci", "test"):
            if script_name in scripts:
                candidates.append(f"{runner} run {script_name}")

    if (cwd / "pyproject.toml").exists() or (cwd / "pytest.ini").exists():
        if not candidates:
            candidates.append("pytest")

    return candidates


def command_preflight(args: argparse.Namespace) -> int:
    missing = require_tools(["git"] if args.local else ["git", "gh", "gix"])
    if missing:
        fail("required tools are missing", {"missing_tools": missing})

    cwd = repo_root()
    default_branch = (
        resolve_default_branch_local(cwd, args.default_branch)
        if args.local
        else resolve_default_branch(cwd, args.default_branch)
    )
    versions = version_info(cwd)
    status_lines = run(["git", "status", "--short"], cwd=cwd).stdout.splitlines()
    current_branch = run(["git", "branch", "--show-current"], cwd=cwd).stdout.strip()
    open_prs = []
    if not args.local:
        open_prs = gh_json(
            ["gh", "pr", "list", "--base", default_branch, "--state", "open", "--json", "number,title,headRefName,url"],
            cwd,
        )
    payload = {
        "ok": not status_lines and not open_prs and current_branch == default_branch,
        "scope": "local" if args.local else "remote",
        "repo_root": str(cwd),
        "default_branch": default_branch,
        "current_branch": current_branch,
        "dirty_status": status_lines,
        "open_prs": open_prs,
        "latest_tag": versions["version_tags"][0] if versions["version_tags"] else None,
        "version_info": versions,
        "validation_candidates": detect_validation_candidates(cwd),
    }
    emit(payload)
    return 0 if payload["ok"] else 1


def command_generate_notes(args: argparse.Namespace) -> int:
    cwd = repo_root()
    release_date = parse_release_date(args.release_date).isoformat()
    revision = "HEAD"
    if args.since_tag:
        boundary = run(["git", "rev-parse", "--verify", f"{args.since_tag}^{{commit}}"], cwd=cwd, check=False)
        if boundary.returncode != 0:
            fail("changelog boundary tag does not resolve locally", {"since_tag": args.since_tag})
        revision = f"{args.since_tag}..HEAD"

    log_result = run(["git", "log", "--format=%s", revision], cwd=cwd)
    subjects = [line.strip() for line in log_result.stdout.splitlines() if line.strip()]
    if not subjects:
        fail("no local commits are available for release notes", {"revision": revision})

    print(f"## [{args.version}] - {release_date}")
    print()
    for subject in subjects:
        print(f"- {subject}")
    return 0


def release_artifact_dir(cwd: Path, override: str | None = None) -> Path:
    if override:
        return Path(override).expanduser().resolve()
    raw_path = run(["git", "rev-parse", "--git-path", "mprlab-release"], cwd=cwd).stdout.strip()
    artifact_path = Path(raw_path)
    if not artifact_path.is_absolute():
        artifact_path = cwd / artifact_path
    return artifact_path.resolve()


def resolve_commit(cwd: Path, revision: str, label: str) -> str:
    result = run(["git", "rev-parse", "--verify", f"{revision}^{{commit}}"], cwd=cwd, check=False)
    if result.returncode != 0:
        raise HelperError(f"{label} does not resolve to a commit", {label: revision})
    return result.stdout.strip()


def repository_file(cwd: Path, relative_path: str, label: str) -> Path:
    canonical = PurePosixPath(relative_path)
    if canonical.is_absolute() or canonical.as_posix() != relative_path or ".." in canonical.parts:
        raise HelperError(f"{label} must be a canonical repository-relative path", {label: relative_path})
    resolved = (cwd / relative_path).resolve()
    if cwd not in resolved.parents:
        raise HelperError(f"{label} resolves outside the repository", {label: relative_path})
    return resolved


def read_module_path(cwd: Path) -> str:
    go_mod = cwd / "go.mod"
    if not go_mod.is_file():
        raise HelperError("Go install module file is missing", {"path": str(go_mod)})
    module_lines = [
        line.split()
        for line in go_mod.read_text(encoding="utf-8").splitlines()
        if line.strip().startswith("module ")
    ]
    if len(module_lines) != 1 or len(module_lines[0]) != 2:
        raise HelperError("Go install module path is invalid", {"path": str(go_mod)})
    return module_lines[0][1]


def validate_go_install_contract(
    cwd: Path,
    value: Any,
    product_version: str,
    release_commit: str,
    verify_remote: bool = False,
) -> dict[str, str] | None:
    if value is None:
        return None
    if not isinstance(value, dict) or set(value) != {"module_path", "version", "product_version_file"}:
        raise HelperError("Go install release contract is invalid", {"go_install": value})
    normalized = {key: str(value.get(key) or "").strip() for key in value}
    if not all(normalized.values()):
        raise HelperError("Go install release contract is incomplete", {"go_install": normalized})
    if read_module_path(cwd) != normalized["module_path"]:
        raise HelperError(
            "Go install module path does not match the release contract",
            {"expected": normalized["module_path"], "actual": read_module_path(cwd)},
        )
    if not GO_INSTALL_TRANSPORT_RE.match(normalized["version"]):
        raise HelperError("Go install transport version must use the v1 module line", {"version": normalized["version"]})
    if normalized["version"] == product_version:
        raise HelperError("Go install transport version conflicts with the product version", {"version": product_version})
    product_version_path = repository_file(
        cwd,
        normalized["product_version_file"],
        "product_version_file",
    )
    if not product_version_path.is_file():
        raise HelperError("Go install product version file is missing", {"path": str(product_version_path)})
    actual_product_version = product_version_path.read_text(encoding="utf-8").strip()
    if actual_product_version != product_version:
        raise HelperError(
            "Go install product version file does not match the release",
            {"expected": product_version, "actual": actual_product_version},
        )
    if resolve_commit(cwd, normalized["version"], "go_install_version") != release_commit:
        raise HelperError(
            "Go install transport tag does not point at the release commit",
            {"version": normalized["version"], "release_commit": release_commit},
        )
    tag_object_type = run(
        ["git", "cat-file", "-t", f"refs/tags/{normalized['version']}"], cwd=cwd
    ).stdout.strip()
    if tag_object_type != "tag":
        raise HelperError(
            "Go install transport tag is not annotated",
            {"version": normalized["version"], "object_type": tag_object_type},
        )
    if verify_remote:
        remote_commit = ls_remote_tag_commit(cwd, normalized["version"])
        if remote_commit != release_commit:
            raise HelperError(
                "remote Go install transport tag does not match the release commit",
                {"version": normalized["version"], "remote_commit": remote_commit, "release_commit": release_commit},
            )
    return normalized


def command_write_product_version(args: argparse.Namespace) -> int:
    cwd = repo_root()
    product_version_path = repository_file(cwd, args.path, "product_version_file")
    if not product_version_path.is_file():
        fail("Go install product version file is missing", {"path": str(product_version_path)})
    product_version_path.write_text(args.version.strip() + "\n", encoding="utf-8")
    emit({"ok": True, "path": str(product_version_path), "version": args.version.strip()})
    return 0


def command_initialize_release_artifact(args: argparse.Namespace) -> int:
    cwd = repo_root()
    artifact_path = release_artifact_dir(cwd, args.artifact_dir)
    if artifact_path.exists():
        shutil.rmtree(artifact_path)
    (artifact_path / "payloads").mkdir(parents=True)
    staging = {
        "schema_version": 1,
        "artifact_kind": "mprlab.release.staging",
        "version": args.version,
        "source_commit": resolve_commit(cwd, args.source_commit, "source_commit"),
        "release_timestamp": parse_release_timestamp(args.release_timestamp).isoformat(),
    }
    go_install = go_install_from_args(args)
    if go_install is not None:
        staging["go_install"] = go_install
    (artifact_path / "staging.json").write_text(
        json.dumps(staging, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )
    emit({"ok": True, "artifact_dir": str(artifact_path), "staging": staging})
    return 0


def go_install_from_args(args: argparse.Namespace) -> dict[str, str] | None:
    values = {
        "module_path": getattr(args, "go_install_module", None),
        "version": getattr(args, "go_install_version", None),
        "product_version_file": getattr(args, "product_version_file", None),
    }
    supplied = {key: str(value).strip() for key, value in values.items() if value is not None}
    if not supplied:
        return None
    if set(supplied) != set(values) or not all(supplied.values()):
        raise HelperError("Go install release arguments are incomplete", {"go_install": supplied})
    return supplied


def inventory_payloads(artifact_path: Path) -> list[dict[str, Any]]:
    payload_root = artifact_path / "payloads"
    if not payload_root.is_dir():
        return []

    payloads: list[dict[str, Any]] = []
    for path in sorted(payload_root.rglob("*")):
        if path.is_symlink():
            raise HelperError("prepared release payloads must not contain symlinks", {"path": str(path)})
        if not path.is_file():
            continue
        relative_path = path.relative_to(artifact_path).as_posix()
        payloads.append(
            {
                "path": relative_path,
                "size": path.stat().st_size,
                "sha256": sha256_file(path),
            }
        )
    return payloads


def verify_payloads(artifact_path: Path, payloads: Any) -> list[dict[str, Any]]:
    if not isinstance(payloads, list):
        raise HelperError("prepared release payload inventory is invalid")

    expected_paths: set[str] = set()
    verified: list[dict[str, Any]] = []
    for entry in payloads:
        if not isinstance(entry, dict):
            raise HelperError("prepared release payload entry is invalid", {"entry": entry})
        relative_path = entry.get("path")
        if not isinstance(relative_path, str) or not relative_path.startswith("payloads/"):
            raise HelperError("prepared release payload path is invalid", {"path": relative_path})
        path = (artifact_path / relative_path).resolve()
        if artifact_path not in path.parents or not path.is_file() or path.is_symlink():
            raise HelperError("prepared release payload is missing or unsafe", {"path": relative_path})
        actual_size = path.stat().st_size
        actual_sha256 = sha256_file(path)
        if entry.get("size") != actual_size or entry.get("sha256") != actual_sha256:
            raise HelperError(
                "prepared release payload does not match the manifest",
                {
                    "path": relative_path,
                    "expected_size": entry.get("size"),
                    "actual_size": actual_size,
                    "expected_sha256": entry.get("sha256"),
                    "actual_sha256": actual_sha256,
                },
            )
        if relative_path in expected_paths:
            raise HelperError("prepared release payload path is duplicated", {"path": relative_path})
        expected_paths.add(relative_path)
        verified.append(entry)

    actual_paths = {entry["path"] for entry in inventory_payloads(artifact_path)}
    if actual_paths != expected_paths:
        raise HelperError(
            "prepared release payload inventory is incomplete",
            {"expected_paths": sorted(expected_paths), "actual_paths": sorted(actual_paths)},
        )
    return verified


def command_write_release_artifact(args: argparse.Namespace) -> int:
    cwd = repo_root()
    release_commit = resolve_commit(cwd, args.release_commit, "release_commit")
    source_commit = resolve_commit(cwd, args.source_commit, "source_commit")
    head_commit = resolve_commit(cwd, "HEAD", "head")
    tag_commit = resolve_commit(cwd, args.version, "version")
    if release_commit != head_commit:
        fail("release commit must be HEAD", {"release_commit": release_commit, "head": head_commit})
    if tag_commit != release_commit:
        fail("local release tag must point at the release commit", {"version": args.version, "tag_commit": tag_commit})

    parent_result = run(["git", "rev-parse", "--verify", f"{release_commit}^"], cwd=cwd, check=False)
    parent_commit = parent_result.stdout.strip() if parent_result.returncode == 0 else ""
    if parent_commit != source_commit:
        fail(
            "release commit must directly follow the prepared source commit",
            {"source_commit": source_commit, "release_parent": parent_commit},
        )

    go_install = go_install_from_args(args)
    validated_go_install = validate_go_install_contract(
        cwd,
        go_install,
        args.version,
        release_commit,
    )
    changed_files = sorted(run(
        ["git", "diff-tree", "--no-commit-id", "--name-only", "-r", release_commit], cwd=cwd
    ).stdout.splitlines())
    expected_changed_files = ["CHANGELOG.md"]
    if validated_go_install is not None:
        expected_changed_files.append(validated_go_install["product_version_file"])
    if changed_files != sorted(expected_changed_files):
        fail(
            "release commit does not contain the exact release metadata files",
            {"expected_changed_files": sorted(expected_changed_files), "changed_files": changed_files},
        )

    notes_source = Path(args.notes_file)
    notes = notes_source.read_text(encoding="utf-8").strip()
    if not notes:
        fail("release notes file is empty", {"notes_file": str(notes_source)})

    artifact_path = release_artifact_dir(cwd, args.artifact_dir)
    staging_path = artifact_path / "staging.json"
    if not staging_path.is_file():
        fail("prepared release staging area is missing", {"artifact_dir": str(artifact_path)})
    staging = json.loads(staging_path.read_text(encoding="utf-8"))
    expected_staging = {
        "artifact_kind": "mprlab.release.staging",
        "version": args.version,
        "source_commit": source_commit,
        "go_install": validated_go_install,
    }
    for key, expected_value in expected_staging.items():
        if staging.get(key) != expected_value:
            fail(
                "prepared release staging area does not match the release",
                {"field": key, "expected": expected_value, "actual": staging.get(key)},
            )

    notes_path = artifact_path / "notes.md"
    notes_path.write_text(notes + "\n", encoding="utf-8")
    payloads = inventory_payloads(artifact_path)
    manifest = {
        "schema_version": RELEASE_MANIFEST_SCHEMA,
        "artifact_kind": "mprlab.release",
        "version": args.version,
        "source_commit": source_commit,
        "release_commit": release_commit,
        "default_branch": args.default_branch,
        "release_timestamp": parse_release_timestamp(args.release_timestamp).isoformat(),
        "notes_sha256": sha256_file(notes_path),
        "payloads": payloads,
    }
    if validated_go_install is not None:
        manifest["go_install"] = validated_go_install
    manifest_path = artifact_path / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    staging_path.unlink()
    emit({"ok": True, "artifact_dir": str(artifact_path), "manifest": manifest})
    return 0


def load_release_artifact(cwd: Path, override: str | None = None) -> tuple[Path, dict[str, Any], Path]:
    artifact_path = release_artifact_dir(cwd, override)
    manifest_path = artifact_path / "manifest.json"
    notes_path = artifact_path / "notes.md"
    if not manifest_path.is_file() or not notes_path.is_file():
        raise HelperError(
            "prepared release artifact is missing; run make release",
            {"artifact_dir": str(artifact_path)},
        )
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    if manifest.get("schema_version") != RELEASE_MANIFEST_SCHEMA or manifest.get("artifact_kind") != "mprlab.release":
        raise HelperError("prepared release manifest has an invalid contract", {"manifest": str(manifest_path)})
    actual_notes_sha256 = sha256_file(notes_path)
    if manifest.get("notes_sha256") != actual_notes_sha256:
        raise HelperError(
            "prepared release notes do not match the manifest",
            {"expected": manifest.get("notes_sha256"), "actual": actual_notes_sha256},
        )
    verify_payloads(artifact_path, manifest.get("payloads"))
    return artifact_path, manifest, notes_path


def release_notes_from_changelog(cwd: Path, version: str) -> str:
    changelog_path = cwd / "CHANGELOG.md"
    if not changelog_path.is_file():
        raise HelperError("release changelog is missing", {"changelog": str(changelog_path)})
    changelog = changelog_path.read_text(encoding="utf-8")
    heading = re.compile(rf"^## \[{re.escape(version)}\](?:\s+-[^\n]*)?$", re.MULTILINE)
    match = heading.search(changelog)
    if not match:
        raise HelperError("release changelog entry is missing", {"version": version})
    next_heading = re.search(r"^##\s+", changelog[match.end() :], re.MULTILINE)
    end = match.end() + next_heading.start() if next_heading else len(changelog)
    return changelog[match.start() : end].strip()


def validate_exact_release_manifest(
    cwd: Path,
    manifest: dict[str, Any],
    notes: str,
    version: str,
    default_branch: str,
) -> None:
    if manifest.get("schema_version") != RELEASE_MANIFEST_SCHEMA or manifest.get("artifact_kind") != "mprlab.release":
        raise HelperError("published release manifest has an invalid contract", {"version": version})
    release_commit = str(manifest.get("release_commit") or "")
    source_commit = str(manifest.get("source_commit") or "")
    expected = {
        "version": version,
        "release_commit": resolve_commit(cwd, "HEAD", "head"),
        "source_commit": resolve_commit(cwd, "HEAD^", "release_parent"),
        "default_branch": default_branch,
    }
    actual = {
        "version": manifest.get("version"),
        "release_commit": release_commit,
        "source_commit": source_commit,
        "default_branch": manifest.get("default_branch"),
    }
    if actual != expected:
        raise HelperError(
            "published release manifest does not match exact tag",
            {"expected": expected, "actual": actual},
        )
    if resolve_commit(cwd, version, "version") != expected["release_commit"]:
        raise HelperError("exact release tag does not point at HEAD", {"version": version})
    tag_object_type = run(["git", "cat-file", "-t", f"refs/tags/{version}"], cwd=cwd).stdout.strip()
    if tag_object_type != "tag":
        raise HelperError("exact release tag is not annotated", {"version": version, "object_type": tag_object_type})
    go_install = validate_go_install_contract(
        cwd,
        manifest.get("go_install"),
        version,
        release_commit,
    )
    changed_files = sorted(run(
        ["git", "diff-tree", "--no-commit-id", "--name-only", "-r", release_commit], cwd=cwd
    ).stdout.splitlines())
    expected_changed_files = ["CHANGELOG.md"]
    if go_install is not None:
        expected_changed_files.append(go_install["product_version_file"])
    if changed_files != sorted(expected_changed_files):
        raise HelperError(
            "exact release commit does not contain the exact release metadata files",
            {
                "version": version,
                "expected_changed_files": sorted(expected_changed_files),
                "changed_files": changed_files,
            },
        )
    release_timestamp = manifest.get("release_timestamp")
    if not isinstance(release_timestamp, str) or not release_timestamp:
        raise HelperError("published release manifest has no release timestamp", {"version": version})
    parse_release_timestamp(release_timestamp)
    changelog_notes = release_notes_from_changelog(cwd, version)
    if normalize_markdown(notes) != normalize_markdown(changelog_notes):
        raise HelperError("exact release notes do not match CHANGELOG.md", {"version": version})


def load_exact_release_artifact(
    cwd: Path,
    version: str,
    default_branch: str,
    override: str | None = None,
) -> tuple[Path, dict[str, Any], Path]:
    artifact_path, manifest, notes_path = load_release_artifact(cwd, override)
    validate_exact_release_manifest(
        cwd,
        manifest,
        notes_path.read_text(encoding="utf-8"),
        version,
        default_branch,
    )
    return artifact_path, manifest, notes_path


def artifact_has_missing_files(artifact_path: Path, manifest: dict[str, Any]) -> bool:
    if not (artifact_path / "notes.md").is_file():
        return True
    payloads = manifest.get("payloads")
    if not isinstance(payloads, list):
        return False
    return any(
        not isinstance(entry, dict)
        or not isinstance(entry.get("path"), str)
        or not (artifact_path / entry["path"]).is_file()
        for entry in payloads
    )


def promote_release_artifact(cwd: Path, candidate_path: Path) -> Path:
    destination = release_artifact_dir(cwd)
    candidate = candidate_path.resolve()
    if candidate == destination or candidate.parent != destination.parent:
        raise HelperError(
            "release candidate must be a sibling of the canonical receipt",
            {"candidate": str(candidate), "destination": str(destination)},
        )
    backup = Path(tempfile.mkdtemp(prefix="mprlab-release-backup.", dir=destination.parent))
    backup.rmdir()
    moved_destination = False
    try:
        if destination.exists():
            destination.rename(backup)
            moved_destination = True
        candidate.rename(destination)
    except OSError as error:
        if moved_destination and backup.exists() and not destination.exists():
            backup.rename(destination)
        raise HelperError(
            "failed to promote verified release receipt",
            {"candidate": str(candidate), "destination": str(destination), "error": str(error)},
        ) from error
    if backup.exists():
        shutil.rmtree(backup)
    return destination


def published_release_asset_names(manifest: dict[str, Any]) -> list[str]:
    payloads = manifest.get("payloads")
    if not isinstance(payloads, list):
        raise HelperError("published release payload inventory is invalid")
    names = ["manifest.json"]
    prefix = "payloads/release-assets/"
    for entry in payloads:
        if not isinstance(entry, dict) or not isinstance(entry.get("path"), str):
            raise HelperError("published release payload entry is invalid", {"entry": entry})
        relative_path = entry["path"]
        canonical_path = PurePosixPath(relative_path)
        if (
            not relative_path.startswith(prefix)
            or canonical_path.is_absolute()
            or canonical_path.as_posix() != relative_path
            or ".." in canonical_path.parts
        ):
            raise HelperError(
                "published release payload path is not recoverable from GitHub Release",
                {"path": relative_path},
            )
        names.append(canonical_path.name)
    if len(names) != len(set(names)):
        raise HelperError("published release asset names are duplicated", {"asset_names": names})
    return names


def recover_published_exact_release(
    cwd: Path,
    version: str,
    default_branch: str,
    local_manifest_contents: bytes | None,
    dry_run: bool,
) -> Path | None:
    missing = require_tools(["git", "gh"])
    if missing:
        raise HelperError("required tools are missing", {"missing_tools": missing})
    release = gh_json(
        [
            "gh",
            "release",
            "view",
            version,
            "--json",
            "tagName,body,publishedAt,isDraft,isPrerelease,targetCommitish,url,assets",
        ],
        cwd,
    )
    if not isinstance(release, dict):
        raise HelperError("published GitHub Release response is invalid", {"version": version})
    release_errors: list[str] = []
    if release.get("tagName") != version:
        release_errors.append("tagName does not match")
    if release.get("isDraft"):
        release_errors.append("release is a draft")
    if release.get("isPrerelease"):
        release_errors.append("release is a prerelease")
    if not release.get("publishedAt"):
        release_errors.append("release has no publication timestamp")
    if release.get("targetCommitish") not in (default_branch, resolve_commit(cwd, "HEAD", "head")):
        release_errors.append("targetCommitish does not match the release")
    if release_errors:
        raise HelperError(
            "published GitHub Release does not match exact tag",
            {"version": version, "errors": release_errors, "url": release.get("url")},
        )
    remote_tag_commit = ls_remote_tag_commit(cwd, version)
    head_commit = resolve_commit(cwd, "HEAD", "head")
    if remote_tag_commit != head_commit:
        raise HelperError(
            "remote release tag does not match exact tag",
            {"version": version, "remote_tag_commit": remote_tag_commit, "head": head_commit},
        )

    destination = release_artifact_dir(cwd)
    destination.parent.mkdir(parents=True, exist_ok=True)
    candidate = Path(tempfile.mkdtemp(prefix="mprlab-release-candidate.", dir=destination.parent))
    try:
        manifest_path = candidate / "manifest.json"
        run(
            ["gh", "release", "download", version, "--pattern", "manifest.json", "--dir", str(candidate)],
            cwd=cwd,
        )
        if not manifest_path.is_file():
            raise HelperError("published release manifest asset is missing", {"version": version})
        remote_manifest_contents = manifest_path.read_bytes()
        if local_manifest_contents is not None and local_manifest_contents != remote_manifest_contents:
            raise HelperError("published release manifest conflicts with local sealed manifest", {"version": version})
        try:
            manifest = json.loads(remote_manifest_contents)
        except json.JSONDecodeError as error:
            raise HelperError(
                "published release manifest is invalid JSON",
                {"version": version, "error": str(error)},
            ) from error
        if not isinstance(manifest, dict):
            raise HelperError("published release manifest is not an object", {"version": version})
        notes = str(release.get("body") or "").strip() + "\n"
        validate_exact_release_manifest(cwd, manifest, notes, version, default_branch)
        validate_go_install_contract(
            cwd,
            manifest.get("go_install"),
            version,
            head_commit,
            verify_remote=True,
        )
        expected_asset_names = published_release_asset_names(manifest)
        actual_asset_names = sorted(
            asset.get("name")
            for asset in release.get("assets") or []
            if isinstance(asset, dict) and isinstance(asset.get("name"), str)
        )
        if sorted(expected_asset_names) != actual_asset_names:
            raise HelperError(
                "published GitHub Release assets do not match the manifest",
                {"expected": sorted(expected_asset_names), "actual": actual_asset_names},
            )
        (candidate / "notes.md").write_text(notes, encoding="utf-8")
        for entry in manifest["payloads"]:
            relative_path = entry["path"]
            asset_name = Path(relative_path).name
            download_directory = candidate / relative_path
            download_directory.parent.mkdir(parents=True, exist_ok=True)
            temporary_download = candidate / f"download-{asset_name}"
            temporary_download.mkdir()
            run(
                ["gh", "release", "download", version, "--pattern", asset_name, "--dir", str(temporary_download)],
                cwd=cwd,
            )
            downloaded_path = temporary_download / asset_name
            if not downloaded_path.is_file():
                raise HelperError(
                    "published release payload asset is missing",
                    {"version": version, "asset": asset_name},
                )
            downloaded_path.rename(download_directory)
            temporary_download.rmdir()
        load_exact_release_artifact(cwd, version, default_branch, str(candidate))
        if dry_run:
            emit({"ok": True, "action": "recover", "dry_run": True, "version": version})
            return None
        promoted = promote_release_artifact(cwd, candidate)
        emit({"ok": True, "action": "recovered", "version": version, "artifact_dir": str(promoted)})
        return promoted
    finally:
        if candidate.exists():
            shutil.rmtree(candidate)


def command_reuse_exact_release(args: argparse.Namespace) -> int:
    cwd = repo_root()
    artifact_path = release_artifact_dir(cwd, args.artifact_dir)
    manifest_path = artifact_path / "manifest.json"
    local_manifest_contents: bytes | None = None
    if manifest_path.is_file():
        local_manifest_contents = manifest_path.read_bytes()
        try:
            manifest = json.loads(local_manifest_contents)
        except json.JSONDecodeError as error:
            raise HelperError(
                "local sealed release manifest is invalid JSON",
                {"manifest": str(manifest_path), "error": str(error)},
            ) from error
        if not isinstance(manifest, dict):
            raise HelperError("local sealed release manifest is not an object", {"manifest": str(manifest_path)})
        try:
            validate_exact_release_manifest(
                cwd,
                manifest,
                (artifact_path / "notes.md").read_text(encoding="utf-8")
                if (artifact_path / "notes.md").is_file()
                else release_notes_from_changelog(cwd, args.version),
                args.version,
                args.default_branch,
            )
        except HelperError as error:
            raise HelperError(
                "local sealed release conflicts with exact tag",
                {"version": args.version, "error": str(error), "details": error.details},
            ) from error
        if not artifact_has_missing_files(artifact_path, manifest):
            try:
                load_exact_release_artifact(cwd, args.version, args.default_branch, args.artifact_dir)
            except HelperError as error:
                raise HelperError(
                    "local sealed release conflicts with exact tag",
                    {"version": args.version, "error": str(error), "details": error.details},
                ) from error
            emit(
                {
                    "ok": True,
                    "action": "reuse",
                    "dry_run": args.dry_run,
                    "version": args.version,
                    "artifact_dir": str(artifact_path),
                }
            )
            print(f"Reused sealed release {args.version}.")
            return 0

    recovered = recover_published_exact_release(
        cwd,
        args.version,
        args.default_branch,
        local_manifest_contents,
        args.dry_run,
    )
    if not args.dry_run and recovered is None:
        raise HelperError("published exact release recovery did not produce a receipt", {"version": args.version})
    if args.dry_run:
        print(f"Verified recoverable exact release {args.version}.")
    else:
        print(f"Recovered and reused sealed release {args.version}.")
    return 0


def command_promote_release_artifact(args: argparse.Namespace) -> int:
    cwd = repo_root()
    candidate, manifest, _ = load_release_artifact(cwd, args.artifact_dir)
    destination = promote_release_artifact(cwd, candidate)
    emit({"ok": True, "artifact_dir": str(destination), "manifest": manifest})
    return 0


def command_verify_release_artifact(args: argparse.Namespace) -> int:
    cwd = repo_root()
    artifact_path, manifest, _ = load_release_artifact(cwd, args.artifact_dir)
    emit({"ok": True, "artifact_dir": str(artifact_path), "manifest": manifest})
    return 0


def release_asset_paths(artifact_path: Path, manifest: dict[str, Any]) -> list[Path]:
    prefix = "payloads/release-assets/"
    assets = [artifact_path / "manifest.json"] + [
        artifact_path / entry["path"]
        for entry in manifest.get("payloads", [])
        if entry["path"].startswith(prefix)
    ]
    names = [path.name for path in assets]
    if len(names) != len(set(names)):
        raise HelperError("GitHub Release asset names must be unique", {"asset_names": names})
    return assets


def publish_release_assets(cwd: Path, version: str, assets: list[Path]) -> list[dict[str, Any]]:
    if not assets:
        return []

    run(["gh", "release", "upload", version, *[str(path) for path in assets], "--clobber"], cwd=cwd)
    published: list[dict[str, Any]] = []
    with tempfile.TemporaryDirectory(prefix="mprlab-release-assets-") as temporary_directory:
        download_root = Path(temporary_directory)
        for asset in assets:
            asset_dir = download_root / asset.name
            asset_dir.mkdir()
            run(
                ["gh", "release", "download", version, "--pattern", asset.name, "--dir", str(asset_dir)],
                cwd=cwd,
            )
            downloaded = asset_dir / asset.name
            expected_sha256 = sha256_file(asset)
            actual_sha256 = sha256_file(downloaded)
            if actual_sha256 != expected_sha256:
                raise HelperError(
                    "published GitHub Release asset does not match the prepared payload",
                    {
                        "asset": asset.name,
                        "expected_sha256": expected_sha256,
                        "actual_sha256": actual_sha256,
                    },
                )
            published.append(
                {
                    "name": asset.name,
                    "sha256": actual_sha256,
                    "size": downloaded.stat().st_size,
                }
            )
    return published


def command_publish_prepared_release(args: argparse.Namespace) -> int:
    missing = require_tools(["git", "gh"])
    if missing:
        fail("required tools are missing", {"missing_tools": missing})

    cwd = repo_root()
    artifact_path, manifest, notes_path = load_release_artifact(cwd, args.artifact_dir)
    version = str(manifest.get("version") or "")
    default_branch = str(manifest.get("default_branch") or "")
    release_commit = str(manifest.get("release_commit") or "")
    source_commit = str(manifest.get("source_commit") or "")
    release_assets = release_asset_paths(artifact_path, manifest)
    if not all((version, default_branch, release_commit, source_commit)):
        fail("prepared release manifest is incomplete", {"artifact_dir": str(artifact_path)})
    go_install = validate_go_install_contract(
        cwd,
        manifest.get("go_install"),
        version,
        release_commit,
    )

    current_branch = run(["git", "branch", "--show-current"], cwd=cwd).stdout.strip()
    dirty_status = run(["git", "status", "--short"], cwd=cwd).stdout.splitlines()
    head_commit = resolve_commit(cwd, "HEAD", "head")
    tag_commit = resolve_commit(cwd, version, "version")
    errors: list[str] = []
    if current_branch != default_branch:
        errors.append(f"current branch is {current_branch or '<detached>'}; expected {default_branch}")
    if dirty_status:
        errors.append("worktree is dirty")
    if head_commit != release_commit:
        errors.append("HEAD does not match the prepared release commit")
    if tag_commit != release_commit:
        errors.append("local release tag does not match the prepared release commit")
    if errors:
        fail("prepared release is not publishable", {"errors": errors, "dirty_status": dirty_status})

    remote_ref = f"refs/remotes/{RELEASE_REMOTE}/{default_branch}"
    run(
        [
            "git",
            "fetch",
            "--prune",
            RELEASE_REMOTE,
            f"+refs/heads/{default_branch}:{remote_ref}",
        ],
        cwd=cwd,
    )
    remote_branch_commit = resolve_commit(cwd, remote_ref, "remote_branch")
    if remote_branch_commit not in (source_commit, release_commit):
        fail(
            "remote default branch changed after make release; reconcile and run make release again",
            {
                "remote_branch": remote_branch_commit,
                "prepared_source_commit": source_commit,
                "prepared_release_commit": release_commit,
            },
        )

    open_prs = gh_json(
        ["gh", "pr", "list", "--base", default_branch, "--state", "open", "--json", "number,title,headRefName,url"],
        cwd,
    )
    if open_prs:
        fail("open pull requests target the default branch", {"open_prs": open_prs})

    remote_tag_commit = ls_remote_tag_commit(cwd, version)
    if remote_tag_commit and remote_tag_commit != release_commit:
        fail(
            "remote release tag points at a different commit",
            {"version": version, "remote_tag_commit": remote_tag_commit, "release_commit": release_commit},
        )
    remote_go_install_commit = ls_remote_tag_commit(cwd, go_install["version"]) if go_install else ""
    if remote_go_install_commit and remote_go_install_commit != release_commit:
        fail(
            "remote Go install transport tag points at a different commit",
            {
                "version": go_install["version"],
                "remote_tag_commit": remote_go_install_commit,
                "release_commit": release_commit,
            },
        )

    plan = {
        "push_branch": remote_branch_commit != release_commit,
        "push_tag": not remote_tag_commit,
        "push_go_install_tag": bool(go_install and not remote_go_install_commit),
        "publish_github_release": True,
        "release_assets": [path.name for path in release_assets],
    }
    if args.dry_run:
        emit(
            {
                "ok": True,
                "dry_run": True,
                "artifact_dir": str(artifact_path),
                "version": version,
                "release_commit": release_commit,
                "remote": RELEASE_REMOTE,
                "plan": plan,
            }
        )
        return 0

    if plan["push_branch"]:
        run(["git", "push", RELEASE_REMOTE, f"HEAD:refs/heads/{default_branch}"], cwd=cwd)
    if plan["push_tag"]:
        run(["git", "push", RELEASE_REMOTE, f"refs/tags/{version}:refs/tags/{version}"], cwd=cwd)
    if go_install and plan["push_go_install_tag"]:
        run(
            [
                "git",
                "push",
                RELEASE_REMOTE,
                f"refs/tags/{go_install['version']}:refs/tags/{go_install['version']}",
            ],
            cwd=cwd,
        )
    if go_install:
        validate_go_install_contract(cwd, go_install, version, release_commit, verify_remote=True)

    publish_args = argparse.Namespace(version=version, notes_file=str(notes_path), title=None)
    if command_publish_release(publish_args) != 0:
        return 1
    published_assets = publish_release_assets(cwd, version, release_assets)
    verify_args = argparse.Namespace(
        version=version,
        release_commit=release_commit,
        notes_file=str(notes_path),
        default_branch=default_branch,
        watch_run=[],
        skip_pages=True,
        expect_pages_text=[],
    )
    verify_result = command_verify_release(verify_args)
    if verify_result == 0 and published_assets:
        emit({"ok": True, "published_release_assets": published_assets})
    return verify_result


def normalize_markdown(text: str) -> str:
    return "\n".join(line.rstrip() for line in text.strip().splitlines()).strip()


def command_insert_changelog(args: argparse.Namespace) -> int:
    cwd = repo_root()
    notes_path = Path(args.notes_file)
    notes = notes_path.read_text(encoding="utf-8").strip()
    if not notes:
        fail("release notes file is empty", {"notes_file": str(notes_path)})

    changelog = cwd / args.changelog
    if changelog.exists():
        existing = changelog.read_text(encoding="utf-8")
    else:
        existing = "# Changelog\n\n"

    first_heading = next((line.strip() for line in notes.splitlines() if line.startswith("## ")), None)
    if first_heading and re.search(rf"^{re.escape(first_heading)}$", existing, re.MULTILINE):
        if normalize_markdown(notes) in normalize_markdown(existing):
            emit({"ok": True, "changed": False, "changelog": str(changelog), "reason": "release notes already present"})
            return 0
        fail("changelog already contains a matching release heading with different content", {"heading": first_heading})

    section = notes.rstrip() + "\n\n"
    match = RELEASE_HEADING_RE.search(existing)
    if match:
        updated = existing[: match.start()] + section + existing[match.start() :]
    else:
        h1 = re.search(r"^# .*$", existing, re.MULTILINE)
        if h1:
            insert_at = h1.end()
            while insert_at < len(existing) and existing[insert_at] == "\n":
                insert_at += 1
            updated = existing[:insert_at].rstrip() + "\n\n" + section + existing[insert_at:].lstrip()
        else:
            updated = section + existing.lstrip()

    changelog.write_text(updated, encoding="utf-8")
    emit({"ok": True, "changed": updated != existing, "changelog": str(changelog)})
    return 0


def command_publish_release(args: argparse.Namespace) -> int:
    missing = require_tools(["git", "gh"])
    if missing:
        fail("required tools are missing", {"missing_tools": missing})

    cwd = repo_root()
    notes_path = Path(args.notes_file)
    expected_notes = normalize_markdown(notes_path.read_text(encoding="utf-8"))
    if not expected_notes:
        fail("release notes file is empty", {"notes_file": str(notes_path)})

    title = args.title or f"Release {args.version}"
    view_command = [
        "gh",
        "release",
        "view",
        args.version,
        "--json",
        "tagName,name,body,publishedAt,isDraft,isPrerelease,targetCommitish,url",
    ]
    existing_proc = run(view_command, cwd=cwd, check=False)
    action = "none"
    command: list[str] | None = None

    if existing_proc.returncode != 0:
        action = "created"
        command = [
            "gh",
            "release",
            "create",
            args.version,
            "--verify-tag",
            "--title",
            title,
            "--notes-file",
            str(notes_path),
            "--latest",
        ]
    else:
        existing = json.loads(existing_proc.stdout)
        actual_notes = normalize_markdown(existing.get("body") or "")
        needs_edit = (
            existing.get("tagName") != args.version
            or existing.get("name") != title
            or existing.get("isDraft")
            or actual_notes != expected_notes
        )
        if needs_edit:
            action = "updated"
            command = [
                "gh",
                "release",
                "edit",
                args.version,
                "--verify-tag",
                "--title",
                title,
                "--notes-file",
                str(notes_path),
                "--draft=false",
                "--latest",
            ]

    if command:
        run(command, cwd=cwd)

    refreshed = gh_json(view_command, cwd)
    errors: list[str] = []
    if refreshed.get("tagName") != args.version:
        errors.append("GitHub Release object has the wrong tagName")
    if refreshed.get("isDraft"):
        errors.append("GitHub Release object is still a draft")
    if not refreshed.get("publishedAt"):
        errors.append("GitHub Release object has no publishedAt timestamp")
    if normalize_markdown(refreshed.get("body") or "") != expected_notes:
        errors.append("GitHub Release body does not match generated release notes")

    payload = {"ok": not errors, "action": action, "release": refreshed, "errors": errors}
    emit(payload)
    return 0 if not errors else 1


def ls_remote_tag_commit(cwd: Path, version: str) -> str:
    peeled = run(["git", "ls-remote", "--tags", RELEASE_REMOTE, f"refs/tags/{version}^{{}}"], cwd=cwd).stdout.strip()
    if peeled:
        return peeled.split()[0]
    direct = run(["git", "ls-remote", "--tags", RELEASE_REMOTE, f"refs/tags/{version}"], cwd=cwd).stdout.strip()
    return direct.split()[0] if direct else ""


def fetch_url(url: str, head_only: bool = False) -> dict[str, Any]:
    method = "HEAD" if head_only else "GET"
    request = urllib.request.Request(url, method=method, headers={"User-Agent": "gitrelease-helper/1"})
    try:
        with urllib.request.urlopen(request, timeout=20) as response:
            body = "" if head_only else response.read(1_000_000).decode("utf-8", errors="replace")
            return {"ok": True, "status": response.status, "url": response.geturl(), "body": body}
    except urllib.error.HTTPError as exc:
        return {"ok": False, "status": exc.code, "error": str(exc), "url": url}
    except urllib.error.URLError as exc:
        return {"ok": False, "error": str(exc), "url": url}


def optional_gh_json(command: list[str], cwd: Path) -> dict[str, Any]:
    proc = run(command, cwd=cwd, check=False)
    if proc.returncode == 0:
        return {"ok": True, "data": json.loads(proc.stdout or "null")}
    return {"ok": False, "returncode": proc.returncode, "stderr": proc.stderr.strip(), "stdout": proc.stdout.strip()}


def collect_pages(cwd: Path, expected_texts: list[str]) -> tuple[dict[str, Any], list[str]]:
    errors: list[str] = []
    pages = optional_gh_json(["gh", "api", "repos/{owner}/{repo}/pages"], cwd)
    if not pages["ok"]:
        stderr = pages.get("stderr", "")
        if "404" in stderr or "Not Found" in stderr:
            return {"configured": False, "lookup": pages}, []
        errors.append("GitHub Pages configuration lookup failed")
        return {"configured": None, "lookup": pages}, errors

    data = pages["data"] or {}
    html_url = data.get("html_url")
    result: dict[str, Any] = {
        "configured": True,
        "config": data,
        "latest_build": optional_gh_json(
            ["gh", "api", "repos/{owner}/{repo}/pages/builds/latest", "--jq", "{status,error,commit,created_at,updated_at,url}"],
            cwd,
        ),
        "latest_deployment": optional_gh_json(
            [
                "gh",
                "api",
                "repos/{owner}/{repo}/deployments?environment=github-pages",
                "--jq",
                ".[0] | {id,sha,ref,created_at,statuses_url}",
            ],
            cwd,
        ),
    }
    if not html_url:
        errors.append("GitHub Pages is configured but has no html_url")
        return result, errors

    head = fetch_url(html_url, head_only=True)
    if not head["ok"]:
        head = fetch_url(html_url, head_only=False)
    result["site_probe"] = {key: value for key, value in head.items() if key != "body"}
    if not head["ok"]:
        errors.append("GitHub Pages URL is not reachable")

    if expected_texts:
        page = fetch_url(html_url, head_only=False)
        body = page.get("body", "") if page["ok"] else ""
        missing = [text for text in expected_texts if text not in body]
        result["expected_text_check"] = {"ok": not missing, "missing": missing}
        if missing:
            errors.append("GitHub Pages URL does not contain expected release text")

    return result, errors


def collect_runs(cwd: Path, default_branch: str, release_commit: str) -> dict[str, Any]:
    return {
        "for_release_commit": optional_gh_json(
            [
                "gh",
                "run",
                "list",
                "--commit",
                release_commit,
                "--json",
                "databaseId,name,event,status,conclusion,headSha,url",
                "--limit",
                "20",
            ],
            cwd,
        ),
        "release_events": optional_gh_json(
            [
                "gh",
                "run",
                "list",
                "--event",
                "release",
                "--json",
                "databaseId,name,event,status,conclusion,headSha,url",
                "--limit",
                "20",
            ],
            cwd,
        ),
        "default_branch_push_events": optional_gh_json(
            [
                "gh",
                "run",
                "list",
                "--event",
                "push",
                "--branch",
                default_branch,
                "--json",
                "databaseId,name,event,status,conclusion,headSha,url",
                "--limit",
                "20",
            ],
            cwd,
        ),
    }


def command_verify_release(args: argparse.Namespace) -> int:
    missing = require_tools(["git", "gh"])
    if missing:
        fail("required tools are missing", {"missing_tools": missing})

    cwd = repo_root()
    default_branch = resolve_default_branch(cwd, args.default_branch)
    release_commit = run(["git", "rev-parse", args.release_commit], cwd=cwd).stdout.strip()
    errors: list[str] = []

    local_tag_proc = run(["git", "rev-list", "-n", "1", args.version], cwd=cwd, check=False)
    local_tag_commit = local_tag_proc.stdout.strip() if local_tag_proc.returncode == 0 else ""
    remote_tag_commit = ls_remote_tag_commit(cwd, args.version)
    if local_tag_commit != release_commit:
        errors.append("local tag does not point at release commit")
    if remote_tag_commit != release_commit:
        errors.append("remote tag does not point at release commit")

    release_proc = run(
        [
            "gh",
            "release",
            "view",
            args.version,
            "--json",
            "tagName,name,body,publishedAt,isDraft,isPrerelease,targetCommitish,url",
        ],
        cwd=cwd,
        check=False,
    )
    release: dict[str, Any] | None = None
    if release_proc.returncode != 0:
        errors.append("GitHub Release object is missing or unreadable")
    else:
        release = json.loads(release_proc.stdout)
        if release.get("tagName") != args.version:
            errors.append("GitHub Release object has the wrong tagName")
        if release.get("isDraft"):
            errors.append("GitHub Release object is still a draft")
        if not release.get("publishedAt"):
            errors.append("GitHub Release object has no publishedAt timestamp")
        if args.notes_file:
            expected_notes = normalize_markdown(Path(args.notes_file).read_text(encoding="utf-8"))
            actual_notes = normalize_markdown(release.get("body") or "")
            if expected_notes != actual_notes:
                errors.append("GitHub Release body does not match generated release notes")

    watched_runs: list[dict[str, Any]] = []
    for run_id in args.watch_run:
        proc = run(["gh", "run", "watch", str(run_id), "--exit-status"], cwd=cwd, check=False)
        watched_runs.append(
            {
                "run_id": run_id,
                "returncode": proc.returncode,
                "stdout": proc.stdout.strip(),
                "stderr": proc.stderr.strip(),
            }
        )
        if proc.returncode != 0:
            errors.append(f"watched GitHub Actions run failed or did not complete: {run_id}")

    pages, page_errors = ({"skipped": True}, [])
    if not args.skip_pages:
        pages, page_errors = collect_pages(cwd, args.expect_pages_text)
        errors.extend(page_errors)

    payload = {
        "ok": not errors,
        "repo_root": str(cwd),
        "default_branch": default_branch,
        "version": args.version,
        "release_commit": release_commit,
        "local_tag_commit": local_tag_commit,
        "remote_tag_commit": remote_tag_commit,
        "release": release,
        "runs": collect_runs(cwd, default_branch, release_commit),
        "watched_runs": watched_runs,
        "pages": pages,
        "final_status": run(["git", "status", "--short"], cwd=cwd).stdout.splitlines(),
        "errors": errors,
    }
    emit(payload)
    return 0 if not errors else 1


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Deterministic helper for the Git Release skill.")
    subparsers = parser.add_subparsers(dest="command", required=True)

    preflight = subparsers.add_parser("preflight", help="Check deterministic release preconditions.")
    preflight.add_argument("--default-branch")
    preflight.add_argument("--release-date", help="Release date in YYYY-MM-DD format. Used as midnight if no timestamp is provided.")
    preflight.add_argument(
        "--release-timestamp",
        help="Release timestamp in ISO format for CalVer candidate generation, for example 2026-04-29T06:17:41 -> 26.429.61741.",
    )
    preflight.add_argument(
        "--local",
        action="store_true",
        help="Use only local Git state. Do not query GitHub or a remote repository.",
    )
    preflight.set_defaults(func=command_preflight)

    notes = subparsers.add_parser("generate-notes", help="Generate deterministic release notes from local Git history.")
    notes.add_argument("--version", required=True)
    notes.add_argument("--release-date", required=True)
    notes.add_argument("--since-tag")
    notes.set_defaults(func=command_generate_notes)

    changelog = subparsers.add_parser("insert-changelog", help="Insert generated release notes into CHANGELOG.md.")
    changelog.add_argument("--notes-file", required=True)
    changelog.add_argument("--changelog", default="CHANGELOG.md")
    changelog.set_defaults(func=command_insert_changelog)

    product_version = subparsers.add_parser(
        "write-product-version",
        help="Write the product version embedded in a root Go module release.",
    )
    product_version.add_argument("--path", required=True)
    product_version.add_argument("--version", required=True)
    product_version.set_defaults(func=command_write_product_version)

    publish = subparsers.add_parser("publish-release", help="Create or update the GitHub Release object.")
    publish.add_argument("--version", required=True)
    publish.add_argument("--notes-file", required=True)
    publish.add_argument("--title")
    publish.set_defaults(func=command_publish_release)

    initialize_artifact = subparsers.add_parser(
        "initialize-release-artifact",
        help="Create an empty local staging area for release payloads.",
    )
    initialize_artifact.add_argument("--version", required=True)
    initialize_artifact.add_argument("--source-commit", required=True)
    initialize_artifact.add_argument("--release-timestamp", required=True)
    initialize_artifact.add_argument("--artifact-dir")
    initialize_artifact.add_argument("--go-install-module")
    initialize_artifact.add_argument("--go-install-version")
    initialize_artifact.add_argument("--product-version-file")
    initialize_artifact.set_defaults(func=command_initialize_release_artifact)

    artifact = subparsers.add_parser(
        "write-release-artifact",
        help="Write the prepared local release manifest and notes under the repository Git directory.",
    )
    artifact.add_argument("--version", required=True)
    artifact.add_argument("--source-commit", required=True)
    artifact.add_argument("--release-commit", required=True)
    artifact.add_argument("--notes-file", required=True)
    artifact.add_argument("--default-branch", required=True)
    artifact.add_argument("--release-timestamp", required=True)
    artifact.add_argument("--artifact-dir")
    artifact.add_argument("--go-install-module")
    artifact.add_argument("--go-install-version")
    artifact.add_argument("--product-version-file")
    artifact.set_defaults(func=command_write_release_artifact)

    verify_artifact = subparsers.add_parser(
        "verify-release-artifact",
        help="Verify the prepared local release manifest, notes, and payload hashes.",
    )
    verify_artifact.add_argument("--artifact-dir")
    verify_artifact.set_defaults(func=command_verify_release_artifact)

    reuse_exact = subparsers.add_parser(
        "reuse-exact-release",
        help="Verify and reuse or recover the sealed release for an exact tag at HEAD.",
    )
    reuse_exact.add_argument("--version", required=True)
    reuse_exact.add_argument("--default-branch", required=True)
    reuse_exact.add_argument("--artifact-dir")
    reuse_exact.add_argument("--dry-run", action="store_true")
    reuse_exact.set_defaults(func=command_reuse_exact_release)

    promote_artifact = subparsers.add_parser(
        "promote-release-artifact",
        help="Atomically replace the canonical receipt with a verified release candidate.",
    )
    promote_artifact.add_argument("--artifact-dir", required=True)
    promote_artifact.set_defaults(func=command_promote_release_artifact)

    publish_prepared = subparsers.add_parser(
        "publish-prepared-release",
        help="Push the prepared branch/tag and publish the matching GitHub Release object.",
    )
    publish_prepared.add_argument("--artifact-dir")
    publish_prepared.add_argument("--dry-run", action="store_true")
    publish_prepared.set_defaults(func=command_publish_prepared_release)

    verify = subparsers.add_parser("verify-release", help="Verify remote tag, GitHub Release, runs, and Pages.")
    verify.add_argument("--version", required=True)
    verify.add_argument("--release-commit", required=True)
    verify.add_argument("--notes-file")
    verify.add_argument("--default-branch")
    verify.add_argument("--watch-run", action="append", default=[])
    verify.add_argument("--skip-pages", action="store_true")
    verify.add_argument("--expect-pages-text", action="append", default=[])
    verify.set_defaults(func=command_verify_release)

    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()
    try:
        return args.func(args)
    except HelperError as exc:
        fail(str(exc), exc.details)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
