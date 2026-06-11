package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/johnny1110/evva/internal/swarm"
	"github.com/johnny1110/evva/internal/swarm/agentdef"
	pubtools "github.com/johnny1110/evva/pkg/tools"
)

// newSkillPublish builds the Leader's skill_publish tool (RP-26 Part B): write
// a skill into the space-shared dir and reload every member, so a procedure the
// leader codified once becomes team-wide, compaction-proof know-how. This is
// the ONE deliberate opening in the RP-10 "agents load skills, never author
// them" discipline, and it is shaped to stay narrow: the tool can only reach
// the shared dir (there is no member parameter — a leader cannot write into a
// member's private skills/ and quietly change its persona), the tool_use event
// self-audits the publish into the event log (RP-17), and the User remains the
// final arbiter via the web's shared-skills list/delete.
func newSkillPublish(mc swarm.MemberContext) pubtools.Tool {
	return &swarmTool{
		name: toolSkillPublish,
		desc: "Publish a skill to the whole team: writes <name>/SKILL.md into the space-shared skills directory " +
			"and reloads every member, so each picks it up at its next run boundary. Use this to institutionalize " +
			"a procedure — a report format, a checklist, a how-to — instead of re-explaining it by message " +
			"(messages die with context compaction; skills persist). Keep the body a self-contained instruction " +
			"sheet, and make `description` say when to use it — that line is what teammates see in their skill list. " +
			"A member's own same-named skill still wins over the shared copy. To update a skill you published " +
			"before, set `overwrite: true`. The operator can review and delete shared skills from the web.",
		schema: `{"type":"object","properties":{` +
			`"name":{"type":"string","description":"Skill name (becomes the folder name): lowercase-kebab-case, no slashes."},` +
			`"description":{"type":"string","description":"One line: what the skill does and when to use it. Shown in every member's skill list."},` +
			`"body":{"type":"string","description":"The skill instructions (markdown). Self-contained — a teammate loads ONLY this text."},` +
			`"overwrite":{"type":"boolean","description":"Replace an existing shared skill of the same name (publish a new version). Default false."}` +
			`},"required":["name","description","body"]}`,
		exec: func(_ context.Context, input json.RawMessage) (pubtools.Result, error) {
			var in struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Body        string `json:"body"`
				Overwrite   bool   `json:"overwrite"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return errf("skill_publish: invalid input: %v", err), nil
			}
			name := strings.TrimSpace(in.Name)
			if name == "" {
				return errf("skill_publish: 'name' is required"), nil
			}
			// The description is what a teammate's catalog line shows — a shared
			// skill nobody can tell the purpose of is exactly the garbage the
			// EX-6 governance worry was about, so it is mandatory here even
			// though the on-disk format tolerates its absence.
			if strings.TrimSpace(in.Description) == "" {
				return errf("skill_publish: 'description' is required — it is the line teammates see in their skill list"), nil
			}
			if err := mc.Space.PublishSharedSkill(name, in.Description, in.Body, in.Overwrite); err != nil {
				if errors.Is(err, agentdef.ErrSkillExists) {
					return errf("skill_publish: a shared skill named %q already exists. "+
						"Set overwrite:true to publish a new version of it, or pick another name.", name), nil
				}
				return errf("skill_publish: %v", err), nil
			}
			verb := "Published"
			if in.Overwrite {
				verb = "Republished"
			}
			return okf("%s shared skill %q — every member loads it at its next run boundary "+
				"(a member's own same-named skill still wins). If the team should apply it right away, "+
				"announce it with send_message to \"all\".", verb, name), nil
		},
	}
}
