# handoff.md — how this file works

`handoff.md` is the running session log a coding agent reads at the **start**
of every session and writes to at the **end** of every session. It is what
lets context survive across sessions and across different models — the agent
should never have to reconstruct "what was I doing" from git log alone.

**Rules for this file:**

1. **Newest entry always goes on top.** Prepend, never append — the top of
   the file is always "what happened most recently and what's pending."
2. **One entry per session**, using the exact heading format below. If a
   session has multiple distinct legs (e.g. resumed later the same day),
   either add a new dated entry or clearly mark a sub-section — don't merge
   unrelated work into one undifferentiated blob.
3. **Every entry ends with a "Next Session" section.** This is the single
   most important part of the entry — it's the actual handoff. Be specific:
   name files, name the exact next step, name what was *deliberately left
   out of scope* and why (so it isn't silently forgotten or redone).
4. **Verification results are pasted, not summarized.** "Tests pass" is not
   verification; the actual command and its actual output (or a faithful
   excerpt) is.
5. This file is allowed to grow long. Don't prune old entries — they're the
   permanent record. If it gets unwieldy to read top-to-bottom, that's a sign
   to `grep` for a module/date, not a sign to delete history.
6. Fixed bugs get their permanent record in `AGENTS.md`'s Known Pitfalls
   section (with the `~~struck~~ → fixed` convention) and in git history —
   this file's job is session narrative and handoff state, not a bug
   database. Open bugs live in `BUG_REPORT.md`.

**Entry shape** (copy for each new session, newest on top):

```
# Handoff — <YYYY-MM-DD> (Session <N>) — <one-line title>
## Summary            (2-5 sentences + **Commit status:** hashes / "not committed")
## What Was Done      (sub-tasks; root cause for bugs; files table; "Deliberately not done")
## Verification       (exact commands + pasted output; state what was NOT verified)
## Next Session       (specific, numbered next steps)
```

---

# Handoff — 2026-10-20 (Session 1) — Agent şablonlarının kurulumu

## Summary

`setup.md` talimatına göre `*.example.md` şablonlarından `AGENTS.md`,
`handoff.md` ve `BUG_REPORT.md` oluşturuldu, example dosyaları silindi. Proje
teknolojisi henüz belli olmadığı için tüm teknoloji/mimari/komut/proje-özel
bölümler bilerek `TBD` bırakıldı; süreç bölümleri (Agent Working Rules,
Known Pitfalls kuralları, handoff kuralları) şablondan aynen alındı.

**Commit status:** not yet committed — dosyalar working tree'de duruyor.

---

## What Was Done

### 1. Şablonların kurulması

| File | Change |
|------|--------|
| `AGENTS.md` | `AGENTS.example.md`'den oluşturuldu; Tech Stack, Architecture, Module Map, Development, Gotchas, Code Style `TBD` |
| `handoff.md` | `handoff.example.md`'den oluşturuldu; örnek girdiler silindi, bu ilk gerçek girdi eklendi |
| `BUG_REPORT.md` | `BUG_REPORT.example.md`'den oluşturuldu; tüm sayaçlar 0 |
| `*.example.md` (3 dosya) | Silindi |

**Deliberately not done / out of scope:** Teknoloji yığını, komutlar,
commit convention'ı ve AI-attribution (Co-Authored-By) politikası
doldurulmadı — proje koda bağlanınca / sahibi karar verince eklenecek.
`setup.md` silinmedi (talimat dosyası; istenirse silinebilir).

---

## Verification

Kod olmadığı için derleme/test komutu yok; yalnızca dosyaların varlığı
kontrol edildi (bkz. oturum çıktısı).

---

## Next Session

1. Proje teknolojisi belli olunca `AGENTS.md`'deki `TBD` bölümlerini doldur:
   Tech Stack, Architecture/Module Map, Development komutları, Verification Commands.
2. Rule 6'daki commit convention'ı ve attribution politikasını kullanıcıyla netleştir.
3. İlk gerçek kod/bug bulgularını `AGENTS.md` Known Pitfalls ve `BUG_REPORT.md`'ye işle.
