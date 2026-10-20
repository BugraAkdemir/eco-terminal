# Eco Terminal — Design direction ("Saha Atlası / Field Atlas")

Applies to every Flutter screen (B foundation, C wizard/expert, F intelligence UI). Color tokens: `ARCHITECTURE.md`.
**Rule #1: usability beats decoration.** Users range from villagers/farmers/tourists in the field (sun glare, gloves, weak signal) to
researchers at a desktop. If a flourish costs clarity, drop the flourish.

## Concept
A naturalist's field atlas meets a modern GIS terminal: deep-navy "terminal" surfaces, thin cartographic lines, subtle topographic
contour texture, and field-guide style cards. Memorable details (use sparingly, never over data):
- faint **contour-line texture** on panel/empty-state backgrounds (vector/painter, ~4–6% opacity);
- observation **"survey tag" status stamp** (rounded label + icon) instead of plain colored dots;
- species/observation cards laid out like **field-guide entries** (photo, common name large, "Kullanıcı tahmini" small caption, date/place in mono);
- map markers = distinct **shape + icon per category** with a status ring (shape, not only color).

## Typography (bundle fonts as assets — offline-safe; OFL licenses; VERIFY Turkish glyphs ğ Ğ ş Ş ı İ ö ç ü render)
- Display / headings: **Fraunces** (characterful serif) — screen titles, big numbers in summary cards.
- Body / UI: **Atkinson Hyperlegible** (designed for legibility) — all UI text, forms, buttons.
- Data / coordinates / IDs: **IBM Plex Mono** — coordinates, reference numbers (ET-2026-…), timestamps, counts in tables.
No Inter/Roboto/system-default look. Minimum body 16 sp on mobile, 14 sp on desktop dense panels; must scale with OS text size up to 200% without clipping.

## Plain-language UX (Turkish first, simple words, no jargon)
- Short sentences, verbs on buttons: "Fotoğraf çek", "Konumu kullan", "Gönder", "Devam". One main action per screen.
- Status labels with plain meaning (always icon + text, never color alone), each with a one-line explanation on tap:
  Taslak · Gönderildi · Yapay zekâ ön inceleme yaptı · Uzman inceliyor · **Uzman onayladı** · Ek bilgi istendi · Reddedildi · Çözüldü.
- AI is always labelled **"Yapay zekâ önerisi — doğrulanmadı"**. User guesses: **"Sizin tahmininiz"**. Never present either as verified fact.
- Errors say what happened + what to do ("İnternet yok. Kaydınız telefonda saklandı, bağlantı gelince otomatik gönderilecek.").
- Emergencies (injured wildlife, fire): prominent notice telling the user to call the official emergency line (112 / local authority);
  the app must NOT claim an official body received the report.

## Mobile (citizen) rules
- Touch targets ≥ 48 dp, spacing ≥ 8 dp; primary actions at thumb reach (bottom). Sticky bottom bar with the single main button.
- Observation wizard: **one decision per screen**, big category cards (icon + 2–3 word label + tiny example), progress indicator "3 / 6",
  always-visible back, auto-saved draft, camera first. Large "Atla" for optional steps.
- **Offline is a first-class state:** persistent but calm banner "Çevrimdışı — kayıtlarınız telefonda saklanır"; queue badge on Profil/Bildirimlerim;
  per-item states (Bekliyor / Gönderiliyor / Gönderildi / Hata + "Tekrar dene").
- Bright-sun mode: light theme with high contrast is first-class (not an afterthought); body text contrast ≥ 7:1 where feasible, ≥ 4.5:1 minimum.

## Desktop (researcher / reviewer) rules
- Map-first: left Information & Intelligence panel (resizable, collapsible, ~25–30%), map fills the rest. Dense but ordered: clear hierarchy,
  tabular numerals (mono), consistent filter chips, keyboard focus rings, tooltips on icon-only buttons, shortcuts for the review queue (J/K next/prev, V verify, R reject).
- "Active region" chip always visible; AI Assistant visibly shows which region/period it is answering about (context-handoff feel).
- Charts: fl_chart, ≤ 5 colors, direct labels, units always shown, empty/insufficient-data states explained (never a blank chart).

## Motion
Restrained and purposeful (150–250 ms): panel slide, staggered list reveal on first load, marker drop-in, skeleton shimmer. Respect OS
"reduce motion" (disable non-essential animation). No looping decorative animation over the map.

## Accessibility checklist (reviewed per feature)
Semantics labels on all icon buttons/markers · focus order · text scaling 200% · color-independent status · contrast AA+ · screen-reader friendly map
alternative (list view of the same observations) · captions in tr + en.

## Do not
Purple-on-white gradients, generic card grids with no hierarchy, tiny gray-on-navy text, decorative glass that reduces map/data legibility, jargon
("RAG", "embedding", "inference") in user-facing text.
