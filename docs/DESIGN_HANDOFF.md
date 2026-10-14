# SamenKlaar — Design handoff (web + mobile)

Status: approved design, 14 Oct 2026
Design canvas (source of truth for visuals): https://claude.ai/code/artifact/7f82706e-21c4-4e84-ab23-9f50f07a1224
Audience: Claude Code implementing in this repo (Svelte 5 + Tailwind import in `src/app.css`, Go API).

---

## 0. How to use this document

Read sections 1–4 before touching any file. Then implement in the order of section 10, one phase per PR.

Ground rules:

1. **Surgical, not a rewrite.** Keep every existing behaviour, API call, guard and state transition. This is a visual and layout change. Where a screen needs data or behaviour the app does not have, it is marked **NEW · needs backend** or **NEW · UI only**. Do not build NEW · needs backend items unless the phase says so.
2. **All user-facing strings go through `t()`** from `src/lib/i18n.svelte.ts` with an `nl` entry. Section 8 lists new strings with Dutch drafts (flagged for native review).
3. **Tokens only.** No raw hex values in components. Use the CSS custom properties from section 2.
4. **One primary button per view.** If you need two, one of them is wrong.
5. **Do not invent product facts in copy** (response times, ratings, counts). Use only data the API returns.
6. Run `make check-flow` after every phase. It must stay green.

---

## 1. Design principles (why the screens look the way they do)

| # | Principle | What it means in code |
|---|---|---|
| 1 | **One next action, always in the same place** | Every request view derives a single "next step" from `requestStatus()`. Web: right column `NextStepPanel`. Mobile: the offer/action card in the thread or the sticky footer button. |
| 2 | **"Your turn" is the blue at full strength** | Filled Blue 700 pill + Blue 50 panel = the user must act. Waiting on someone else = grey outlined pill. Never use another colour for "your turn". |
| 3 | **What's agreed is always visible** | `AgreementSummary` shows each term (service, price, visit, address) with its own state (agreed ✓ / pending / held). |
| 4 | **Chat never changes the deal** | Prices, visits and extra work are offer cards with explicit accept/decline. Plain messages can't change terms. |
| 5 | **Plain language and the reason behind a disabled button** | Every disabled primary button has a one‑line reason under it ("Add your address to continue"). |

---

## 2. Design tokens

Replace the `:root` block in `src/app.css` with the block below. The **legacy aliases** at the bottom keep existing selectors working while you migrate; delete them in Phase 6.

```css
:root {
  /* Neutrals */
  --sk-bg: #FFFFFF;          /* page background — pure white, no tinted ground */
  --sk-subtle: #F6F8FB;      /* secondary surfaces, segmented track, info banners */
  --sk-line: #E3E7EF;        /* card borders, dividers */
  --sk-line-strong: #CDD4E1; /* hover borders, empty step circles */
  --sk-input-border: #8A94A8;/* form controls (≥3:1 on white) */
  --sk-muted: #5B6478;       /* secondary text (5.9:1 on white) */
  --sk-ink: #0F1729;         /* primary text */

  /* Delft blue */
  --sk-blue-50: #EDF1FA;     /* your-turn panel, selected list item, own chat bubble */
  --sk-blue-200: #C9D5EF;    /* your-turn panel border, focus halo */
  --sk-blue-700: #1F3D8F;    /* primary buttons, links, your-turn pill, focus ring */
  --sk-blue-800: #183277;    /* primary hover */

  /* Status (used sparingly) */
  --sk-success: #1E6B4A;  --sk-success-bg: #E8F4EE;
  --sk-warning: #7A5200;  --sk-warning-bg: #FBF3E0;  --sk-warning-dot: #C98A0A;
  --sk-danger:  #B42318;  --sk-danger-bg:  #FDEEEC;  --sk-danger-border: #F3C5BF;

  /* Skeleton */
  --sk-skeleton-1: #EEF1F5; --sk-skeleton-2: #F3F5F8;

  /* Type */
  --sk-font: "IBM Plex Sans", ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  --sk-font-mono: "IBM Plex Mono", ui-monospace, monospace;

  /* Radius */
  --sk-r-control: 10px;  /* buttons, inputs */
  --sk-r-panel: 12px;    /* offer cards, banners, callouts */
  --sk-r-card: 16px;     /* cards */
  --sk-r-sheet: 20px;    /* bottom sheets, hero preview */
  --sk-r-pill: 999px;

  /* Elevation — cards use borders, not shadows */
  --sk-shadow-raised: 0 1px 2px rgba(15,23,41,.12);   /* selected segment only */
  --sk-shadow-float: 0 8px 24px rgba(15,23,41,.10);   /* menus, popovers */
  --sk-shadow-toast: 0 8px 24px rgba(15,23,41,.18);
  --sk-scrim: rgba(15,23,41,.40);

  /* Spacing scale (4-based) */
  --sk-1: 4px; --sk-2: 8px; --sk-3: 12px; --sk-4: 16px; --sk-5: 20px;
  --sk-6: 24px; --sk-8: 32px; --sk-10: 40px; --sk-12: 48px; --sk-16: 64px;

  font-family: var(--sk-font);
  color: var(--sk-ink);
  background: var(--sk-bg);
  font-size: 15px;
  line-height: 1.55;
  font-synthesis: none;

  /* Legacy aliases — remove in Phase 6 */
  --blue: var(--sk-blue-700);
  --blue-dark: var(--sk-blue-800);
  --blue-soft: var(--sk-blue-50);
  --ink: var(--sk-ink);
  --muted: var(--sk-muted);
  --border: var(--sk-line);
  --surface: #FFFFFF;
  --lime: var(--sk-blue-700); /* lime is retired */
}
```

