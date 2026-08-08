package syncflow

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConflictPlanDeterministicallyCombinesB089ShapedInsertions(t *testing.T) {
	base := "- [ ] [B089] Base provider contract.\n"
	target := "- [x] [B089] Target provider contract.\n- [x] [I042] Target branch addition.\n"
	incoming := base + "- [ ] [I037] Stashed operator addition.\n"
	diff3Content := "stable prefix\n" +
		mergeConflictPlanMarker("<", mergeConflictPlanTargetLabel) + "\n" +
		target +
		mergeConflictPlanMarker("|", mergeConflictPlanBaseLabel) + "\n" +
		base +
		strings.Repeat("=", mergeConflictPlanMarkerSize) + "\n" +
		incoming +
		mergeConflictPlanMarker(">", mergeConflictPlanIncomingLabel) + "\n" +
		"stable suffix\n"

	plan, planErr := newMergeConflictPlan(".mprlab/ISSUES.md", diff3Content)
	require.NoError(t, planErr)
	require.Len(t, plan.Hunks, 1)

	resolution, deterministic := resolveDeterministicConflictHunk(plan.Hunks[0])
	require.True(t, deterministic)
	require.Contains(t, resolution, "- [x] [B089] Target provider contract.")
	require.Contains(t, resolution, "- [x] [I042] Target branch addition.")
	require.Contains(t, resolution, "- [ ] [I037] Stashed operator addition.")
	require.NotContains(t, resolution, "- [ ] [B089] Base provider contract.")

	rendered, renderErr := plan.render(map[string]string{plan.Hunks[0].ID: resolution})
	require.NoError(t, renderErr)
	require.Equal(t, "stable prefix\n"+resolution+"stable suffix\n", rendered)
}

func TestConflictPlanLeavesGenericSameAnchorInsertionsForSemanticResolution(t *testing.T) {
	hunk := mergeConflictHunk{
		Path:     "config.yml",
		Target:   "setting: target\n",
		Incoming: "setting: incoming\n",
	}

	resolution, deterministic := resolveDeterministicConflictHunk(hunk)
	require.False(t, deterministic)
	require.Empty(t, resolution)
}

func TestStructuredStashHunkResolutionRequiresBothIntents(t *testing.T) {
	hunk := mergeConflictHunk{
		ID:       "hunk-id",
		Base:     "base\n",
		Target:   "target\n",
		Incoming: "stashed\n",
	}
	options := mergeConflictResolutionOptions{Mode: mergeConflictResolutionModeStash}

	validContent, validErr := validateMergeConflictHunkResponse(options, hunk, mergeConflictHunkResponse{
		HunkID:  "hunk-id",
		Content: "target\nstashed\n",
	})
	require.NoError(t, validErr)
	require.Equal(t, "target\nstashed\n", validContent)

	_, lossyErr := validateMergeConflictHunkResponse(options, hunk, mergeConflictHunkResponse{
		HunkID:  "hunk-id",
		Content: "target\n",
	})
	require.EqualError(t, lossyErr, "hunk hunk-id does not preserve target and stashed intent")
}

func TestMarkerFreeStashResolutionPreservesStashedDeletion(t *testing.T) {
	conflictFile := mergeConflictFile{
		Path:   "obsolete.txt",
		Base:   mergeConflictStage{Mode: "100644", Present: true, Content: "base\n"},
		Target: mergeConflictStage{Mode: "100644", Present: true, Content: "target\n"},
	}

	resolution, resolutionErr := deterministicMarkerFreeConflictResolution(
		mergeConflictResolutionOptions{Mode: mergeConflictResolutionModeStash},
		conflictFile,
	)
	require.NoError(t, resolutionErr)
	require.True(t, resolution.Delete)
	require.Equal(t, "obsolete.txt", resolution.Path)
}

func TestNormalizeMergeConflictHunkTerminatorUsesStageConvention(t *testing.T) {
	hunk := mergeConflictHunk{Target: "target\r\n", Incoming: "incoming\r\n"}
	require.Equal(t, "resolved\r\n", normalizeMergeConflictHunkTerminator("resolved", hunk))
	require.Equal(t, "", normalizeMergeConflictHunkTerminator("", hunk))
}

func TestConflictFileValidationRejectsBinaryAndUnsupportedObjectModes(t *testing.T) {
	binaryConflict := mergeConflictFile{
		Path:   "fixture.bin",
		Base:   mergeConflictStage{Mode: "100644", Present: true, Content: "base\x00value"},
		Target: mergeConflictStage{Mode: "100644", Present: true, Content: "target\x00value"},
	}
	require.EqualError(t, validateMergeConflictFile(binaryConflict), "base stage for fixture.bin is binary and cannot be resolved as text")

	symlinkConflict := mergeConflictFile{
		Path:   "fixture-link",
		Base:   mergeConflictStage{Mode: "120000", Present: true, Content: "base-target"},
		Target: mergeConflictStage{Mode: "120000", Present: true, Content: "target"},
	}
	require.EqualError(t, validateMergeConflictFile(symlinkConflict), `base stage for fixture-link: unsupported Git object mode "120000"`)
}
