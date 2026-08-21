package tests

import "strings"

type reportedIssueFormatConflictFixture struct {
	Base       string
	Upstream   string
	Stash      string
	Resolved   string
	Candidates map[int]string
}

// newReportedIssueFormatConflictFixture retains the four conflict-region shapes
// from the reported download_your_data stash failure without pinning unrelated
// repository content.
func newReportedIssueFormatConflictFixture() reportedIssueFormatConflictFixture {
	regionOneBase := reportedConflictLines(
		"- The file starts with a title line, for example `# ISSUES`.",
		"- Issues are grouped under level-2 headings.",
		"- Sections are `BugFixes`, `Improvements`, `Maintenance`, `Features`, and `Planning`.",
		"- Optional subheadings may organize a section, but issue IDs must still match the parent section.",
	)
	regionOneUpstream := reportedConflictLines(
		"- The file starts with a title line (for example, `# ISSUES`),",
		"  followed by optional guidance text.",
		"- Issues are grouped under level-2 headings (`## ...`).",
		"- Optional subheadings (`### ...`) may be used within a section for organization (for example, \"Recurring\"), but IDs must still match the parent section. Recurring semantics are canonically represented by the identifier suffix; parsers normalize entries under a `Recurring` subheading to that suffix.",
		"- Sections are:",
		"  - BugFixes",
		"  - Improvements",
		"  - Maintenance",
		"  - Features",
		"  - Planning",
	)
	classificationAndHygiene := reportedConflictLines(
		"## Issue Classification",
		"",
		"Classify each issue by its requested outcome. Priority, urgency, affected code, and title words do not control the section.",
		"",
		"Use this ordered test:",
		"",
		"1. Use `BugFixes` only for an observed and reproducible violation of a current canonical contract.",
		"2. Use `Features` for a new user or operator capability, public interface, resource kind, workflow, or product behavior.",
		"3. Use `Improvements` for a one-time change to an existing capability, architecture, test system, or acceptance boundary.",
		"4. Use `Maintenance` for repeatable upkeep under an unchanged solution contract. The same activity must remain valid for a future run.",
		"5. Use `Planning` for analysis, a decision, or a plan that does not authorize implementation.",
		"",
		"File each reproducible defect from an acceptance or migration issue as a separate BugFix issue. Split mixed outcomes across their correct sections.",
		"",
		"Use priority and blocked state as separate attributes. Correct a misclassified unresolved issue before implementation. Preserve completed issue IDs as historical references.",
		"",
		"## Resolved Issue Hygiene",
		"",
		"Before archival, review each resolved non-recurring issue for durable product,",
		"architecture, operator, security, testing, and skill consequences. Update each",
		"affected source-of-truth document or skill before you move the issue.",
		"",
		"Preserve the complete resolved entry and its identifier in the repository",
		"archive. Keep unresolved, blocked, planning, and recurring issues in the active",
		"tracker. Validate identifiers, dependencies, and duplicate IDs across both",
		"files.",
	)
	regionOneStash := reportedConflictLines(
		"- The file starts with a title line, for example `# ISSUES`.",
		"- Issues are grouped under level-2 headings.",
		"- Sections are `BugFixes`, `Improvements`, `Maintenance`, `Features`, and `Planning`.",
		"- Optional subheadings can organize a section, but issue IDs must still match the parent section.",
		"",
	) + classificationAndHygiene
	regionOneResolved := regionOneUpstream + "\n" + classificationAndHygiene

	regionTwoBase := reportedConflictLines(
		"- `[ ]` means open.",
		"- `[-]` means taken.",
		"- `[!]` means blocked and must include a `Blocked:` body line.",
		"- `[x]` means closed.",
		"- The external ID is required.",
		"- Priority `(P0)` through `(P2)` is optional.",
		"- Dependencies `{ID,ID}` are optional.",
		"- The title is required.",
	)
	regionTwoUpstream := reportedConflictLines(
		"- `[ ]` means open (unresolved), `[-]` means taken (actively being worked, but still unresolved), `[!]` means blocked (unresolved), `[x]` means closed (resolved).",
		"- The external ID is required.",
		"- Priority and dependencies are optional and appear immediately after the ID.",
		"- The title is required.",
		"- Blocked issues (`[!]`) MUST include a short explanation in the body (at minimum one indented line starting with `Blocked:`).",
	)
	regionTwoStash := reportedConflictLines(
		"- `[ ]` means open.",
		"- `[-]` means taken.",
		"- `[!]` means blocked and must include a `Blocked:` body line.",
		"- `[x]` means closed.",
		"- The external ID is necessary.",
		"- Priority `(P0)` through `(P2)` is optional.",
		"- Dependencies `{ID,ID}` are optional.",
		"- The title is necessary.",
		"- Write each new or changed title in ASD-STE100 Simplified Technical English.",
	)
	regionTwoResolved := reportedConflictLines(
		"- `[ ]` means open (unresolved), `[-]` means taken (actively being worked, but still unresolved), `[!]` means blocked (unresolved), `[x]` means closed (resolved).",
		"- The external ID is necessary.",
		"- Priority and dependencies are optional and appear immediately after the ID.",
		"- The title is necessary.",
		"- Blocked issues (`[!]`) MUST include a short explanation in the body (at minimum one indented line starting with `Blocked:`).",
		"- Write each new or changed title in ASD-STE100 Simplified Technical English.",
	)

	regionThreeBase := reportedConflictLines("Additional body lines are indented by two spaces. Structured issue bodies should use plain labels:")
	regionThreeUpstream := reportedConflictLines(
		"  Deliverables:",
		"  Patch the initialization path and document the failure mode.",
	)
	regionThreeStash := reportedConflictLines("Indent additional body lines by two spaces. Structured issue bodies must use plain labels:")
	regionThreeResolved := regionThreeUpstream

	regionFourBase := reportedConflictLines("`Blocked:` is required only for blocked issues and must name the external dependency, missing input, or policy decision preventing progress.")
	regionFourUpstream := reportedConflictLines(
		"  Blocked: waiting on upstream API credentials.",
		"  ```bash",
		"  timeout -k 30s -s SIGKILL 30s make test",
		"  ```",
		"```",
	)
	regionFourStash := reportedConflictLines(
		"`Blocked:` is necessary only for blocked issues. It must identify the dependency, input, or policy decision that prevents progress.",
		"",
		"Write each new or changed body in ASD-STE100. Use `.mprlab/AGENTS.DOCS.md` and `.mprlab/TERMINOLOGY.md`.",
	)
	regionFourResolved := regionFourUpstream + "\n" + regionFourStash

	prefix := reportedConflictLines("# ISSUES.md Format", "", "## Structure", "")
	betweenOneAndTwo := reportedConflictLines("", "## Issue Entries", "", "Rules:", "")
	betweenTwoAndThree := reportedConflictLines("", "## Example", "", "```text", "- [!] [B042] (P0) Fix crash on startup", "")
	betweenThreeAndFour := reportedConflictLines("", "  Validation:", "  Reproduce the startup path with the affected configuration.", "")
	suffix := reportedConflictLines("", "End of fixture.")

	return reportedIssueFormatConflictFixture{
		Base:       prefix + regionOneBase + betweenOneAndTwo + regionTwoBase + betweenTwoAndThree + regionThreeBase + betweenThreeAndFour + regionFourBase + suffix,
		Upstream:   prefix + regionOneUpstream + betweenOneAndTwo + regionTwoUpstream + betweenTwoAndThree + regionThreeUpstream + betweenThreeAndFour + regionFourUpstream + suffix,
		Stash:      prefix + regionOneStash + betweenOneAndTwo + regionTwoStash + betweenTwoAndThree + regionThreeStash + betweenThreeAndFour + regionFourStash + suffix,
		Resolved:   prefix + regionOneResolved + betweenOneAndTwo + regionTwoResolved + betweenTwoAndThree + regionThreeResolved + betweenThreeAndFour + regionFourResolved + suffix,
		Candidates: map[int]string{1: regionOneResolved, 2: regionTwoResolved, 3: regionThreeResolved, 4: regionFourResolved},
	}
}