Also:

- `index.html`: `theme-color` → `#1F3D8F`. The `<link rel="preload">` still points at `geist-variable.woff2`, which no longer exists. Point it at `/fonts/IBMPlexSans-Regular.woff2`.
- Remove the `IBMPlexSans-Light` `@font-face`. Weights in use: **400, 500, 600** only.
- `public/samenklaar-mark.svg`: background `#1F3D8F`, strokes `#FFFFFF` (lime retired).
- Contrast is checked for every text/background pair above (all ≥4.5:1 except the input border, which is a non-text 3:1 boundary).

---

## 3. Typography

| Role | Size / line-height / weight | Letter-spacing | Use |
|---|---|---|---|
| Display | 44 / 1.1 / 600 (landing: `clamp(40px,5vw,58px)` / 1.06) | -0.02em | Landing hero only |
| H1 | 32 / 1.2 / 600 (mobile 24–28) | -0.015em | Page titles |
| H2 | 24 / 1.25 / 600 (mobile 20–22) | -0.01em | Card titles, request title |
| H3 | 18 / 1.35 / 600 | 0 | Section titles |
| Body | 15 / 1.55 / 400, strong 500 | 0 | Default |
| Small | 13 / 1.5 / 400 | 0 | Meta, helper text |
| Caption | 12 / 1.4 / 400 | 0 | Timestamps, footnotes |
| Label | 12 / 600 / UPPERCASE | 0.08em | Section eyebrows ("YOUR TURN", "AGREEMENT") |
| Amount | 20–40 / 1.1–1.2 / 600 | -0.01em | Prices; always `font-variant-numeric: tabular-nums` |

Rules: no weight 300 anywhere. Mobile inputs use 16px text (prevents iOS zoom). Money format: `€ 95,00` (nl‑NL style with a space after €), using `Intl.NumberFormat('nl-NL', {style:'currency', currency:'EUR'})`.

---

## 4. Spacing, shape, focus, icons

- Spacing inside a component: 8–16px. Between components: 20–24px. Between sections: 40–48px.
- Touch targets: **min 44×44px** for every interactive element (chips may be 40px tall when they sit in a wrapping group with 8px gaps).
- Focus: `outline: 2px solid var(--sk-blue-700); outline-offset: 3px;` on buttons and links. Inputs: border `--sk-blue-700` + `box-shadow: 0 0 0 3px var(--sk-blue-200)`. Danger buttons use a danger-coloured ring.
- Icons: inline SVG, 24×24 viewBox, `stroke-width: 2`, round caps/joins, `fill: none`, 16–20px rendered. Decorative icons get `aria-hidden="true"`. No emoji, no glyph arrows like `→` or `⌄` in UI text (replace the existing `⌄` in `.account-menu summary::after` and the `→` in button labels).
- Icon set used in the designs: calendar, price (euro), message, location pin, photo, check, clock, wrench, alert triangle, info circle, lock, send arrow, plus, close, chevron left/right/down, camera, star. Use one consistent set (e.g. Lucide via inline SVG) and match the stroke rules above.

---

## 5. Components

For each component: anatomy → variants/states → accessibility → where it lives now.

### 5.1 Button
- Height 44 (mobile primary in sticky footer: 50). Padding 0 18px. Radius `--sk-r-control`. 14–15px, weight 600 (primary/quiet) or 500 (secondary/danger).
- **Primary**: bg blue-700, white text. Hover blue-800. Disabled: bg `--sk-line`, text `--sk-muted` (never just opacity).
- **Secondary**: white bg, 1px `--sk-line` border, ink text. Hover bg `--sk-subtle`, border `--sk-line-strong`. On a blue-50 panel the border is blue-200 and text blue-700.
- **Quiet**: transparent, blue-700 text. Hover bg blue-50.
- **Danger**: transparent, danger text. Hover bg danger-bg. Confirmation step uses a solid danger button ("Yes, cancel").
- **Working**: spinner (16px stroke arc) + present-participle label ("Approving…"); keep width stable; `aria-busy="true"`.
- Labels say what happens: "Approve € 95 & book Thu 09:00", "Send to Sanne for approval". Never "Submit".
- Disabled primary always has a reason line below it (12px muted, centred).
- Existing: global `button`, `.primary`, `.secondary`, `.text-button`, `.send-button` in `app.css`. Restyle these; don't add a second button system.

### 5.2 Icon button
44×44, radius 10. Variants: primary (send), outline (attach), ghost (close, remove). Must have `aria-label`.

### 5.3 Fields
- Label above (13px/500), control, helper or error below (12px).
- Input/select: min-height 44 (48 on mobile forms), padding 0 12px, border `--sk-input-border`, radius 10.
- Focus: see 4. Error: border danger + error line with alert icon + `aria-invalid="true"` + `aria-describedby`. Disabled: bg subtle, border line, text `#8A94A8`.
- **Money input**: `€` prefix cell (subtle bg, right border) + tabular numbers.
- **Textarea**: same border, `resize: vertical` on desktop, none on mobile.
- **Area chips input** (pro onboarding): removable chips (blue-50, blue-700 text, 24px × close button with `aria-label="Remove {area}"`) + free text. Store as the existing comma-separated labels.

