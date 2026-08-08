#!/usr/bin/env python3
"""Publish the audited one-off SemVer correction for the Gix release history."""

from __future__ import annotations

import argparse
import copy
import gzip
import hashlib
import io
import json
import shutil
import subprocess
import sys
import tarfile
import tempfile
from dataclasses import dataclass
from pathlib import Path, PurePosixPath
from typing import Any


REPOSITORY = "tyemirov/gix"
DEFAULT_BRANCH = "master"
MANIFEST_NAME = "manifest.json"
PAGES_NAME = "pages.tar.gz"
EXPECTED_PAYLOAD_NAMES = {
    "checksums.txt",
    "gix_darwin_amd64",
    "gix_darwin_arm64",
    "gix_linux_amd64",
    "gix_linux_arm64",
    "gix_windows_amd64.exe",
    PAGES_NAME,
}


@dataclass(frozen=True)
class ReleaseAlias:
    corrected_version: str
    original_version: str
    source_commit: str
    release_commit: str


ALIASES = (
    ReleaseAlias(
        "v2.0.0",
        "v1.1.9",
        "1a977121f6d7019f6daa5031ab05af1f8b2d86f7",
        "eb37940e32a26812cfbe0df7e559ddf1ffc757a8",
    ),
    ReleaseAlias(
        "v3.0.0",
        "v1.1.10",
        "bb993f36fad49ac20cae6dcf5f3e067bbe3e847c",
        "07dac8d36fea5ac95af8b4e51f30a78ffd455522",
    ),
    ReleaseAlias(
        "v4.0.0",
        "v1.1.23",
        "b8fb3631d39309e80f4a3feed6c5a275400ef572",
        "81cb5cdc65d8bc16a79f470e942dc919f4d383d9",
    ),
    ReleaseAlias(
        "v4.1.0",
        "v1.1.24",
        "5ad4a72ab1f503e558a3c23f58c08086a2e05104",
        "8ec60f45e37565ad9eab35da254f568e88058331",
    ),
    ReleaseAlias(
        "v5.0.0",
        "v1.1.25",
        "adaf3d5b383d7d69e2610545484be959d3c927a7",
        "e4056555fa9e0eb157cc08ec14e120c604809322",
    ),
)


class MigrationError(RuntimeError):
    """The historical release state does not match the audited contract."""