type reportedLifecycleConflictFixture struct {
	Base   string
	Ours   string
	Theirs string
}

// newReportedLifecycleConflictFixture retains the replacement and deletion
// regions from the reported llm-proxy merge without unrelated file content.
func newReportedLifecycleConflictFixture() reportedLifecycleConflictFixture {
	const functionPrefix = "package tests\n\nfunc "
	const functionBodyPrefix = "(testingInstance *testing.T) {\n\trepositoryRoot := operationalRepositoryRoot(testingInstance)\n"
	const assertionV4 = "\tif schemaVersion, schemaAvailable := resourcesDocument[\"schema_version\"].(int); !schemaAvailable || schemaVersion != 4 {\n\t\ttestingInstance.Fatalf(\"unexpected lifecycle schema version: %#v\", resourcesDocument[\"schema_version\"])\n\t}\n"
	const assertionV5 = "\tif schemaVersion, schemaAvailable := resourcesDocument[\"schema_version\"].(int); !schemaAvailable || schemaVersion != 5 {\n\t\ttestingInstance.Fatalf(\"unexpected lifecycle schema version: %#v\", resourcesDocument[\"schema_version\"])\n\t}\n"
	const functionSuffix = "\tuseRepositoryRoot(repositoryRoot)\n}\n"

	return reportedLifecycleConflictFixture{
		Base:   functionPrefix + "TestOperationalRepositoryOwnsSchemaV4Lifecycle" + functionBodyPrefix + assertionV4 + functionSuffix,
		Ours:   functionPrefix + "TestOperationalRepositoryOwnsSchemaV5Lifecycle" + functionBodyPrefix + assertionV5 + functionSuffix,
		Theirs: functionPrefix + "TestOperationalRepositoryOwnsVersionlessLifecycle" + functionBodyPrefix + functionSuffix,
	}
}

func reportedConflictLines(lines ...string) string {
	return strings.Join(lines, "\n") + "\n"
}