### 5.4 Segmented control
Track: subtle bg, 1px line border, radius 10, padding 3. Segment: min-height 34–38, radius 8. Selected: white, `--sk-shadow-raised`, ink 600. Unselected: transparent, muted 500. Use `role="tablist"`/`tab` for view switches, or radio semantics for modes. Used for: Customer/Professional switch (existing `.role-switch`), composer modes, operator tabs, operator check results.

### 5.5 Answer chip
Pill, min-height 40, 1px line border. Selected: blue-700 border, blue-50 bg, blue-700 text + check icon. Existing: `.answer-options button`.

### 5.6 Check / radio
20px box. Checked checkbox: blue-700 fill + white check. Radio checked: 6px blue-700 ring. Whole row is the label (min-height 44).

### 5.7 Status pill
| Variant | Style | When |
|---|---|---|
| your-turn | bg blue-700, white 600, white 5–6px dot | user must act |
| waiting | 1px line border, muted text, hollow dot | waiting on the other person |
| scheduled / in-progress | bg blue-50, blue-700 text, solid dot | booked, working, "Today 09:00" |
| done ("Klaar") | success-bg, success text, check icon | paid / complete |
| review | warning-bg, warning text | under review, late cancellation, in review (pro) |
| error | danger-bg, danger text | payment failed, work issue (operator list) |
| neutral | subtle bg, muted | cancelled, refunded, describing |

Padding 3px 10px, radius pill, 12px. Map from `requestStatus()` codes — see section 6.

### 5.8 App bar
- Height 64 (web), 56–60 (mobile). Bottom border line.
- Web customer: logo (mark 28px + "SamenKlaar" 18/600) · spacer · Customer/Professional segmented · notifications bell (existing `NotificationCenter`; icon button with blue-700 unread dot, panel uses `--sk-shadow-float`) · avatar button (initials, 40px circle, subtle bg).
- Web pro: same, plus a "PRO" tag after the wordmark and nav links **Jobs · Free times · Profile & services** (selected: blue-50 bg, blue-700 600).
- Operator: "OPERATOR" tag, "Updated hh:mm" + Refresh, avatar.
- Mobile: logo + avatar; the role switch moves into the account menu (existing behaviour).
- Language switch (existing) moves into the account menu on mobile; on web it can stay in the bar as a quiet text button "NL / EN".

### 5.9 Request list (left column web / requests screen mobile)
- Item: padding 12, radius 10. Title 14/600 when current, 500 otherwise. Pill on the right. Second line = **the next step in words**, never the raw status ("Approve Jan's visit · Thu 16 Oct").
- Current item: blue-50 bg (`aria-current="page"`).
- Sort: your-turn first, then scheduled by date, then waiting, then done.
- "New request" row: blue-700 text, plus icon.
- Existing: `WorkspaceNavigation.svelte` (`.request-tab`). Convert from horizontal tabs to this vertical list on web; on mobile keep it as its own screen behind the back arrow.

### 5.10 Agreement summary
- **Web**: right column section "AGREEMENT". Rows: icon (✓ success = agreed, clock = held/pending, lock = address) · label · value. Total row bold. Extra work rows appear as their own lines.
- **Mobile**: a 48px collapsible bar under the header ("✓ Thu 16 Oct, 09:00–10:30 · € 95,00 ⌄"), subtle bg. Expanding shows the same rows.
- **Pre-booking** (plan page): the "To book, confirm three things" checklist (5.15) replaces it.
- Existing: `AgreementSummary.svelte`.

### 5.11 Next-step panel (web, right column, top)
- Your turn: blue-50 bg, blue-200 border, radius 16, padding 18. Eyebrow "YOUR TURN" (blue-700). Title 17/600. One sentence. Primary button (full width) + secondary. Optional consequence line (12px muted).
- Waiting: white bg, line border, eyebrow "NEXT STEP" (muted), title says who acts ("Jan confirms the visit") + what happens if they don't.
- Below it: AGREEMENT (5.10), then AFTER THAT (plain list of remaining steps), then a danger quiet button when cancellation is allowed.
- **Rule**: on web the actionable buttons for the current step live **only** here. The thread shows a compact card for the same event with "Answer on the right →". On mobile the buttons live in the thread card / sticky footer.
- **NEW · UI only** (it reuses existing actions).

### 5.12 Message
- Other person: avatar (32 web / 28 mobile, circle, initials) + name line (12/600 muted + time) + text 15/1.55. **No bubble.**
- Assistant: same layout with a **square** avatar (radius 8, blue-50, blue-700 mark) so it never reads as a person. Name "SamenKlaar assistant".
- You: right-aligned bubble, blue-50, radius 14 14 4 14, max-width 75–84%. Meta line under it ("You · 09:41 · Read").
- Failed: white bubble, danger-border, muted text + "Not sent · Retry".
- Typing: three 6px dots in a subtle pill, `role="img" aria-label="Jan is typing"`.
- System event: centred 12px muted text between two hairlines ("Jan started work · 09:04").
- Long history: collapse to "Earlier · N messages" pill (existing `.conversation-history`).
- Thread container: `role="log" aria-live="polite"`; web max-width 720–760, centred.

