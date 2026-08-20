package migrate

const (
	safetyReasonOpenPullRequestsConstant = "open pull requests still target source branch"
	safetyReasonBranchProtectedConstant  = "source branch is protected"
	safetyReasonWorkflowMentionsConstant = "workflow files still reference source branch"
	safetyReasonSourceChangesConstant    = "source branch contains changes absent from target branch"
)

// SafetyInputs captures conditions that influence branch deletion safety.
type SafetyInputs struct {
	OpenPullRequestCount int
	BranchProtected      bool
	WorkflowMentions     bool
	SourceChangesMissing bool
}

// SafetyStatus conveys whether it is safe to delete the source branch.
type SafetyStatus struct {
	SafeToDelete    bool
	BlockingReasons []string
}

// SafetyEvaluator evaluates safety inputs to produce a status.
type SafetyEvaluator struct{}

// Evaluate determines whether it is safe to delete the source branch.
func (SafetyEvaluator) Evaluate(inputs SafetyInputs) SafetyStatus {
	blockingReasons := make([]string, 0, 3)
	if inputs.OpenPullRequestCount > 0 {
		blockingReasons = append(blockingReasons, safetyReasonOpenPullRequestsConstant)
	}
	if inputs.BranchProtected {
		blockingReasons = append(blockingReasons, safetyReasonBranchProtectedConstant)
	}
	if inputs.WorkflowMentions {
		blockingReasons = append(blockingReasons, safetyReasonWorkflowMentionsConstant)
	}
	if inputs.SourceChangesMissing {
		blockingReasons = append(blockingReasons, safetyReasonSourceChangesConstant)
	}

	return SafetyStatus{SafeToDelete: len(blockingReasons) == 0, BlockingReasons: blockingReasons}
}
