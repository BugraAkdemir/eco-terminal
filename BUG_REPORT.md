# BUG_REPORT.md — eco-terminal Open Bug List

> **Purpose:** the list of bugs that are genuinely open right now and block a
> stable release. **Fixed bugs are not kept here** — once fixed, delete the
> entry entirely. Its permanent record is git history (the commit message)
> and, if it's the kind of finding worth a future agent knowing about even
> after it's fixed, `AGENTS.md`'s Known Pitfalls section (`~~struck~~ →
> fixed` convention). This file's only job is: *what's still broken, right
> now, today.*
>
> **Last updated:** 2026-10-20 — file created, no bugs recorded yet.

---

## Summary

| Severity | Open |
|----------|------|
| 🔴 CRITICAL | 0 |
| 🟠 HIGH | 0 |
| 🟡 MEDIUM | 0 |
| 🟢 LOW | 0 |
| 🔧 TECH DEBT | 0 |
| **TOTAL** | **0** |

**Severity guide** (keep this consistent across sessions/agents so the table
above stays meaningfully comparable over time):

- **CRITICAL** — data loss/corruption, security vulnerability, or the app is
  unusable for its primary purpose. Blocks any release.
- **HIGH** — a real feature is broken or produces wrong results for a common
  case. Should block the next release unless explicitly deferred.
- **MEDIUM** — a real bug, but narrow (edge case, cosmetic-but-confusing,
  workaround exists).
- **LOW** — minor/cosmetic, no workaround needed, or affects a rarely-used
  path.
- **TECH DEBT** — not a bug a user would notice today, but a maintainability
  or scalability risk (missing tests on a critical path, an architecture
  decision that will need revisiting before the next big feature, etc.).

Entry format: `- **<ID>** <one-line title> — <path/to/file.ext>` followed by
2-4 sentences: exact reproduction, root cause if known, current impact, and any
fix attempt that was tried and reverted (and why).

---

## Open Bugs

### 🔴 CRITICAL

_None._

### 🟠 HIGH

_None._

### 🟡 MEDIUM

_None._

### 🟢 LOW

_None._

### 🔧 TECH DEBT

_None._

---

## Residual (not a fix, just a thing to keep tracking)

_None._

---

*A bug fixed in this list should be deleted from it entirely, not marked
struck-through — `git log` and commit messages are the permanent record for
this file's purpose. (Contrast with `AGENTS.md`'s Known Pitfalls section,
which deliberately does keep struck-through history — the two files have
different jobs: this one is "what's open right now," that one is "what have
we learned about this codebase over time.")*
