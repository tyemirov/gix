# I018 Shared UI Migration

I018 prepares the Gix documentation page for mpr-ui I009.
The application source starts at `a33e01cf2924cf7cda32b77a8cb7a76e5f51a93f`.
The shared candidate is `7c2f9e36453c6081db7641b7efae00c6e271fa39`.
The browser test verifies the JavaScript and CSS SHA-256 values before use.

## Release Unit

Publish the documentation page through the existing Pages artifact workflow.
The footer uses the current `menu` input and preserves all eleven product links.
The page retains literal `@latest` shared JavaScript and CSS URLs.
Its current license content remains available through the footer modal.
The page has no authentication config or theme control.

GitHub confirms `gix.mprlab.com` and the `gh-pages` publication branch.
The existing `make pages-artifact` target packages `docs` and adds the release identity.
The user owns release, publication, and deployment.

## Validation

Hosted CI run `34270238265` passed at `6b42b4b75b576f38a29f57da30a1b114855a895f`.
Only `CHANGELOG.md` differs between that qualified source and the migration base.
That result supplies the initial CI gate.

The existing Chrome harness verifies the real documentation page at 390 and 1280 pixels.
Both regressions first failed with the shared library error for `links-collection`.
Both then passed against the migrated footer.
They verify eleven menu links, horizontal bounds, keyboard dismissal, focus, license content, and page reload.
The tests control shared asset and telemetry responses at the external browser boundary.
The shared assets retain their literal `@latest` request URLs.

Run `make test-docs-browser` for these focused checks.
Run `make ci` for all required local checks.
Set `GIX_TEST_BROWSER` when Chrome is outside the existing discovery locations.
The hosted workflow supplies Chrome and includes documentation page changes in its path filters.
Final local CI passed formatting, Go vet, staticcheck, ineffassign, application tests, 16 licensing tests, and the CLI integration suite.
Hosted CI passed at `c9334e8ddd898aba31443ed6c5507fa8546f9d67` on the second attempt.
The [hosted run](https://github.com/tyemirov/gix/actions/runs/34303302514) records both attempts.
The first attempt timed out in the unchanged workspace-startup browser test.
Three focused local repetitions passed. The hosted rerun then passed the complete suite.
This result does not establish a source fix for that intermittent timeout.

## Publication And Acceptance

The [public asset record](mpr-ui/public-assets-2026-09-09.json) records four observations from one network location.
All four requests returned HTTP 200.
The shared assets permit a seven-day browser cache and a twelve-hour shared cache.
The observations do not establish the coordinated cache transition.

1. Complete application preparation and final candidate qualification under mpr-ui I009.
2. Prepare the documentation maintenance artifact for the coordinated interruption.
3. Let the user select the production window and run the publication sequence.
4. Verify the public page, release identity, and both shared asset digests after the cache transition.
5. Verify all product links, keyboard controls, and license content at mobile and desktop widths.
6. Keep I018 blocked until shared publication, cache transition, and public acceptance pass.
