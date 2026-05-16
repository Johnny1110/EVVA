# Test Cases — Manual / Acceptance

This directory holds the **manual** test scenarios for evva: flows that
can't be cleanly expressed as a Go unit test (`*_test.go`) because they
exercise an interactive TUI, depend on a real terminal, or span the
whole agent → tool → UI loop end-to-end.

Automated tests still live next to the code (`go test ./...`). What
lives here is the human-in-the-loop layer: things a reviewer should
actually run before shipping a behavioral change.

---

## 1. Layout

```
docs/test-case/
├── README.md            ← this file
└── tools/               ← one markdown file per evva tool
    ├── write_file.md
    ├── edit_file.md
    └── ...
```

`tools/` is the only category for now. Other categories
(`ui/`, `agent/`, `session/`) can grow alongside it as scenarios that
aren't tool-scoped show up.

One file = one feature surface. Don't shard a single feature across
multiple files; reviewers should be able to read one file top to bottom
and know what to check.