def run(
    arguments: list[str],
    *,
    cwd: Path,
    check: bool = True,
) -> subprocess.CompletedProcess[str]:
    result = subprocess.run(
        arguments,
        cwd=cwd,
        check=False,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if check and result.returncode != 0:
        detail = result.stderr.strip() or result.stdout.strip() or "command failed"
        raise MigrationError(f"{arguments[0]} failed: {detail}")
    return result


def sha256_bytes(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def json_bytes(value: Any) -> bytes:
    return (json.dumps(value, indent=2, sort_keys=True) + "\n").encode()


def corrected_notes(body: str, alias: ReleaseAlias) -> str:
    original_heading = f"## [{alias.original_version}] - "
    if not body.startswith(original_heading):
        raise MigrationError(
            f"{alias.original_version} release notes do not start with the canonical heading"
        )
    if body.count(alias.original_version) != 1:
        raise MigrationError(
            f"{alias.original_version} release notes contain an ambiguous version reference"
        )
    return body.replace(alias.original_version, alias.corrected_version, 1).rstrip() + "\n"


def validate_manifest(manifest: dict[str, Any], alias: ReleaseAlias) -> list[dict[str, Any]]:
    expected_identity = {
        "schema_version": 2,
        "artifact_kind": "mprlab.release",
        "version": alias.original_version,
        "source_commit": alias.source_commit,
        "release_commit": alias.release_commit,
        "default_branch": DEFAULT_BRANCH,
    }
    for key, expected in expected_identity.items():
        if manifest.get(key) != expected:
            raise MigrationError(
                f"{alias.original_version} manifest {key} mismatch: "
                f"expected {expected!r}, found {manifest.get(key)!r}"
            )
    timestamp = manifest.get("release_timestamp")
    if not isinstance(timestamp, str) or not timestamp:
        raise MigrationError(f"{alias.original_version} manifest has no release timestamp")
    notes_hash = manifest.get("notes_sha256")
    if not isinstance(notes_hash, str) or len(notes_hash) != 64:
        raise MigrationError(f"{alias.original_version} manifest has an invalid notes hash")

    payloads = manifest.get("payloads")
    if not isinstance(payloads, list):
        raise MigrationError(f"{alias.original_version} manifest payload inventory is invalid")
    payload_names: set[str] = set()
    for payload in payloads:
        if not isinstance(payload, dict):
            raise MigrationError(f"{alias.original_version} manifest payload entry is invalid")
        path_value = payload.get("path")
        size_value = payload.get("size")
        hash_value = payload.get("sha256")
        if not isinstance(path_value, str) or not path_value.startswith("payloads/release-assets/"):
            raise MigrationError(f"{alias.original_version} payload path is invalid: {path_value!r}")
        name = PurePosixPath(path_value).name
        if name in payload_names:
            raise MigrationError(f"{alias.original_version} payload name is duplicated: {name}")
        if not isinstance(size_value, int) or size_value < 0:
            raise MigrationError(f"{alias.original_version} payload size is invalid: {name}")
        if not isinstance(hash_value, str) or len(hash_value) != 64:
            raise MigrationError(f"{alias.original_version} payload hash is invalid: {name}")
        payload_names.add(name)
    if payload_names != EXPECTED_PAYLOAD_NAMES:
        raise MigrationError(
            f"{alias.original_version} payload inventory mismatch: "
            f"expected {sorted(EXPECTED_PAYLOAD_NAMES)}, found {sorted(payload_names)}"
        )
    return payloads


def validate_source_assets(
    source_assets: Path,
    manifest: dict[str, Any],
    notes: str,
    alias: ReleaseAlias,
) -> None:
    actual_names = {path.name for path in source_assets.iterdir() if path.is_file()}
    expected_names = EXPECTED_PAYLOAD_NAMES | {MANIFEST_NAME}
    if actual_names != expected_names:
        raise MigrationError(
            f"{alias.original_version} GitHub asset inventory mismatch: "
            f"expected {sorted(expected_names)}, found {sorted(actual_names)}"
        )
    if sha256_bytes(notes.encode()) != manifest["notes_sha256"]:
        raise MigrationError(f"{alias.original_version} release notes do not match its manifest")
    for payload in manifest["payloads"]:
        path = source_assets / PurePosixPath(payload["path"]).name
        if path.stat().st_size != payload["size"] or sha256_file(path) != payload["sha256"]:
            raise MigrationError(
                f"{alias.original_version} asset does not match its manifest: {path.name}"
            )


def rewrite_pages_archive(
    source_archive: Path,
    destination_archive: Path,
    alias: ReleaseAlias,
    manifest: dict[str, Any],
) -> None:
    marker_seen = False
    destination_archive.parent.mkdir(parents=True, exist_ok=True)
    with tarfile.open(source_archive, "r:gz") as source:
        with destination_archive.open("wb") as raw_destination:
            with gzip.GzipFile(fileobj=raw_destination, mode="wb", mtime=0) as compressed:
                with tarfile.open(fileobj=compressed, mode="w") as destination:
                    for member in source.getmembers():
                        normalized_name = member.name.removeprefix("./")
                        path = PurePosixPath(normalized_name)
                        if member.name.startswith("/") or ".." in path.parts:
                            raise MigrationError(
                                f"{alias.original_version} Pages archive contains an unsafe path: {member.name}"
                            )
                        if member.issym() or member.islnk():
                            raise MigrationError(
                                f"{alias.original_version} Pages archive contains a link: {member.name}"
                            )
                        replacement = copy.copy(member)
                        if normalized_name == ".mprlab-release.json":
                            if marker_seen or not member.isfile():
                                raise MigrationError(
                                    f"{alias.original_version} Pages marker is duplicated or invalid"
                                )
                            original_file = source.extractfile(member)
                            if original_file is None:
                                raise MigrationError(
                                    f"{alias.original_version} Pages marker cannot be read"
                                )
                            original_marker = json.loads(original_file.read())
                            expected_marker = {
                                "schema_version": 1,
                                "release_version": alias.original_version,
                                "source_commit": alias.source_commit,
                                "release_timestamp": manifest["release_timestamp"],
                            }
                            if original_marker != expected_marker:
                                raise MigrationError(
                                    f"{alias.original_version} Pages marker does not match the source release"
                                )
                            corrected_marker = dict(expected_marker)
                            corrected_marker["release_version"] = alias.corrected_version
                            contents = json_bytes(corrected_marker)
                            replacement.size = len(contents)
                            destination.addfile(replacement, io.BytesIO(contents))
                            marker_seen = True
                            continue
                        if member.isfile():
                            source_file = source.extractfile(member)
                            if source_file is None:
                                raise MigrationError(
                                    f"{alias.original_version} Pages file cannot be read: {member.name}"
                                )
                            destination.addfile(replacement, source_file)
                        else:
                            destination.addfile(replacement)
    if not marker_seen:
        raise MigrationError(f"{alias.original_version} Pages marker is missing")


def prepare_alias(
    alias: ReleaseAlias,
    source_assets: Path,
    release_body: str,
    destination: Path,
) -> dict[str, Any]:
    source_manifest = json.loads((source_assets / MANIFEST_NAME).read_text(encoding="utf-8"))
    if not isinstance(source_manifest, dict):
        raise MigrationError(f"{alias.original_version} manifest is not an object")
    validate_manifest(source_manifest, alias)
    source_notes = release_body.rstrip() + "\n"
    validate_source_assets(source_assets, source_manifest, source_notes, alias)

    corrected_release_notes = corrected_notes(release_body, alias)
    corrected_assets = destination / "assets"
    corrected_assets.mkdir(parents=True, exist_ok=True)
    for name in EXPECTED_PAYLOAD_NAMES - {PAGES_NAME}:
        shutil.copyfile(source_assets / name, corrected_assets / name)
    rewrite_pages_archive(
        source_assets / PAGES_NAME,
        corrected_assets / PAGES_NAME,
        alias,
        source_manifest,
    )

    corrected_manifest = copy.deepcopy(source_manifest)
    corrected_manifest["version"] = alias.corrected_version
    corrected_manifest["notes_sha256"] = sha256_bytes(corrected_release_notes.encode())
    for payload in corrected_manifest["payloads"]:
        name = PurePosixPath(payload["path"]).name
        path = corrected_assets / name
        payload["size"] = path.stat().st_size
        payload["sha256"] = sha256_file(path)
    (corrected_assets / MANIFEST_NAME).write_bytes(json_bytes(corrected_manifest))
    (destination / "notes.md").write_text(corrected_release_notes, encoding="utf-8")
    validate_corrected_alias(alias, destination)
    return corrected_manifest


def validate_corrected_alias(alias: ReleaseAlias, destination: Path) -> None:
    assets = destination / "assets"
    manifest = json.loads((assets / MANIFEST_NAME).read_text(encoding="utf-8"))
    expected_identity = {
        "schema_version": 2,
        "artifact_kind": "mprlab.release",
        "version": alias.corrected_version,
        "source_commit": alias.source_commit,
        "release_commit": alias.release_commit,
        "default_branch": DEFAULT_BRANCH,
    }
    for key, expected in expected_identity.items():
        if manifest.get(key) != expected:
            raise MigrationError(
                f"{alias.corrected_version} prepared manifest {key} mismatch"
            )
    notes = (destination / "notes.md").read_bytes()
    if sha256_bytes(notes) != manifest.get("notes_sha256"):
        raise MigrationError(f"{alias.corrected_version} prepared notes hash mismatch")
    actual_names = {path.name for path in assets.iterdir() if path.is_file()}
    if actual_names != EXPECTED_PAYLOAD_NAMES | {MANIFEST_NAME}:
        raise MigrationError(f"{alias.corrected_version} prepared asset inventory mismatch")
    for payload in manifest.get("payloads", []):
        path = assets / PurePosixPath(payload["path"]).name
        if path.stat().st_size != payload.get("size") or sha256_file(path) != payload.get("sha256"):
            raise MigrationError(
                f"{alias.corrected_version} prepared asset mismatch: {path.name}"
            )
    with tarfile.open(assets / PAGES_NAME, "r:gz") as archive:
        marker_members = [
            member
            for member in archive.getmembers()
            if member.name.removeprefix("./") == ".mprlab-release.json"
        ]
        if len(marker_members) != 1:
            raise MigrationError(f"{alias.corrected_version} prepared Pages marker is invalid")
        marker_file = archive.extractfile(marker_members[0])
        if marker_file is None:
            raise MigrationError(f"{alias.corrected_version} prepared Pages marker is unreadable")
        marker = json.loads(marker_file.read())
    expected_marker = {
        "schema_version": 1,
        "release_version": alias.corrected_version,
        "source_commit": alias.source_commit,
        "release_timestamp": manifest["release_timestamp"],
    }
    if marker != expected_marker:
        raise MigrationError(f"{alias.corrected_version} prepared Pages marker mismatch")


def local_tag_commit(repo: Path, version: str) -> str | None:
    result = run(["git", "rev-parse", "--verify", f"refs/tags/{version}^{{commit}}"], cwd=repo, check=False)
    return result.stdout.strip() if result.returncode == 0 else None


def local_tag_type(repo: Path, version: str) -> str | None:
    result = run(["git", "cat-file", "-t", f"refs/tags/{version}"], cwd=repo, check=False)
    return result.stdout.strip() if result.returncode == 0 else None


def remote_tag_commit(repo: Path, version: str) -> str | None:
    result = run(
        ["git", "ls-remote", "--tags", "origin", f"refs/tags/{version}", f"refs/tags/{version}^{{}}"],
        cwd=repo,
    )
    lines = [line.split() for line in result.stdout.splitlines() if line.strip()]
    if not lines:
        return None
    peeled = [object_id for object_id, ref in lines if ref == f"refs/tags/{version}^{{}}"]
    if len(lines) != 2 or len(peeled) != 1:
        raise MigrationError(f"remote tag {version} is not a single annotated tag")
    return peeled[0]


def validate_destination_tag_state(
    alias: ReleaseAlias,
    local_commit: str | None,
    local_type: str | None,
    remote_commit: str | None,
    release_exists: bool,
) -> None:
    if local_commit is not None and (
        local_commit != alias.release_commit or local_type != "tag"
    ):
        raise MigrationError(f"local tag {alias.corrected_version} conflicts with the audited tag")
    if remote_commit is not None and remote_commit != alias.release_commit:
        raise MigrationError(f"remote tag {alias.corrected_version} has a conflicting target")
    if release_exists and remote_commit is None:
        raise MigrationError(
            f"GitHub Release {alias.corrected_version} exists without its audited remote tag"
        )


def github_release(repo: Path, version: str) -> dict[str, Any] | None:
    result = run(
        [
            "gh",
            "release",
            "view",
            version,
            "--repo",
            REPOSITORY,
            "--json",
            "tagName,name,body,publishedAt,isDraft,isPrerelease,targetCommitish,assets",
        ],
        cwd=repo,
        check=False,
    )
    if result.returncode == 0:
        value = json.loads(result.stdout)
        if not isinstance(value, dict):
            raise MigrationError(f"GitHub Release {version} response is invalid")
        return value
    error = (result.stderr + result.stdout).lower()
    if "release not found" in error:
        return None
    raise MigrationError(f"gh failed while reading {version}: {(result.stderr or result.stdout).strip()}")


def validate_release_metadata(
    alias: ReleaseAlias,
    release: dict[str, Any],
    prepared_directory: Path,
) -> dict[str, dict[str, Any]]:
    expected = {
        "tagName": alias.corrected_version,
        "name": f"Release {alias.corrected_version}",
        "body": (prepared_directory / "notes.md").read_text(encoding="utf-8").rstrip(),
        "isDraft": False,
        "isPrerelease": False,
        "targetCommitish": DEFAULT_BRANCH,
    }
    for key, expected_value in expected.items():
        actual_value = release.get(key)
        if key == "body" and isinstance(actual_value, str):
            actual_value = actual_value.rstrip()
        if actual_value != expected_value:
            raise MigrationError(
                f"GitHub Release {alias.corrected_version} {key} mismatch: "
                f"expected {expected_value!r}, found {actual_value!r}"
            )
    if not release.get("publishedAt"):
        raise MigrationError(f"GitHub Release {alias.corrected_version} is not published")
    assets = release.get("assets")
    if not isinstance(assets, list):
        raise MigrationError(f"GitHub Release {alias.corrected_version} assets are invalid")
    indexed: dict[str, dict[str, Any]] = {}
    for asset in assets:
        if not isinstance(asset, dict) or not isinstance(asset.get("name"), str):
            raise MigrationError(f"GitHub Release {alias.corrected_version} has an invalid asset")
        name = asset["name"]
        if name in indexed:
            raise MigrationError(f"GitHub Release {alias.corrected_version} duplicates {name}")
        indexed[name] = asset
    expected_names = EXPECTED_PAYLOAD_NAMES | {MANIFEST_NAME}
    unexpected_names = set(indexed) - expected_names
    if unexpected_names:
        raise MigrationError(
            f"GitHub Release {alias.corrected_version} has unexpected assets: {sorted(unexpected_names)}"
        )
    return indexed


def verify_published_assets(
    repo: Path,
    alias: ReleaseAlias,
    prepared_directory: Path,
    published_assets: dict[str, dict[str, Any]],
) -> set[str]:
    verified: set[str] = set()
    for name, asset in published_assets.items():
        prepared_path = prepared_directory / "assets" / name
        if asset.get("size") != prepared_path.stat().st_size:
            raise MigrationError(
                f"GitHub Release {alias.corrected_version} asset size mismatch: {name}"
            )
        digest = asset.get("digest")
        if digest and digest != f"sha256:{sha256_file(prepared_path)}":
            raise MigrationError(
                f"GitHub Release {alias.corrected_version} asset hash mismatch: {name}"
            )
        if not digest:
            with tempfile.TemporaryDirectory() as download_root:
                run(
                    [
                        "gh",
                        "release",
                        "download",
                        alias.corrected_version,
                        "--repo",
                        REPOSITORY,
                        "--dir",
                        download_root,
                        "--pattern",
                        name,
                    ],
                    cwd=repo,
                )
                if sha256_file(Path(download_root) / name) != sha256_file(prepared_path):
                    raise MigrationError(
                        f"GitHub Release {alias.corrected_version} downloaded asset mismatch: {name}"
                    )
        verified.add(name)
    return verified


def verify_repository(repo: Path) -> None:
    if run(["git", "rev-parse", "--show-toplevel"], cwd=repo).stdout.strip() != str(repo):
        raise MigrationError("migration must run from the Gix repository root")
    if run(["git", "branch", "--show-current"], cwd=repo).stdout.strip() != DEFAULT_BRANCH:
        raise MigrationError(f"migration must run on {DEFAULT_BRANCH}")
    if run(["git", "status", "--porcelain"], cwd=repo).stdout:
        raise MigrationError("migration requires a clean worktree")
    if run(["git", "rev-parse", "origin/master"], cwd=repo).stdout.strip() != run(
        ["git", "rev-parse", "HEAD"], cwd=repo
    ).stdout.strip():
        raise MigrationError("migration requires master aligned with origin/master")
    remote_url = run(["git", "remote", "get-url", "origin"], cwd=repo).stdout.strip()
    if not remote_url.endswith("tyemirov/gix.git"):
        raise MigrationError(f"migration origin is not {REPOSITORY}: {remote_url}")
    for alias in ALIASES:
        if run(["git", "rev-parse", f"{alias.release_commit}^"], cwd=repo).stdout.strip() != alias.source_commit:
            raise MigrationError(
                f"{alias.original_version} release commit does not directly follow its audited source"
            )
        if (
            local_tag_commit(repo, alias.original_version) != alias.release_commit
            or local_tag_type(repo, alias.original_version) != "tag"
        ):
            raise MigrationError(
                f"local {alias.original_version} tag is not the audited annotated tag"
            )
        if remote_tag_commit(repo, alias.original_version) != alias.release_commit:
            raise MigrationError(
                f"remote {alias.original_version} tag is not the audited annotated tag"
            )


def prepare_all(repo: Path, staging: Path) -> None:
    for alias in ALIASES:
        destination = staging / alias.corrected_version
        source_assets = destination / "source-assets"
        source_assets.mkdir(parents=True, exist_ok=True)
        release = github_release(repo, alias.original_version)
        if release is None:
            raise MigrationError(f"source GitHub Release {alias.original_version} is missing")
        expected_release = {
            "tagName": alias.original_version,
            "name": f"Release {alias.original_version}",
            "isDraft": False,
            "isPrerelease": False,
            "targetCommitish": DEFAULT_BRANCH,
        }
        for key, expected in expected_release.items():
            if release.get(key) != expected:
                raise MigrationError(
                    f"source GitHub Release {alias.original_version} {key} mismatch"
                )
        if not release.get("publishedAt"):
            raise MigrationError(f"source GitHub Release {alias.original_version} is not published")
        run(
            [
                "gh",
                "release",
                "download",
                alias.original_version,
                "--repo",
                REPOSITORY,
                "--dir",
                str(source_assets),
                "--pattern",
                "*",
            ],
            cwd=repo,
        )
        prepare_alias(alias, source_assets, str(release.get("body") or ""), destination)
        shutil.rmtree(source_assets)


def preflight_destination(repo: Path, staging: Path) -> dict[str, set[str]]:
    verified_assets: dict[str, set[str]] = {}
    for alias in ALIASES:
        prepared = staging / alias.corrected_version
        validate_corrected_alias(alias, prepared)
        local_commit = local_tag_commit(repo, alias.corrected_version)
        local_type = local_tag_type(repo, alias.corrected_version)
        remote_commit = remote_tag_commit(repo, alias.corrected_version)
        release = github_release(repo, alias.corrected_version)
        validate_destination_tag_state(
            alias,
            local_commit,
            local_type,
            remote_commit,
            release is not None,
        )
        if release is None:
            verified_assets[alias.corrected_version] = set()
            continue
        indexed = validate_release_metadata(alias, release, prepared)
        verified_assets[alias.corrected_version] = verify_published_assets(
            repo, alias, prepared, indexed
        )
    return verified_assets


def publish_all(repo: Path, staging: Path, existing_assets: dict[str, set[str]]) -> None:
    expected_names = EXPECTED_PAYLOAD_NAMES | {MANIFEST_NAME}
    for alias in ALIASES:
        prepared = staging / alias.corrected_version
        if local_tag_commit(repo, alias.corrected_version) is None:
            run(
                [
                    "git",
                    "tag",
                    "-a",
                    alias.corrected_version,
                    "-m",
                    f"Release {alias.corrected_version}",
                    alias.release_commit,
                ],
                cwd=repo,
            )
        if remote_tag_commit(repo, alias.corrected_version) is None:
            run(
                [
                    "git",
                    "push",
                    "origin",
                    f"refs/tags/{alias.corrected_version}:refs/tags/{alias.corrected_version}",
                ],
                cwd=repo,
            )
        release = github_release(repo, alias.corrected_version)
        if release is None:
            run(
                [
                    "gh",
                    "release",
                    "create",
                    alias.corrected_version,
                    "--repo",
                    REPOSITORY,
                    "--verify-tag",
                    "--target",
                    DEFAULT_BRANCH,
                    "--title",
                    f"Release {alias.corrected_version}",
                    "--notes-file",
                    str(prepared / "notes.md"),
                ],
                cwd=repo,
            )
        missing_names = sorted(expected_names - existing_assets[alias.corrected_version])
        if missing_names:
            run(
                [
                    "gh",
                    "release",
                    "upload",
                    alias.corrected_version,
                    "--repo",
                    REPOSITORY,
                    *[str(prepared / "assets" / name) for name in missing_names],
                ],
                cwd=repo,
            )
        refreshed = github_release(repo, alias.corrected_version)
        if refreshed is None:
            raise MigrationError(f"GitHub Release {alias.corrected_version} disappeared")
        indexed = validate_release_metadata(alias, refreshed, prepared)
        verified = verify_published_assets(repo, alias, prepared, indexed)
        if verified != expected_names:
            raise MigrationError(
                f"GitHub Release {alias.corrected_version} asset inventory is incomplete"
            )
        if remote_tag_commit(repo, alias.corrected_version) != alias.release_commit:
            raise MigrationError(f"remote tag {alias.corrected_version} changed during publication")
        print(f"Published and verified {alias.corrected_version}.")


def parse_arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Prepare or publish the bounded Gix historical SemVer correction."
    )
    parser.add_argument(
        "--publish",
        action="store_true",
        help="Publish the verified tags and GitHub Releases; the default is read-only preparation.",
    )
    return parser.parse_args()


def main() -> int:
    arguments = parse_arguments()
    repo = Path(run(["git", "rev-parse", "--show-toplevel"], cwd=Path.cwd()).stdout.strip())
    verify_repository(repo)
    with tempfile.TemporaryDirectory(prefix="gix-semver-migration-") as staging_root:
        staging = Path(staging_root)
        prepare_all(repo, staging)
        existing_assets = preflight_destination(repo, staging)
        if arguments.publish:
            publish_all(repo, staging, existing_assets)
        else:
            print("Historical SemVer correction is fully staged and verified; no tags or releases changed.")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (MigrationError, json.JSONDecodeError, OSError, tarfile.TarError) as error:
        print(f"error: {error}", file=sys.stderr)
        raise SystemExit(1)
