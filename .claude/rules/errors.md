---
paths:
  - "**/*.go"
---

# Error handling

- Wrap errors with context: `fmt.Errorf("doing X: %w", err)`
- Return errors up the call stack; only handle (log/status-update) at controller boundary
- Use `errors.Is` / `errors.As` for type checks, never string matching
- Don't swallow errors silently; if ignoring, add a brief inline comment explaining why
