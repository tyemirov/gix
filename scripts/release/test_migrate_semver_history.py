import importlib.util
import io
import json
import pathlib
import sys
import tarfile
import tempfile
import unittest


MODULE_PATH = pathlib.Path(__file__).with_name("migrate_semver_history.py")
MODULE_SPEC = importlib.util.spec_from_file_location("migrate_semver_history", MODULE_PATH)
assert MODULE_SPEC is not None and MODULE_SPEC.loader is not None
MIGRATION = importlib.util.module_from_spec(MODULE_SPEC)
sys.modules[MODULE_SPEC.name] = MIGRATION
MODULE_SPEC.loader.exec_module(MIGRATION)


class SemverHistoryMigrationTest(unittest.TestCase):
    def test_mapping_is_exact_and_complete(self) -> None:
        self.assertEqual(
            [alias.corrected_version for alias in MIGRATION.ALIASES],
            ["v2.0.0", "v3.0.0", "v4.0.0", "v4.1.0", "v5.0.0"],
        )
        self.assertEqual(
            len({alias.release_commit for alias in MIGRATION.ALIASES}),
            len(MIGRATION.ALIASES),
        )

    def test_prepare_alias_preserves_payloads_and_corrects_version_bound_fields(self) -> None:
        alias = MIGRATION.ALIASES[0]
        with tempfile.TemporaryDirectory() as temporary_root:
            root = pathlib.Path(temporary_root)
            source = root / "source"
            destination = root / "destination"
            source.mkdir()
            release_timestamp = "2026-07-24T12:43:04-07:00"
            release_body = f"## [{alias.original_version}] - 2026-07-24\n\n- Breaking change\n"
            payloads = []
            original_hashes = {}
            for name in MIGRATION.EXPECTED_PAYLOAD_NAMES - {MIGRATION.PAGES_NAME}:
                contents = f"payload:{name}\n".encode()
                path = source / name
                path.write_bytes(contents)
                original_hashes[name] = MIGRATION.sha256_file(path)
                payloads.append(
                    {
                        "path": f"payloads/release-assets/bin/{name}",
                        "size": len(contents),
                        "sha256": original_hashes[name],
                    }
                )
            pages = source / MIGRATION.PAGES_NAME
            marker = {
                "schema_version": 1,
                "release_version": alias.original_version,
                "source_commit": alias.source_commit,
                "release_timestamp": release_timestamp,
            }
            with tarfile.open(pages, "w:gz") as archive:
                index_contents = b"historical docs\n"
                index = tarfile.TarInfo("./index.html")
                index.size = len(index_contents)
                archive.addfile(index, io.BytesIO(index_contents))
                marker_contents = MIGRATION.json_bytes(marker)
                marker_info = tarfile.TarInfo("./.mprlab-release.json")
                marker_info.size = len(marker_contents)
                archive.addfile(marker_info, io.BytesIO(marker_contents))
            payloads.append(
                {
                    "path": "payloads/release-assets/pages.tar.gz",
                    "size": pages.stat().st_size,
                    "sha256": MIGRATION.sha256_file(pages),
                }
            )
            manifest = {
                "schema_version": 2,
                "artifact_kind": "mprlab.release",
                "version": alias.original_version,
                "source_commit": alias.source_commit,
                "release_commit": alias.release_commit,
                "default_branch": MIGRATION.DEFAULT_BRANCH,
                "release_timestamp": release_timestamp,
                "notes_sha256": MIGRATION.sha256_bytes(release_body.encode()),
                "payloads": sorted(payloads, key=lambda payload: payload["path"]),
            }
            (source / MIGRATION.MANIFEST_NAME).write_bytes(MIGRATION.json_bytes(manifest))

            MIGRATION.prepare_alias(alias, source, release_body, destination)

            corrected = json.loads(
                (destination / "assets" / MIGRATION.MANIFEST_NAME).read_text()
            )
            self.assertEqual(corrected["version"], alias.corrected_version)
            self.assertEqual(corrected["source_commit"], alias.source_commit)
            self.assertEqual(corrected["release_commit"], alias.release_commit)
            self.assertEqual(
                corrected["notes_sha256"],
                MIGRATION.sha256_file(destination / "notes.md"),
            )
            self.assertTrue(
                (destination / "notes.md").read_text().startswith(
                    f"## [{alias.corrected_version}] - "
                )
            )
            for name, original_hash in original_hashes.items():
                self.assertEqual(
                    MIGRATION.sha256_file(destination / "assets" / name),
                    original_hash,
                )
            with tarfile.open(destination / "assets" / MIGRATION.PAGES_NAME, "r:gz") as archive:
                marker_file = archive.extractfile("./.mprlab-release.json")
                assert marker_file is not None
                corrected_marker = json.loads(marker_file.read())
            self.assertEqual(corrected_marker["release_version"], alias.corrected_version)
            self.assertEqual(corrected_marker["source_commit"], alias.source_commit)

    def test_corrected_notes_reject_ambiguous_source_version(self) -> None:
        alias = MIGRATION.ALIASES[0]
        with self.assertRaisesRegex(MIGRATION.MigrationError, "ambiguous"):
            MIGRATION.corrected_notes(
                f"## [{alias.original_version}] - 2026-07-24\n\n- mentions {alias.original_version}\n",
                alias,
            )

    def test_destination_preflight_rejects_a_conflicting_tag(self) -> None:
        alias = MIGRATION.ALIASES[0]
        with self.assertRaisesRegex(MIGRATION.MigrationError, "conflicting target"):
            MIGRATION.validate_destination_tag_state(
                alias,
                alias.release_commit,
                "tag",
                "0" * 40,
                False,
            )


if __name__ == "__main__":
    unittest.main()
