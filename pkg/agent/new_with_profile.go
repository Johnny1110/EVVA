package agent

import (
	"fmt"

	agent_impl "github.com/johnny1110/evva/internal/agent"
	"github.com/johnny1110/evva/internal/permission"
	"github.com/johnny1110/evva/internal/question"
)

// NewWithProfile is the flexible public constructor a downstream app
// uses to build an agent against its own profile and option set. Unlike
// New (which loads the bundled Main persona, memdir, and a non-interactive
// permission stub) this constructor wires only what the caller supplies
// — no skill catalog, no memory snapshot, no agent registry by default.
//
// A non-interactive permission broker + question broker are installed
// internally so async approval / question requests don't park the
// caller forever; pass agent_impl.WithPermissionBroker /
// WithQuestionBroker via the variadic opts list to override.
//
// Example:
//
//	prof, _ := agent.NewProfile("custom", "you are helpful",
//	    []tools.ToolName{tools.READ_FILE, tools.BASH},
//	    "anthropic", constant.CLAUDE_SONNET_4_6,
//	    agent.ProfileOptions{})
//
//	ag, _ := agent.NewWithProfile(prof,
//	    agent.WithConfig(cfg),
//	    agent.WithSink(mySink),
//	    agent.WithMaxIterations(20),
//	)
//	resp, _ := ag.Run(ctx, "...")
func NewWithProfile(profile Profile, opts ...Option) (Agent, error) {
	permBroker := permission.NewBroker()
	permission.SetOnRequest(permBroker, func(req permission.ApprovalRequest) {
		_ = permBroker.Respond(req.ID, permission.Decision{
			Behavior: permission.BehaviorDeny,
			Reason:   "no interactive approval surface; install agent.WithPermissionBroker for interactivity",
		})
	})

	qBroker := question.NewBroker()
	question.SetOnRequest(qBroker, func(req question.Request) {
		_ = qBroker.Respond(req.ID, question.Response{})
	})

	defaults := []Option{
		agent_impl.WithPermissionBroker(permBroker),
		agent_impl.WithQuestionBroker(qBroker),
	}
	all := append(defaults, opts...)

	inner, err := agent_impl.New(nil, profile, all...)
	if err != nil {
		return nil, fmt.Errorf("agent: %w", err)
	}
	return &agentAdapter{inner: inner}, nil
}