### 5.13 Offer card (price / visit / extra work)
| State | Look | Actions |
|---|---|---|
| Awaiting you | blue-50 panel, blue-200 border, header icon + "Visit proposed by Jan" + your-turn pill, big value (20/600), consequence line | Primary accept + secondary counter ("Suggest another time" / "Keep the original plan") |
| Awaiting them | white, line border, waiting pill, new value + struck-through old value, quoted note | Quiet "Withdraw offer" |
| Accepted | collapses to one line: success check circle + "Visit booked · Thu 16 Oct, 09:00–10:30" + "Approved by you · time" | Optional quiet action ("Add to calendar") |
| Replaced / withdrawn | subtle bg, muted, struck-through value + reason | none |

Extra work card adds a mini ledger: Agreed so far / Extra work / **New total**. Existing: `PlanNegotiation.svelte`, `ProposalComposer.svelte`, and the change block in `CustomerJobPanel.svelte`.

### 5.14 Composer
- Bordered box (input border), radius 14, padding 10–12. Pinned to the bottom of the thread column as its own layout row (content never scrolls under it).
- Modes (segmented): **Message · Price · Visit** (customer and pro, after a match). During intake: message only, plus photo button.
- Price mode: "New total" money input + optional reason + helper "Jan must accept before the price changes." + button "Send offer".
- Visit mode: Date + Start; end is derived from service duration ("Ends 10:30 · 90‑min service · Change"); helper "Not reserved until Jan accepts." + button "Propose visit".
- Web hint: "Enter to send · Shift+Enter for a new line".
- Drafts persist per account/role/request (existing `drafts.ts`) — keep.
- Existing: `MessageComposer.svelte`, `ProposalComposer.svelte`, `VisitTimeFields.svelte`.

### 5.15 "Confirm three things" checklist (plan page)
Rows: step circle (done = success-bg ✓, active = blue-700 filled number, todo = line-strong outline) · title · sub-line · trailing quiet action ("Make an offer", "Change"). The active row expands inline to its form (address fields / time picker). Pending price offer shows the waiting state (clock icon + offer mini card + "Withdraw offer").

### 5.16 Match card
- Best fit: 1px blue-200 border (web compare grid: 2px blue-700), "BEST FIT" label, avatar + name + service, fit reason with ✓, facts grid (Price incl. call‑out / Takes / Earliest), "What's included" disclosure, primary "Review {name}'s plan", helper "Next you'll see the full price, pick a time and add your address. No payment yet."
- Alternative: line border, same facts in the same positions, secondary button.
- Web: 3 columns, equal height, buttons aligned to the bottom (`margin-top:auto`). Mobile: best fit, then "Compare 2 alternatives" screen.
- Existing: `.match-card` in `IntakeConversation.svelte`.

### 5.17 Time picker
- Grouped by day. Day label 13/600. Slots: 44px buttons, tabular numbers. Selected: blue-700 fill, `aria-pressed="true"`. Taken: dashed line border, subtle bg, struck-through, disabled, `aria-label="09:00, no longer available"`.
- Web: days as columns (`repeat(auto-fit, minmax(140px,1fr))`), empty day shows "No times". Mobile: days stacked, slots wrap.
- Footer: "Ends 10:30 · Amsterdam time" + "More days" / "Suggest my own time".
- Existing: `.visit-day` groups in `CustomerJobPanel.svelte`.

### 5.18 Price breakdown
Rows 14px with hairlines; "Not included" row muted; total row 15/600 label + 22–24/600 amount. Existing `.quote-lines`.

### 5.19 Progress
- Intake stepper: 3 segments (4px bars) labelled Details · Area · Match, current label blue-700 600.
- Pro onboarding: mobile = "Step n of 3" + bars; web = numbered steps in the app bar.
- Next-steps list (booked/in-progress cards): 22px circles — done ✓ success, current = blue ring with dot, todo = outlined number.

### 5.20 Completion record / evidence
Photo grid (2–3 columns, 84–150px tall, radius 10–12, each a button that opens the image), work note with label, "Uploaded by Jan · time · Private to you and Jan". Upload tile: dashed input border, blue-700 "Add" (web: "Add or drop", supports drag & drop). Existing: `.evidence-item`, pro upload in `ProfessionalWorkspace.svelte`.

### 5.21 Banners, toast, empty, skeleton
- Error banner: danger-bg, alert icon, bold title + muted explanation that says what did **not** happen ("Nothing was booked or charged.") + recovery button. `role="alert"`.
- Info banner: subtle bg, info icon in blue-700. Test mode: "Test mode. Payments use the PayPal sandbox — no real money moves."
- Toast: ink bg, white text, `--sk-shadow-toast`, optional "Undo" in blue-200. `role="status"`. Auto-dismiss 5s (pause on hover/focus).
- Empty state: dashed line-strong border, 44px blue-50 icon tile, title, one sentence, primary action.
- Skeleton: shapes mirror the card they replace; `aria-hidden="true"` with a separate visible status line ("Checking plumbers who serve De Pijp…").

