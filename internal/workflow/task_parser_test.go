package workflow

import "testing"

func TestParseTaskFileModeRecognizesLegacyAliases(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected TaskFileMode
	}{
		{name: "OverwriteDefault", input: "overwrite", expected: TaskFileModeOverwrite},
		{name: "SkipIfExists", input: "skip-if-exists", expected: TaskFileModeSkipIfExists},
		{name: "AppendIfMissing", input: "append-if-missing", expected: TaskFileModeAppendIfMissing},
		{name: "UnknownFallsBackToOverwrite", input: "trim-only", expected: TaskFileModeOverwrite},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := ParseTaskFileMode(testCase.input)
			if result != testCase.expected {
				t.Fatalf("mode mismatch: expected %s, received %s", testCase.expected, result)
			}
		})
	}
}
