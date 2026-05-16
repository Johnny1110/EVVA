package fs

import "context"

// Decision is the outcome of an approval prompt. Approved gates the
// write; Feedback is optional free-text the user supplied when
// declining — the tool folds it into the error returned to the model
// so the agent can re-plan against the user's actual intent rather
// than a bare "user declined".
//
// An approver that has no feedback channel (e.g. a legacy auto-yes
// implementation) just returns Decision{Approved: true} with empty
// Feedback. Empty Feedback on a declined decision is a clean cancel.
type Decision struct {
	Approved bool
	Feedback string
}

// Approver gates filesystem mutations behind user confirmation. The fs
// tools (write_file, edit_file) compute the proposed diff and call
// Approve before committing. A nil Approver on the tool means "no gate"
// — every mutation is auto-approved. That keeps test paths and headless
// flows that explicitly opt out simple.
//
// Approve blocks until the user decides or ctx is cancelled. Returning
// Decision{Approved: false} is a clean user decline — the tool surfaces
// a friendly "cancelled" error to the model so it can plan around the
// rejection. When Decision.Feedback is non-empty the model will see
// the user's redirection text in the error content.
//
// A non-nil error means the approver itself broke (no input stream,
// terminal closed mid-prompt, etc.) and the mutation aborts.
type Approver interface {
	Approve(ctx context.Context, diff *FileDiff) (Decision, error)
}