### 5.22 Bottom sheet (mobile)
Scrim `--sk-scrim`, sheet radius 20 top, grab handle 40×4, title + close icon button, `role="dialog"` + focus trap + Escape closes. Used for "Propose extra work". On web the same content is a right-column panel or a centred dialog (max 520px).

### 5.23 Pro tab bar (mobile only)
3 tabs (Jobs · Free times · Profile), icon + 12px label, 52px tall, current blue-700 600 with `aria-current="page"`. Web uses the app bar nav instead.

---

## 6. Status → UI mapping

Source: `src/lib/request-status.ts` codes. Customer view unless noted. `{pro}`/`{customer}` = first name.

| Code | Pill | List second line | Next-step title (web panel / mobile card) |
|---|---|---|---|
| describing | neutral "Describing" | Describing the job | — (intake chat) |
| matching | your-turn | Choose a professional | Match screen |
| quote | your-turn (pro: waiting) | Confirm {pro}'s plan | "To book, confirm three things" |
| booking | your-turn | Choose a visit time | Time picker |
| price / date (actor = other) | your-turn | Review the price offer / proposed visit | Offer card awaiting you |
| price / date (actor = you) | waiting "Waiting on {name}" | Offer sent to {name} | Offer card awaiting them |
| appointment_confirmation | waiting (pro: your-turn "Confirm visit") | {pro} confirms Thu 09:00 | "Time held — {pro} confirms next" (pro: "Confirm by hh:mm") |
| choose_new_visit | your-turn | Choose another time | Error banner + time picker |
| booked | scheduled | Thu 16 Oct · 09:00 | "You're booked" + steps |
| working | scheduled "In progress" | {pro} is working | "{pro} is working on it" |
| change | your-turn (pro: waiting) | Review extra work · + € 35 | Extra work card |
| review | your-turn (pro: waiting) | Check {pro}'s work | Completion record + Approve |
| payment | your-turn | Pay € 130 | Pay screen |
| paid | done "Klaar" | Paid · date | "All done" + review |
| dispute_review | review "Under review" | Payment paused | "We're looking into it" |
| refund_pending | review "Refund processing" | Refund processing | Info banner |
| refunded | neutral "Refunded" | Refunded · date | Info banner |
| cancellation_review | review | Cancellation under review | Info banner |
| cancelled | neutral "Cancelled" | Cancelled · date | Closed state, history kept |

Missing-documentation case (evidence submitted, note missing): your-turn "Ask {pro} for the work note" → after asking: waiting.

---

## 7. Layout & breakpoints

### 7.1 Shells
| Shell | Web (≥1280) | 1024–1279 | 640–1023 | <640 (phone) |
|---|---|---|---|---|
| Customer / pro request | 3 columns: list 280–320 · thread flex (content max 760) · right panel 320–360 | list + thread; right panel becomes the collapsible agreement bar under the thread header, and the next-step card sits above the composer | thread only; list opens from a "Requests" button in the header (drawer, focus-trapped) | Phone screens as designed (header + collapsible bar + thread + composer/footer) |
| Intake | list · chat · "Your job so far" panel | list · chat (summary as collapsible bar) | chat only | Phone intake screens |
| Plan page | list · content (max 640) + sticky booking summary (340) | content + summary stacked under the form | single column, summary as sticky footer | Phone plan screens (sticky footer button) |
| Match | list · 3-column compare | 3 columns if ≥260px each, else 1 | best fit + "Compare" | Phone match screens |
| Pro onboarding | centred form (max 1120) + sticky preview | same, preview under form | single column | Phone onboarding |
| Operator | list 340–400 + detail (max 920) | same | list then detail (stacked) | Not designed (desktop tool) |
| Landing | max-width 1200, 2-col hero | same | stacked hero | stacked; nav collapses to Sign in + Start |

Implementation: CSS grid/flex with `flex-wrap` and `minmax`; no JS breakpoints except opening/closing the drawer. Use `100svh` and the existing visual-viewport keyboard handling for the composer. Respect `safe-area-inset-bottom` on sticky footers.

### 7.2 Fixed rules
- Composer and sticky footers are layout rows, never overlays. Thread scrolls; composer doesn't.
- Page horizontal padding: 16 (phone), 24–32 (tablet), 40 (web content areas).
- No horizontal page scroll at 320px.

---

## 8. Screens

Canvas board names in **bold**. "Files" = where the change lands.

### Flow 1 — Customer intake
Files: `CustomerWorkspace.svelte`, `IntakeConversation.svelte`, `WorkspaceNavigation.svelte`, `MessageComposer.svelte`.

