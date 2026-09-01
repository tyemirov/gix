import json
import pathlib
import subprocess
import tempfile
import unittest

import license_rollout


class LicenseRolloutTest(unittest.TestCase):
    def run_git(self, *arguments, cwd=None, check=True):
        return subprocess.run(
            ["git", *map(str, arguments)],
            cwd=cwd,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=check,
            timeout=30,
        )

    def write_manifest(self, repositories):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        path = pathlib.Path(directory.name) / "fleet.json"
        path.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "snapshot_date": "2026-07-28",
                    "repositories": repositories,
                }
            ),
            encoding="utf-8",
        )
        return path

    def make_apply_record(self):
        return license_rollout.RepositoryRecord(
            repository="tyemirov/example",
            profile="polyform-noncommercial",
            disposition="apply",
            default_branch="main",
            visibility="PUBLIC",
            license_holder="Vadym Temirov",
            license_files={
                "LICENSE": "a" * 40,
                "LICENSE.txt": "b" * 40,
            },
            reason="owner-policy",
        )

    def existing_draft_fixture(self):
        record = self.make_apply_record()
        base_commit = "1" * 40
        head_commit = "2" * 40
        expected_blobs = license_rollout.expected_license_blobs(record)
        inspected = license_rollout.LiveRepository(
            repository=record.repository,
            default_branch=record.default_branch,
            commit_sha=base_commit,
            visibility=record.visibility,
            license_files=record.license_files,
        )
        pull_requests = [
            {
                "number": 42,
                "url": "https://github.com/tyemirov/example/pull/42",
                "baseRefName": record.default_branch,
                "baseRefOid": base_commit,
                "headRefName": license_rollout.automation_branch(record),
                "headRefOid": head_commit,
                "isCrossRepository": False,
                "isDraft": True,
                "changedFiles": 5,
                "state": "OPEN",
            }
        ]
        comparison = {
            "status": "ahead",
            "ahead_by": 1,
            "behind_by": 0,
            "total_commits": 1,
            "base_commit": {"sha": base_commit},
            "merge_base_commit": {"sha": base_commit},
            "commits": [
                {
                    "sha": head_commit,
                    "parents": [{"sha": base_commit}],
                    "commit": {
                        "message": (
                            "chore: apply polyform-noncommercial license"
                        )
                    },
                }
            ],
            "files": [
                {
                    "filename": "LICENSE",
                    "status": "modified",
                    "sha": expected_blobs["LICENSE"],
                },
                {
                    "filename": "LICENSE.txt",
                    "status": "removed",
                    "sha": record.license_files["LICENSE.txt"],
                },
                {
                    "filename": "NOTICE",
                    "status": "added",
                    "sha": expected_blobs["NOTICE"],
                },
                {
                    "filename": "COMMERCIAL_LICENSE.md",
                    "status": "added",
                    "sha": expected_blobs["COMMERCIAL_LICENSE.md"],
                },
                {
                    "filename": "CONTRIBUTOR_LICENSE.md",
                    "status": "added",
                    "sha": expected_blobs["CONTRIBUTOR_LICENSE.md"],
                },
            ],
        }
        head_contents = [
            {
                "type": "file",
                "name": name,
                "sha": blob_sha,
            }
            for name, blob_sha in expected_blobs.items()
        ]

        class Runner:
            def __init__(self):
                self.calls = []

            def run_json(self, arguments):
                normalized = [str(argument) for argument in arguments]
                self.calls.append(normalized)
                if normalized[:3] == ["gh", "pr", "list"]:
                    return pull_requests
                if normalized[:3] == ["gh", "pr", "view"]:
                    return dict(pull_requests[0])
                endpoint = normalized[4]
                if "/compare/" in endpoint:
                    return comparison
                if endpoint.endswith("/contents"):
                    return head_contents
                raise AssertionError(f"unexpected command: {normalized}")

        return (
            Runner(),
            record,
            inspected,
            pull_requests,
            comparison,
            head_contents,
            expected_blobs,
        )

    def test_load_manifest_accepts_current_owner_profile_contract(self):
        path = self.write_manifest(
            [
                {
                    "repository": "tyemirov/example",
                    "profile": "polyform-noncommercial",
                    "disposition": "apply",
                    "default_branch": "main",
                    "visibility": "PUBLIC",
                    "license_holder": "Vadym Temirov",
                    "license_files": {"LICENSE": "abc123"},
                    "reason": "owner-policy",
                },
                {
                    "repository": "MarcoPoloResearchLab/example",
                    "profile": "mprlab-proprietary",
                    "disposition": "review",
                    "default_branch": "master",
                    "visibility": "PRIVATE",
                    "license_holder": "Marco Polo Research Lab LLC",
                    "license_files": {},
                    "reason": "confirm contribution rights",
                },
            ]
        )

        manifest = license_rollout.load_manifest(path)

        self.assertEqual(2, len(manifest.repositories))
        self.assertEqual(1, manifest.apply_count)
        self.assertEqual(1, manifest.review_count)

    def test_checked_in_manifest_has_reviewed_fleet_counts(self):
        manifest = license_rollout.load_manifest(
            license_rollout.DEFAULT_MANIFEST_PATH
        )

        self.assertEqual(108, len(manifest.repositories))
        self.assertEqual(101, manifest.apply_count)
        self.assertEqual(7, manifest.review_count)

    def test_load_manifest_rejects_cross_owner_profile(self):
        path = self.write_manifest(
            [
                {
                    "repository": "MarcoPoloResearchLab/example",
                    "profile": "polyform-noncommercial",
                    "disposition": "apply",
                    "default_branch": "main",
                    "visibility": "PUBLIC",
                    "license_holder": "Marco Polo Research Lab LLC",
                    "license_files": {},
                    "reason": "owner-policy",
                }
            ]
        )

        with self.assertRaisesRegex(
            license_rollout.RolloutError,
            "profile polyform-noncommercial requires owner tyemirov",
        ):
            license_rollout.load_manifest(path)

    def test_compare_inventory_rejects_license_blob_drift(self):
        path = self.write_manifest(
            [
                {
                    "repository": "tyemirov/example",
                    "profile": "polyform-noncommercial",
                    "disposition": "apply",
                    "default_branch": "main",
                    "visibility": "PUBLIC",
                    "license_holder": "Vadym Temirov",
                    "license_files": {"LICENSE": "expected"},
                    "reason": "owner-policy",
                }
            ]
        )
        manifest = license_rollout.load_manifest(path)
        live_inventory = {
            "tyemirov/example": license_rollout.LiveRepository(
                repository="tyemirov/example",
                default_branch="main",
                commit_sha="1" * 40,
                visibility="PUBLIC",
                license_files={"LICENSE": "changed"},
            )
        }

        errors = license_rollout.compare_inventory(manifest, live_inventory)

        self.assertEqual(
            [
                "tyemirov/example: license files changed "
                "(expected LICENSE@expected; found LICENSE@changed)"
            ],
            errors,
        )

    def test_compare_inventory_rejects_added_source_repository(self):
        path = self.write_manifest(
            [
                {
                    "repository": "tyemirov/example",
                    "profile": "polyform-noncommercial",
                    "disposition": "apply",
                    "default_branch": "main",
                    "visibility": "PUBLIC",
                    "license_holder": "Vadym Temirov",
                    "license_files": {},
                    "reason": "owner-policy",
                }
            ]
        )
        manifest = license_rollout.load_manifest(path)
        live_inventory = {
            "tyemirov/example": license_rollout.LiveRepository(
                repository="tyemirov/example",
                default_branch="main",
                commit_sha="1" * 40,
                visibility="PUBLIC",
                license_files={},
            ),
            "tyemirov/new-repository": license_rollout.LiveRepository(
                repository="tyemirov/new-repository",
                default_branch="main",
                commit_sha="2" * 40,
                visibility="PRIVATE",
                license_files={},
            ),
        }

        errors = license_rollout.compare_inventory(manifest, live_inventory)

        self.assertEqual(
            ["unreviewed source repository: tyemirov/new-repository"],
            errors,
        )

    def test_inspector_reads_license_files_from_resolved_commit(self):
        commit_sha = "1" * 40
        calls = []

        class Runner:
            def run_json(self, arguments):
                normalized = [str(argument) for argument in arguments]
                calls.append(normalized)
                if normalized[:3] == ["gh", "repo", "list"]:
                    if "tyemirov" in normalized:
                        return [
                            {
                                "nameWithOwner": "tyemirov/example",
                                "defaultBranchRef": {"name": "feature/current"},
                                "visibility": "PUBLIC",
                                "isFork": False,
                                "isArchived": False,
                            }
                        ]
                    return []
                if normalized[4].endswith(
                    "commits/feature%2Fcurrent"
                ):
                    return {"sha": commit_sha}
                if normalized[4].endswith("contents"):
                    return [
                        {
                            "type": "file",
                            "name": "LICENSE",
                            "sha": "license-blob",
                        }
                    ]
                raise AssertionError(f"unexpected command: {normalized}")

        inventory = license_rollout.GitHubInspector(
            Runner(),
            parallelism=1,
        ).inspect()

        inspected = inventory["tyemirov/example"]
        self.assertEqual(commit_sha, inspected.commit_sha)
        contents_call = next(
            call for call in calls if call[4].endswith("contents")
        )
        self.assertIn(f"ref={commit_sha}", contents_call)
        self.assertNotIn("ref=feature/current", contents_call)

    def test_plan_does_not_prepare_clones(self):
        path = self.write_manifest(
            [
                {
                    "repository": "tyemirov/example",
                    "profile": "polyform-noncommercial",
                    "disposition": "apply",
                    "default_branch": "main",
                    "visibility": "PUBLIC",
                    "license_holder": "Vadym Temirov",
                    "license_files": {},
                    "reason": "owner-policy",
                }
            ]
        )
        manifest = license_rollout.load_manifest(path)

        class Inspector:
            def inspect(self):
                return {
                    "tyemirov/example": license_rollout.LiveRepository(
                        repository="tyemirov/example",
                        default_branch="main",
                        commit_sha="1" * 40,
                        visibility="PUBLIC",
                        license_files={},
                    )
                }

        plan = license_rollout.build_plan(manifest, Inspector())

        self.assertEqual(1, plan.apply_count)
        self.assertEqual(0, plan.review_count)

    def test_existing_draft_matches_reviewed_base_head_and_license_diff(self):
        runner, record, inspected, _, _, _, _ = (
            self.existing_draft_fixture()
        )

        state = license_rollout.inspect_pull_request_state(
            runner,
            record,
            inspected,
        )

        self.assertEqual(
            "https://github.com/tyemirov/example/pull/42",
            state.existing_url,
        )
        pull_request_fields = runner.calls[0][
            runner.calls[0].index("--json") + 1
        ]
        self.assertIn("baseRefName", pull_request_fields)
        self.assertIn("baseRefOid", pull_request_fields)
        self.assertIn("headRefOid", pull_request_fields)
        self.assertIn("changedFiles", pull_request_fields)
        self.assertIn("state", pull_request_fields)
        self.assertEqual(
            ["gh", "pr", "view"],
            runner.calls[-1][:3],
        )

    def test_existing_draft_rejects_wrong_base_name_or_commit(self):
        scenarios = (
            ("baseRefName", "develop", "base branch changed"),
            ("baseRefOid", "3" * 40, "base commit changed"),
        )
        for field, value, message in scenarios:
            with self.subTest(field=field):
                (
                    runner,
                    record,
                    inspected,
                    pull_requests,
                    _,
                    _,
                    _,
                ) = self.existing_draft_fixture()
                pull_requests[0][field] = value

                with self.assertRaisesRegex(
                    license_rollout.RolloutError,
                    message,
                ):
                    license_rollout.inspect_pull_request_state(
                        runner,
                        record,
                        inspected,
                    )

    def test_existing_draft_rejects_noncanonical_head_history(self):
        (
            runner,
            record,
            inspected,
            _,
            comparison,
            _,
            _,
        ) = self.existing_draft_fixture()
        comparison["ahead_by"] = 2
        comparison["total_commits"] = 2

        with self.assertRaisesRegex(
            license_rollout.RolloutError,
            "head is not exactly one reviewed commit ahead",
        ):
            license_rollout.inspect_pull_request_state(
                runner,
                record,
                inspected,
            )

    def test_existing_draft_rejects_snapshot_changes_during_validation(self):
        scenarios = (
            ("headRefOid", "3" * 40),
            ("state", "CLOSED"),
        )
        for field, value in scenarios:
            with self.subTest(field=field):
                (
                    runner,
                    record,
                    inspected,
                    pull_requests,
                    _,
                    _,
                    _,
                ) = self.existing_draft_fixture()
                original_run_json = runner.run_json

                def run_json(arguments):
                    normalized = [str(argument) for argument in arguments]
                    if normalized[:3] == ["gh", "pr", "view"]:
                        moved_pull_request = dict(pull_requests[0])
                        moved_pull_request[field] = value
                        runner.calls.append(normalized)
                        return moved_pull_request
                    return original_run_json(arguments)

                runner.run_json = run_json

                with self.assertRaisesRegex(
                    license_rollout.RolloutError,
                    f"changed during validation \\({field}\\)",
                ):
                    license_rollout.inspect_pull_request_state(
                        runner,
                        record,
                        inspected,
                    )

    def test_existing_draft_rejects_extra_or_modified_files(self):
        scenarios = ("extra file", "modified license")
        for scenario in scenarios:
            with self.subTest(scenario=scenario):
                (
                    runner,
                    record,
                    inspected,
                    pull_requests,
                    comparison,
                    head_contents,
                    _,
                ) = self.existing_draft_fixture()
                if scenario == "extra file":
                    comparison["files"].append(
                        {
                            "filename": "README.md",
                            "status": "modified",
                            "sha": "4" * 40,
                        }
                    )
                    pull_requests[0]["changedFiles"] = 6
                else:
                    comparison["files"][0]["sha"] = "5" * 40
                    head_contents[0]["sha"] = "5" * 40

                with self.assertRaisesRegex(
                    license_rollout.RolloutError,
                    "diff does not match the reviewed license rollout",
                ):
                    license_rollout.inspect_pull_request_state(
                        runner,
                        record,
                        inspected,
                    )

    def test_sparse_clone_is_limited_to_license_contract_paths(self):
        calls = []

        class Runner:
            def run(self, arguments, **_options):
                calls.append([str(argument) for argument in arguments])

        record = license_rollout.RepositoryRecord(
            repository="tyemirov/example",
            profile="polyform-noncommercial",
            disposition="apply",
            default_branch="main",
            visibility="PUBLIC",
            license_holder="Vadym Temirov",
            license_files={},
            reason="owner-policy",
        )
        inspected = license_rollout.LiveRepository(
            repository=record.repository,
            default_branch=record.default_branch,
            commit_sha="1" * 40,
            visibility=record.visibility,
            license_files=record.license_files,
        )
        with tempfile.TemporaryDirectory() as directory:
            destination = license_rollout.prepare_sparse_clone(
                Runner(),
                record,
                inspected,
                pathlib.Path(directory),
            )

        self.assertEqual(
            pathlib.Path(directory) / "tyemirov--example",
            destination,
        )
        self.assertEqual("git", calls[0][0])
        self.assertIn("--no-checkout", calls[0])
        self.assertIn("/LICENSE", calls[2])
        self.assertIn("/NOTICE", calls[2])
        self.assertIn("/COMMERCIAL_LICENSE.md", calls[2])
        self.assertIn("/CONTRIBUTOR_LICENSE.md", calls[2])
        self.assertEqual(
            [
                "git",
                "-C",
                str(destination),
                "checkout",
                "-B",
                "main",
                inspected.commit_sha,
            ],
            calls[3],
        )
        self.assertEqual(
            [
                "git",
                "-C",
                str(destination),
                "branch",
                "--unset-upstream",
                "main",
            ],
            calls[4],
        )

    def test_sparse_clone_stays_on_inspected_commit_after_default_branch_moves(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            remote = root / "remote.git"
            source = root / "source"
            clones = root / "clones"
            remote_url = remote.as_uri()
            self.run_git("init", "--bare", remote)
            self.run_git("init", "--initial-branch=main", source)
            self.run_git("config", "user.name", "License Test", cwd=source)
            self.run_git(
                "config",
                "user.email",
                "license-test@example.invalid",
                cwd=source,
            )
            (source / "LICENSE").write_text("reviewed\n", encoding="utf-8")
            self.run_git("add", "LICENSE", cwd=source)
            self.run_git("commit", "-m", "reviewed license", cwd=source)
            self.run_git("remote", "add", "origin", remote_url, cwd=source)
            self.run_git("push", "--set-upstream", "origin", "main", cwd=source)
            inspected_commit = self.run_git(
                "rev-parse",
                "HEAD",
                cwd=source,
            ).stdout.strip()

            (source / "LICENSE").write_text("unreviewed\n", encoding="utf-8")
            self.run_git("commit", "-am", "move default branch", cwd=source)
            self.run_git("push", "origin", "main", cwd=source)
            moved_commit = self.run_git(
                "rev-parse",
                "HEAD",
                cwd=source,
            ).stdout.strip()
            self.assertNotEqual(inspected_commit, moved_commit)
            clones.mkdir()

            class LocalRemoteRunner:
                def run(_self, arguments, **_options):
                    normalized = [
                        remote_url
                        if str(argument)
                        == "git@github.com:tyemirov/example.git"
                        else str(argument)
                        for argument in arguments
                    ]
                    return subprocess.run(
                        normalized,
                        text=True,
                        stdout=subprocess.PIPE,
                        stderr=subprocess.PIPE,
                        check=True,
                        timeout=30,
                    )

            record = license_rollout.RepositoryRecord(
                repository="tyemirov/example",
                profile="polyform-noncommercial",
                disposition="apply",
                default_branch="main",
                visibility="PUBLIC",
                license_holder="Vadym Temirov",
                license_files={"LICENSE": "reviewed-blob"},
                reason="owner-policy",
            )
            inspected = license_rollout.LiveRepository(
                repository=record.repository,
                default_branch=record.default_branch,
                commit_sha=inspected_commit,
                visibility=record.visibility,
                license_files=record.license_files,
            )

            destination = license_rollout.prepare_sparse_clone(
                LocalRemoteRunner(),
                record,
                inspected,
                clones,
            )

            self.assertEqual(
                inspected_commit,
                self.run_git("rev-parse", "HEAD", cwd=destination).stdout.strip(),
            )
            self.assertEqual(
                "reviewed\n",
                self.run_git("show", "HEAD:LICENSE", cwd=destination).stdout,
            )
            upstream = self.run_git(
                "rev-parse",
                "--abbrev-ref",
                "--symbolic-full-name",
                "@{upstream}",
                cwd=destination,
                check=False,
            )
            self.assertNotEqual(0, upstream.returncode)
            self.run_git("fetch", "--prune", "origin", cwd=destination)
            pull = self.run_git(
                "pull",
                "--ff-only",
                cwd=destination,
                check=False,
            )
            self.assertNotEqual(0, pull.returncode)
            self.assertEqual(
                inspected_commit,
                self.run_git("rev-parse", "HEAD", cwd=destination).stdout.strip(),
            )

    def test_gix_workflow_uses_canonical_profile_and_holder_variables(self):
        calls = []

        class Runner:
            def run(self, arguments, **options):
                calls.append(
                    ([str(argument) for argument in arguments], options)
                )

        record = license_rollout.RepositoryRecord(
            repository="tyemirov/example",
            profile="polyform-noncommercial",
            disposition="apply",
            default_branch="main",
            visibility="PUBLIC",
            license_holder="Vadym Temirov",
            license_files={},
            reason="owner-policy",
        )
        license_rollout.run_gix_workflow(
            Runner(),
            gix_path=pathlib.Path("/tools/gix"),
            workflow_path=pathlib.Path("/configs/license-rollout.yaml"),
            profile=record.profile,
            license_holder=record.license_holder,
            records=(record,),
            clone_paths={record.repository: pathlib.Path("/tmp/isolated")},
            workflow_workers=4,
        )

        arguments, options = calls[0]
        self.assertEqual(
            ["/tools/gix", "workflow", "/configs/license-rollout.yaml"],
            arguments[:3],
        )
        self.assertIn("license_template=polyform-noncommercial", arguments)
        self.assertIn("license_licensor=Vadym Temirov", arguments)
        self.assertIn("license_profile=polyform-noncommercial", arguments)
        self.assertNotIn("template=polyform-noncommercial", arguments)
        self.assertTrue(options["stream"])


if __name__ == "__main__":
    unittest.main()
