package permission

import "testing"

func httpCall(input string) ToolCall {
	return ToolCall{Name: "http_request", Input: []byte(input)}
}

func TestHTTPRequestMethodGating(t *testing.T) {
	cases := []struct {
		name, input string
		want        Behavior
	}{
		{"GET allows", `{"method":"GET","url":"http://x"}`, BehaviorAllow},
		{"HEAD allows", `{"method":"HEAD","url":"http://x"}`, BehaviorAllow},
		{"unset method defaults to GET → allows", `{"url":"http://x"}`, BehaviorAllow},
		{"lowercase get allows", `{"method":"get","url":"http://x"}`, BehaviorAllow},
		{"POST asks", `{"method":"POST","url":"http://x","body":{}}`, BehaviorAsk},
		{"PUT asks", `{"method":"PUT","url":"http://x"}`, BehaviorAsk},
		{"DELETE asks", `{"method":"DELETE","url":"http://x"}`, BehaviorAsk},
		{"unparseable input asks (safe default)", `not json`, BehaviorAsk},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := Decide(httpCall(c.input), ModeDefault, nil, Hint{}, "", "")
			if d.Behavior != c.want {
				t.Errorf("got %v, want %v (%s)", d.Behavior, c.want, d.Reason)
			}
		})
	}
}

func TestHTTPRequestPlanMode(t *testing.T) {
	// A read-only GET is allowed even in plan mode (morally identical to web_fetch);
	// a mutating POST is denied outright like any other write in plan mode.
	if d := Decide(httpCall(`{"method":"GET","url":"http://x"}`), ModePlan, nil, Hint{}, "", ""); d.Behavior != BehaviorAllow {
		t.Errorf("plan-mode GET: got %v, want allow", d.Behavior)
	}
	if d := Decide(httpCall(`{"method":"POST","url":"http://x"}`), ModePlan, nil, Hint{}, "", ""); d.Behavior != BehaviorDeny {
		t.Errorf("plan-mode POST: got %v, want deny", d.Behavior)
	}
}

func TestHTTPRequestDenyRuleStillWins(t *testing.T) {
	// A deny rule must override the read-only auto-allow (deny rules win in step 2).
	store := NewStore()
	store.AddSessionRule(Rule{ToolName: "http_request", Behavior: BehaviorDeny, Source: SourceSession})
	if d := Decide(httpCall(`{"method":"GET","url":"http://x"}`), ModeDefault, store, Hint{}, "", ""); d.Behavior != BehaviorDeny {
		t.Errorf("deny rule should win over read auto-allow: got %v", d.Behavior)
	}
}