1. **Describe the job** (mobile 01 / Web D1): H1 "What needs fixing?", one-line explainer, large composer with "Add photo(s)", three start cards (Plumbing / Carpentry / Electrical — "Assessment visit"), privacy footnote. Web adds the "What happens next" panel. Replaces `.welcome` + `.starters`.
2. **AI clarifies** (02 / D2): 3-step progress (Details · Area · Match). Assistant asks one question; chips under the question; free typing always allowed. Web right panel "Your job so far" fills as answers arrive (trade ✓, problem, when "Asking now…", area "Next", photos "Add photos"). **NEW · UI only** (the data is in the intake state).
3. **Area check** (03): the area form renders as a card in the thread (replaces the composer for this step): label "Neighbourhood or postcode area", lock note "Your street address stays private until you choose a professional.", primary "Find professionals". Suggestion chips (e.g. "De Pijp · 1072–1074") are **NEW · needs backend**; ship without them.
4. **Finding a pro** (04): "Your job" summary card with Edit (what the pro sees first) + status line with spinner + skeleton match card. **NEW · UI only**.
5. **Best fit** (05 / D3): H1 "A plumber who fits" (mobile) / "3 plumbers cover De Pijp" (web). Match card per 5.16. "Not my problem? Describe it again" quiet link. Rename "Compare two alternatives" → "Compare 2 alternatives"; "Review this plan" → "Review {name}'s plan".
6. **Compare alternatives** (06 / D3): equal cards, same facts in the same rows. Pro without times: Earliest "No set times" + "You'd agree a visit time together in chat."
7. **Area not covered** (07): icon tile, H1 "No {trade} covers {area} yet", honest explanation (not a booking, no promised match), primary "Save to the waitlist", divider "or", "Try a different area". Saved state: success line "Saved. Your request stays in your account."

### Flow 2 — Plan & booking
Files: `CustomerJobPanel.svelte`, `AgreementSummary.svelte`, `PlanNegotiation.svelte`.

1. **Review the plan** (01 / D4): pro header, price breakdown, "What's included" disclosure, then the "To book, confirm three things" checklist: Price (✓ published price, "Make an offer"), Your address (active: street/number/postcode + optional access notes; "Only {pro} will see it"), Visit time (todo, shows earliest). Sticky footer (mobile) / summary card (web): disabled "Approve & book" + reason "Add your address to continue".
2. **Address & time set** (02 / D4): address row collapses to ✓ + "Change"; time picker expanded, earliest preselected. Primary "Approve € 95 & book Thu 09:00". Helper: "Confirms the price and holds the time while {pro} confirms. You pay only after approving the finished work." This **replaces** the current two buttons (`approve-and-book` / `approve-quote` + choose time): call `approve-and-book` with the selected slot; if no slot is selected (pro has no times), call `approve-quote` and open Visit proposal.
3. **Waiting for {pro} to confirm** (07-Confirming / D5): status waiting. Card "Time held — {pro} confirms next" + steps + "Cancel request". Uses `appointmentConfirmation.status === 'pending'`.
4. **Booked** (03): status scheduled. "You're booked" + steps + "Add to calendar" (**NEW · UI only**: generate an .ics client-side from the appointment) + "Cancel visit" (existing pre-work cancellation).
5. **Alt · Price offer pending** (04): price row shows the waiting state; approve disabled with reason "Waiting for {pro} to answer your € 85 offer".
6. **Edge · No published times** (05): step 3 becomes "Suggest a visit time" (date + start, derived end) + info "Nothing is reserved until you both agree." Primary "Approve € 85 & propose Fri 10:00".
7. **Edge · Time just taken** (06): error banner "Thu 09:00 was just taken — Nothing was booked or charged. Pick another time below — your price and address are kept." Slot struck through, no selection, primary disabled "Choose a time to continue". Also used for `choose_new_visit` (pro declined/expired) with title "{pro} couldn't confirm Thu 09:00".

### Flow 3 — Visit, completion & payment
Files: `CustomerJobPanel.svelte`, `JobReviewPanel.svelte`.

1. **Work in progress** (01): "{pro} is working on it · Started 09:04", steps, info "If {pro} finds more to fix, he'll send it here first. Nothing extra happens without your approval."
2. **Extra work found** (02 / D6): pro message + extra work card (5.13) with "Approve new total · € 130" / "Keep the original plan" + consequence line. Maps to `approve-change` / `decline-change`.
3. **Review the work** (03 / D7): H1 "{pro} says it's done. Check his work." Photos, work note, "What you agreed" ledger, AI compare disclosure ("The assistant can point out gaps. It can't confirm workmanship or safety." → existing `reviewEvidence`). Primary "Approve the work", secondary "Something's not right", helper "Approving doesn't pay yet — you'll confirm payment next."
4. **Pay** (04): success line "You approved the work · time", H1 "Pay {pro}", amount 40px, breakdown, test-mode info banner, primary "Pay € 130 with PayPal" (maps to the existing approve-payment → PayPal sequence; keep the reconcile button for uncertain states, styled secondary with copy "Check payment status"). Helper "You'll confirm the amount on PayPal, then return here."
5. **Klaar** (05): success icon, "All done", paid amount + time, rating (5 star radio buttons, 44px targets) + optional text + "Post review" (existing `JobReviewPanel`), links "Job history & photos", "Start a new request".
6. **Edge · Proof incomplete** (06): checklist (photos ✓, work note — warning "Missing"), primary "Ask {pro} for the work note" (existing `request-details`), approve disabled "Available once the work note is added".
7. **Edge · Problem reported** (07): review pill "Under review", "We're looking into it — Payment is paused…", the customer's report quoted, next steps, chat stays open. The "What went wrong?" input before this screen is a single textarea dialog (not drawn): title "What's not right?", helper "Payment pauses while our team looks at it.", buttons "Report problem" / "Cancel".

### Flow 4 — Professional onboarding
Files: `ProfessionalPending.svelte`, `PayPalPayoutSetup.svelte`, `ProfessionalAvailability.svelte`.

