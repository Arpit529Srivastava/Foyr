# Prompt: Claude Design, round 2

Paste into the existing **Foyr home page** Design canvas (not a new one), with
`docs/website/design-brief.md` attached.

---

Round 2 for the Foyr site. The attached brief is updated. Follow **§0 (domains)** and **§13 (Round 2)**.
Keep every existing board and everything already approved exactly as it is; only make these changes:

1. **Fix existing boards (§13.1):** every customer-app URL becomes `acme-crm.foyrapps.online`
   (dashboard URL bars stay `app.foyr.online/...`); footer "Made in India" as plain text, no emoji;
   mark scroll-reveal groups.
2. **Add the tech marquee (§13.2)** between the hero and the "Every app gets a front door" bento,
   on desktop and mobile.
3. **New board `Waitlist` (§13.3):** inline pill form + modal (mobile: bottom sheet) with all states:
   idle, focused, invalid, loading, success, already on list, server error.
4. **New board `Brand` (§13.4):** 3 doorway logomark options → chosen mark at 16/32/64/256px,
   wordmark + lockup, favicon, apple touch icon, 1200×630 OG image. Then swap the text wordmark in
   every header/footer for the lockup.
5. **New boards `Docs` (§13.5):** docs template, desktop 1440 + mobile 390, dark and light, with the
   Quickstart page as sample content, callouts, tabbed code block, TOC and ⌘K search modal.
6. **New boards `Pricing` (§13.6), `NotFound` (§13.7), `Changelog` (§13.8).**

Same design system (Foyr), same tokens, Inter, no emoji, no invented numbers. Use `[PRICE]`-style
placeholders where facts are missing. Lay the new boards out in a new row below the existing ones.
