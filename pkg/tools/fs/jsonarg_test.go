package fs

import "encoding/json"

// jstr marshals s as a JSON string literal (quotes included) so Windows
// path backslashes survive the trip into tool input — raw `"`+s+`"`
// concatenation builds invalid JSON ("C:\Users\..." → bad \U escape).
func jstr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