1. **Become a professional** (01): H1 "Get jobs that are already clear", 3 steps, "Takes about 5 minutes. Our team reviews every profile before customers can book you.", primary "Start setup".
2. **Trade & areas** (02): display name, trade as radio cards (Electrical: "Assessment visits only for now"), area chips input (5.3).
3. **Your first service** (03 / D8): service name, "What the price covers" (+ "Our assistant uses this to match the right jobs to you."), price + call-out (+ duration: 30/60/90/Other segmented on mobile, select on web), materials, not included. Live "Customers will see" preview (price = service + call-out). Web: "Saved hh:mm" autosave indicator.
4. **When you're free** (04): add-time form (date, start, derived end, "Add time"), list grouped by day with remove buttons. Primary "Send for review", quiet "Skip — I'll agree times in chat".
5. **In review** (05): review pill "In review", checklist (Profile sent ✓, Identity and business check — current, Payout account — "Before paid jobs" → links into `PayPalPayoutSetup`), "While you wait" links, "Switch to customer view".

### Flow 5 — Professional jobs
Files: `ProfessionalWorkspace.svelte`, `WorkspaceNavigation.svelte`.

1. **Jobs home** (01 / D9): sections NEEDS YOU (blue-50 items with deadline "Confirm by 12:02 today"), UPCOMING (date tile 44–48px: weekday 10–11/600 muted + day 15–17/600), WAITING ON CUSTOMER. Mobile bottom tab bar; web app bar nav. Customer name shown as first name + initial — **NEW · needs backend** if the API returns full names only (format client-side if it does).
2. **Confirm a new visit** (02 / D9): deadline panel (your-turn), job summary from intake ("Read the intake conversation"), address (now visible), primary "Confirm visit", danger "Decline and release the time", web adds "Message {customer} first". Maps to `confirm-appointment` / `decline-appointment`.
3. **Visit day** (03): address card with "Directions" (**NEW · UI only**: `https://maps.google.com/?q=` + encoded address, opens new tab) and "Message {customer}", AGREED list, "Proof needed when done" list (shown before starting), primary "Start job" + helper "Tells {customer} you've arrived and started".
4. **Propose extra work** (04): mobile bottom sheet / web panel: description, extra cost, "New total for {customer}", primary "Send to {customer} for approval", helper "Wait for her approval before doing this work. If she declines, finish the original job."
5. **Finish & send proof** (05 / D10): agreed ledger (approved extra marked "Approved"), photo upload grid, work note (autosaved — existing per-request persistence), primary "Send to {customer} for approval" (disabled until ≥1 photo and note; reason line), helper "She approves the work, then pays € 130,00".
6. **Waiting on approval** (06): waiting pill, "Sent for approval", steps, chat open.

### Flow 6 — Operator (web only)
File: `OperatorQueue.svelte`.

1. **Review a professional**: two-pane. Left: H1 "What needs a person", tabs "Applications · n" / "Exceptions · n", list items (name, trade · areas, age). Right: header with review pill "Pending review", Service and Coverage & terms cards, CHECKS list with one segmented control per check (Verified / Not yet / Not required) and an inline reason field when "Not required". Actions: primary "Approve professional" (disabled with reason until all checks answered), secondary "Save checks", danger "Decline". Policy note kept verbatim in meaning: no document numbers or uploads.
2. **Resolve a reported issue**: list items with type pills (Work issue = error, Late cancellation / No reply 24 h = review, No coverage = neutral). Detail: summary strip (Agreed total / Payment / Reported), customer's report, timeline, proof, "Your decision" with shared reason textarea and the existing actions for that exception type (Request rework / Close review; Full refund / Close without refund; Approve cancellation / Keep visit booked; Close after reconciliation).

### Flow 7 — Public landing
File: `App.svelte` (landing branch), `SignIn.svelte`.
White hero (no blue block): eyebrow "Home repairs in Amsterdam", H1 "Describe the problem. We handle the back‑and‑forth.", lead, primary "Describe your job", secondary "I'm a professional", two ✓ promises. Right: static product preview (request card, assistant question, visit offer) marked `aria-hidden`. Then How it works (3 cards), three promises on subtle band, "Starting in Amsterdam" + trade chips, pro invitation panel (blue-50), footer (Privacy, Terms, Source code, Nederlands). Sign in remains the existing Auth0 redirect.

---

## 9. Copy deck (new / changed strings)

Add to `i18n.svelte.ts`. Dutch drafts need a native review before launch.

