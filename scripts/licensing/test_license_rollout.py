import json
import pathlib
import tempfile
import unittest

import license_rollout


class LicenseRolloutTest(unittest.TestCase):
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

        self.assertEqual(103, len(manifest.repositories))
        self.assertEqual(97, manifest.apply_count)
        self.assertEqual(6, manifest.review_count)

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
                visibility="PUBLIC",
                license_files={},
            ),
            "tyemirov/new-repository": license_rollout.LiveRepository(
                repository="tyemirov/new-repository",
                default_branch="main",
                visibility="PRIVATE",
                license_files={},
            ),
        }

        errors = license_rollout.compare_inventory(manifest, live_inventory)

        self.assertEqual(
            ["unreviewed source repository: tyemirov/new-repository"],
            errors,
        )

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
                        visibility="PUBLIC",
                        license_files={},
                    )
                }

        plan = license_rollout.build_plan(manifest, Inspector())

        self.assertEqual(1, plan.apply_count)
        self.assertEqual(0, plan.review_count)

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
        with tempfile.TemporaryDirectory() as directory:
            destination = license_rollout.prepare_sparse_clone(
                Runner(),
                record,
                pathlib.Path(directory),
            )

        self.assertEqual(
            pathlib.Path(directory) / "tyemirov--example",
            destination,
        )
        self.assertEqual("git", calls[0][0])
        self.assertIn("--no-checkout", calls[0])
        self.assertIn("/LICENSE", calls[1])
        self.assertIn("/NOTICE", calls[1])
        self.assertIn("/COMMERCIAL_LICENSE.md", calls[1])
        self.assertEqual(
            ["git", "-C", str(destination), "checkout", "main"],
            calls[2],
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