| English | Nederlands (draft) |
|---|---|
| What needs fixing? | Wat moet er gemaakt worden? |
| Describe it in your own words. We’ll ask a few short questions, then find a professional nearby. | Beschrijf het in je eigen woorden. We stellen een paar korte vragen en zoeken dan een vakman in de buurt. |
| Your turn | Jij bent aan de beurt |
| Waiting on {name} | Wacht op {name} |
| Scheduled | Gepland |
| In progress | Bezig |
| Klaar | Klaar |
| Under review | In behandeling |
| Your job so far | Je klus tot nu toe |
| This summary is what the professional sees first. | Deze samenvatting ziet de vakman als eerste. |
| Neighbourhood or postcode area | Buurt of postcodegebied |
| Your street address stays private until you choose a professional. | Je adres blijft privé tot je een vakman kiest. |
| Find professionals | Vakmensen zoeken |
| Review {name}’s plan | Bekijk het plan van {name} |
| Compare 2 alternatives | Vergelijk 2 alternatieven |
| Not my problem? Describe it again | Klopt dit niet? Beschrijf het opnieuw |
| To book, confirm three things | Bevestig drie dingen om te boeken |
| Only {name} will see it | Alleen {name} ziet dit |
| Add your address to continue | Vul je adres in om verder te gaan |
| Approve € {amount} & book {time} | Akkoord met € {amount} en boek {time} |
| Confirms the price and holds the time while {name} confirms. You pay only after approving the finished work. | Hiermee bevestig je de prijs en houden we de tijd vast tot {name} bevestigt. Je betaalt pas nadat je het werk hebt goedgekeurd. |
| Time held — {name} confirms next | Tijd vastgehouden — {name} bevestigt nu |
| You’re booked | Je afspraak staat |
| Add to calendar | Zet in agenda |
| {time} was just taken | {time} is net bezet |
| Nothing was booked or charged. | Er is niets geboekt of afgeschreven. |
| Approve new total · € {amount} | Akkoord met nieuw totaal · € {amount} |
| Keep the original plan | Houd het oorspronkelijke plan |
| {name} says it’s done. Check his work. | {name} is klaar. Bekijk het werk. |
| Approve the work | Werk goedkeuren |
| Something’s not right | Er klopt iets niet |
| Approving doesn’t pay yet — you’ll confirm payment next. | Goedkeuren is nog geen betaling — die bevestig je hierna. |
| Pay € {amount} with PayPal | Betaal € {amount} met PayPal |
| All done | Helemaal klaar |
| We’re looking into it | We kijken ernaar |
| Payment is paused. | De betaling staat stil. |
| Get jobs that are already clear | Krijg klussen die al duidelijk zijn |
| Confirm visit | Afspraak bevestigen |
| Decline and release the time | Afwijzen en tijd vrijgeven |
| Confirm by {time} today | Bevestig vóór {time} vandaag |
| Proof needed when done | Bewijs nodig als je klaar bent |
| Start job | Klus starten |
| Send to {name} for approval | Stuur naar {name} ter goedkeuring |
| What needs a person | Wat een mens moet beoordelen |
| Approve professional | Vakman goedkeuren |
| Describe the problem. We handle the back‑and‑forth. | Beschrijf het probleem. Wij regelen het heen‑en‑weer. |

The i18n helper is keyed by the English string; add a small `{name}`‑style interpolation helper if one doesn't exist rather than concatenating.

---

## 10. Implementation order (one PR per phase)

**Phase 1 — Tokens & base elements.** Section 2 tokens, font cleanup, mark colours, base `button`/input/label/focus styles, status pill classes. *Accept:* `make check-flow` green; no `#153b88`, `#b7df90`, `--lime` or weight 300 left in `src/`; every page renders with the new palette.

**Phase 2 — App shell & layout.** App bar (web/mobile), vertical request list, three-column request layout with breakpoints (7.1), composer as a layout row, collapsible agreement bar on narrow screens. *Accept:* at 1440/1280/1024/768/390/320 nothing overlaps, no horizontal scroll, composer never covers content.

**Phase 3 — Customer intake & matching** (Flow 1). *Accept:* each intake step matches the canvas board at 390 and 1440; waitlist path works.

**Phase 4 — Plan, booking, visit, completion, payment** (Flows 2–3), including NextStepPanel, offer card states and status mapping (section 6). *Accept:* every status code shows the right pill, list line and next step; disabled primaries always show a reason.

**Phase 5 — Professional** (Flows 4–5). *Accept:* onboarding preview updates live; confirm/decline, start, extra work, finish all reachable on phone and web.

**Phase 6 — Operator, landing, cleanup** (Flows 6–7). Remove legacy aliases and dead classes from `app.css`. *Accept:* no legacy variable names remain.

---

## 11. Accessibility checklist (every phase)

- Text contrast ≥4.5:1 (all token pairs already checked); UI boundaries ≥3:1 (inputs use `--sk-input-border`).
- Every interactive element ≥44px, real `<button>`/`<a>`/`<input>`; icon-only buttons have `aria-label`.
- Visible focus on everything (section 4); logical tab order; drawer and bottom sheet trap focus and close on Escape.
- One `<h1>` per view (the old critique flagged a missing h1 in the customer view).
- Thread `role="log" aria-live="polite"`; banners `role="alert"`; toasts `role="status"`.
- Selected/pressed states use `aria-current` / `aria-pressed` / `aria-selected`, not colour alone (selected items also change weight or show a check).
- Respect `prefers-reduced-motion` for spinners/skeleton shimmer (skeletons are static by default).

## 12. Open items / NEW work summary

| Item | Type |
|---|---|
| Next-step panel, "Your job so far" panel, job summary before matching | NEW · UI only |
| Add to calendar (.ics) | NEW · UI only |
| Directions link for pros | NEW · UI only |
| Area suggestion chips during intake | NEW · needs backend |
| Customer first name + initial for pros | Check API; format client-side |
| "What's not right?" dialog | Check existing dispute endpoint; UI only if it exists |
| Desktop drag-and-drop photo upload | NEW · UI only |
| Dutch copy review | Content |
