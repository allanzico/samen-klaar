# SamenKlaar design system

The design system as it ships, on 18 October 2026. It documents what is in the code: every token, component, pattern and rule below exists in `src/`. It is the only design reference in the repository: the original design handoff and the static board export were removed on 18 October 2026.

**Sources of truth**

| What | Where |
| --- | --- |
| Tokens, base styles, shared primitives (buttons, fields, pills, banners…) | `src/app.css` |
| Shared components | `src/ui/` |
| App shell | `src/features/shell/` |
| Request page and its parts | `src/features/request/` |
| Status → look mapping | `src/lib/status-ui.ts`, `src/lib/step-view.ts`, `src/lib/next-steps.ts` |
| Copy and translations | `src/lib/i18n.svelte.ts`, `src/lib/i18n-redesign.ts`, `src/lib/agent-copy.ts` |

---

## 1. Principles

1. **Say whose turn it is.** Every request shows one next step, and the colour of its tag tells you whether it is yours, someone else's, or done. Nothing else on the page competes with it.
2. **Colour means state, never decoration.** Blue is "your turn", yellow is "booked / current", grey is waiting or pending, red is a problem. State is also carried by a dot, icon or word, never by colour alone.
3. **Quiet surfaces, one border.** Pale blue-grey ground, white cards, hairline dividers. A field has exactly one border; focus darkens it instead of adding a halo. No outlines on every element.
4. **Product density.** 14 px body text, 40 px controls (44 px on phones). Large type is for page titles and amounts only.
5. **Plain language, people first.** Sentence case, second person, first names, no jargon. Say exactly what is missing or what will happen.
6. **The assistant is a character, not a feature.** Each person's assistant has a short bot name and a bot icon, speaks in the first person, asks one thing at a time, and never decides money or finished work for a person.
7. **Layout is CSS.** Breakpoints and density are handled with media queries in the component that needs them; JavaScript only decides whether a menu opens as a popover or a sheet.

---

## 2. Foundations

### 2.1 Colour

All colour comes from custom properties on `:root` in `src/app.css`. Components never use raw hex or rgba.

**Surfaces and text**

| Token | Value | Use |
| --- | --- | --- |
| `--sk-ground` | `#F1F4F9` | Page background, disabled field fill |
| `--sk-surface` | `#FFFFFF` | Cards, boxes, fields, dialogs |
| `--sk-surface-2` | `#E3E8F1` | Segmented track, disabled buttons, banners (info), skeletons |
| `--sk-ink` | `#15264B` | Text, outlines, focus ring |
| `--sk-muted` | `#4E5B77` | Secondary text, waiting/pending |
| `--sk-line` | `#D3DAE6` | Card and box borders |
| `--sk-line-2` | `#E2E7F0` | Lighter dividers |
| `--sk-field` | `#75819A` | Field and chip borders |
| `--sk-dash` | `#7F8CA6` | Dashed empty-state border (3:1 on the page ground) |

**Brand and turn**

| Token | Value | Use |
| --- | --- | --- |
| `--sk-brand` | `#1B2F63` | App bar, primary buttons, filter-tab selected, bot avatar |
| `--sk-brand-2` | `#2B4282` | Hover on navy |
| `--sk-brand-3` | `#142450` | Primary button hover |
| `--sk-on-brand` / `--sk-on-brand-2` | `#FFFFFF` / `#C6D0E8` | Text and icons on navy |
| `--sk-primary`, `--sk-turn-btn` | `#1B2F63` | Primary and "your turn" buttons |
| `--sk-turn-soft` | `#E3ECFC` | Your-turn panels, assistant question card |
| `--sk-turn-line`, `--sk-turn-on-brand` | `#9DB8EC`, `#A9C6FA` | Turn accents |

**Signal and soft**

| Token | Value | Use |
| --- | --- | --- |
| `--sk-signal` | `#F7B32B` | Booked, current journey step, unread dot, logo |
| `--sk-signal-soft` | `#FDEBC6` | Light booked pill |
| `--sk-soft` | `#DCE4F4` | Avatars, in-progress pills, success banners, text selection |

**Status**

| Token | Value | Use |
| --- | --- | --- |
| `--sk-danger` / `--sk-danger-soft` | `#A3301F` / `#F7E0DC` | Errors, destructive actions |
| `--sk-warn` / `--sk-warn-soft` | `#875006` / `#FAE6C9` | Attention, needs a decision soon |

**Logo** (`--sk-mark-a`, `--sk-mark-on-brand`, `--sk-mark-door`) and **overlays** (`--sk-shadow`, `--sk-scrim`) complete the set.

### 2.2 Typography

Two self-hosted variable fonts (`public/fonts/`), with Helvetica Neue, Arial, sans-serif as fallback:

| Role | Family | Token |
| --- | --- | --- |
| Display: titles, amounts, buttons in the composer, names in the thread | Bricolage Grotesque (200–800, width 75–100%) | `--sk-fd` |
| Text: everything else | Instrument Sans (400–700) | `--sk-ft` |

Base: 14 px / 1.5, `font-synthesis: none`, antialiased.

| Style | Size / weight | Where |
| --- | --- | --- |
| `.page-title` | clamp(28px, 3vw, 40px), 800, width 86%, tight tracking | Page and landing titles |
| Request title | 26 px, 800 (20 px on short windows) | Request page header |
| `.section-title` | 18 px, 700 | Section headings |
| `.small-title` | 15 px, 700 | Card headings |
| Body | 14 px, 400 | Default |
| `.secondary-text` | 13 px, muted | Supporting text |
| `.meta`, `.helper`, `.field-help` | 12 px, muted | Timestamps, help, counts |
| `.eyebrow` | 12 px, 700, uppercase, +0.06em | Group labels |
| `.amount` | display 800, tabular numerals, no wrap | Prices |
| Ledger total | 22 px, 800 | Totals |
| Composer text | 16 px display 500 (14 px compact) | Message box |

Money and times use tabular numerals (`.nums`, `.amount`). On phones every input is 16 px so the browser does not zoom.

### 2.3 Shape, lines and elevation

| Token | Value | Use |
| --- | --- | --- |
| `--sk-r-pill` | 999px | Buttons, pills, chips, segmented |
| `--sk-r-card` | 14px | Fields, cards, banners |
| `--sk-r-box` | 16px | Boxes, turn panels |
| `--sk-r-tile` | 12px | Icon tiles, assistant avatar |
| `--sk-r-sheet` | 22px | Phone bottom sheets |
| `--sk-r-seg` | 3px | Stepper segments |

Lines: 0.5 px hairlines between rows (`.rows`, `.ledger`), 1 px card borders, 1.5 px field borders and outline buttons. Elevation is rare: `--sk-shadow-raised` (selected segment), `-card`, `-pop` (popovers), `-dialog`, `-sheet`, `-toast`.

### 2.4 Spacing

Scale tokens `--sk-1` … `--sk-16` = 4, 8, 12, 16, 20, 24, 28, 32, 40, 48, 64 px. Typical gaps: 8 px inside a control group, 12–14 px between a card's parts, 24 px between page sections. Page padding is 40 px on web, 16 px on phones.

### 2.5 Icons

`src/ui/icons.ts` holds 24 × 24 paths, stroke 2, round caps and joins, drawn with `currentColor`. Default size 18 px (20 px in icon buttons, 13 px in pills). Use `<Icon name="…" size={…}/>`; icons are decorative (`aria-hidden`) and always sit next to text or inside a labelled button.

Names: `home list bell calendar clock euro message pin camera photo file check check-circle wrench hammer droplet plug zap bot alert info lock send plus close chevron-right chevron-left chevron-down chevron-up more refresh external star users briefcase user sliders pen upload arrow-right arrow-up sign-out navigation trash`.

Meaning is fixed: `droplet`, `hammer`, `zap` are the three trades (plumbing, carpentry, electrical); `zap` is also the "Fastest" tag. **`bot` is the assistant, and only the assistant.**

### 2.6 Motion

Almost none. Buttons ease colour in 0.12 s; disclosures rotate their chevron in 0.15 s; the spinner turns in 0.8 s (2.4 s under `prefers-reduced-motion`, which also turns off smooth scrolling). Toasts leave after 5 s and stay while hovered or focused.

---

## 3. Layout and responsive behaviour

| Breakpoint | What changes |
| --- | --- |
| ≤ 1279 px | The "next step" aside moves into the dock above the composer |
| ≤ 1023 px | The navy request rail hides; a back link appears |
| ≤ 639 px (phone) | Navy phone header replaces the web header; 44 px touch targets; 16 px inputs; dialogs become bottom sheets; professionals get a bottom tab bar; top bar 56 px |
| Short window (≥ 640 px wide, ≤ 820 px tall) | Request header is tightened and the agreement strip hidden so the conversation keeps its room |

Top bar height is 64 px (`--sk-topbar-h`, 56 px on phones).

**Request page (web):** three columns under the top bar: a 248 px navy rail listing requests, the main column (header, scrolling conversation, composer) and a 380 px aside (min 320 px) with the next step, the agreement and what comes after. Content is capped at 820 px inside the scroll area. The page itself never scrolls: the conversation scrolls, the header and composer stay.

**Other pages:** customer home is a centred column (max 1040 px); the match view is two columns (list and a 300–380 px detail); the operator dashboard is a table-like list with a detail panel.

---

## 4. Components

### 4.1 Buttons (`.btn`)

Pill-shaped, 40 px (32 px `.btn-sm`, 44 px on phones), 14 px / 600.

| Class | Use |
| --- | --- |
| `.btn-primary` | The one main action on a surface |
| `.btn-turn` | The main action of a "your turn" panel |
| `.btn-outline` | Secondary action next to a primary |
| `.btn-light` | Neutral action on white |
| `.btn-quiet` | Tertiary, underlined text |
| `.btn-danger`, `.btn-danger-link` | Destructive actions (cancel, remove) |
| `.btn-on-navy`, `.btn-outline-navy` | On the navy bar |
| `.btn-block` | Full width (cards, phones) |

Disabled buttons turn grey (`--sk-surface-2`) and say why in a `.reason` line when the reason is not obvious. A busy button shows a spinner (`aria-busy`). `.icon-btn` is a 44 px round, icon-only button and always has an `aria-label`.

### 4.2 Fields

`.field` groups a label (14 px, 600), a control and optional `.field-help` (12 px) or `.field-error`. Controls: `.input`, `.textarea`, `.select` (40 px, 1.5 px `--sk-field` border, 14 px radius), `.money-input` (a `€` prefix cell and a tabular input), `.radio-card` (a card that is a radio), checkboxes (`accent-color: ink`) and a switch pattern (44 × 26 track). **One border only**; focus darkens it to `--sk-ink`. Invalid fields set `aria-invalid` and show a red border plus a message. Required rules and limits are stated in help text before the error.

`LocationSearch.svelte` extends the field pattern with a keyboard-accessible combobox: debounced suggestions, loading/error/empty states, explicit selection, and Mapbox attribution. It is shared by customer intake, professional base selection and the full visit address. Address/postcode/city discovery is broad; professional bases accept address/postcode only, and a visit requires a full address. Help copy explains address privacy and service-radius checks.

### 4.3 Chips, segmented control, filter tabs

- `.chip` (32 px pill, 1 px border): a short choice or quick reply. `.chip-row` wraps them.
- `.segmented`: a pill track with 36 px options; the selected option is white with a raised shadow. Used for the composer's mode and recipient and for small either/or choices.
- `.filter-tabs` with `.count`: list filters (All / Your turn / In progress / Done; All / New / Active / Finished). Selected tab is navy.

### 4.4 Pills and tags (`Pill.svelte`, `.pill`)

A pill is a state, with a dot or icon plus text. Variants and their meaning:

| Variant | Meaning | Look |
| --- | --- | --- |
| `your-turn` | You must act | Soft blue, blue dot |
| `waiting` | Waiting on someone | Muted text, outlined, hollow dot |
| `pending` | Not started or under review | Ground fill, hollow dot |
| `booked` | A visit is booked | Yellow (`light` for dense rows) |
| `progress` | Work under way | Soft blue-grey |
| `done` | Finished | Navy with check |
| `warn` / `error` | Needs a decision / a problem | Amber / red with alert icon |
| `neutral` | Draft, closed, no coverage | Grey |
| `best` / `smart` | Match highlights ("Our pick", "Fastest") | Navy with star / soft |
| `fact` | A plain fact (area, price) | White, outlined |

`small` and `row` shrink or embolden the pill. `.count` is a 22 px number bubble; `.role-badge` marks "Pro" on the navy bar.

### 4.4b Containers

`.card` (14 px radius, 1 px line), `.box` (16 px), `.turn-panel` (soft blue fill, no outline), `.rows` (children separated by a hairline), `.ledger` (label/amount rows, total larger), `.avatar` (24/36/44/48 px initials circle), `.icon-tile` (56 or 48 px soft tile with a 24 px icon), `.disclosure` (a `<details>` with a rotating chevron), `.empty` (dashed box with icon tile, title, one sentence and an action), `.skeleton` and `.spinner` for loading.

### 4.5 Banner (`Banner.svelte`)

`info`, `warn`, `error`, `success`; icon, optional bold title, text, optional dismiss. Errors use `role="alert"`, the rest `role="status"`. Used for problems that block or change what the person can do (never for decoration).

### 4.6 Toast (`Toast.svelte`, `toast-state.svelte.ts`)

One short confirmation ("Preferences saved"), `role="status"` with `aria-live="polite"`, gone after 5 s unless hovered or focused. Call `showToast(text)`.

### 4.7 Modal (`Modal.svelte`)

A native `<dialog>`: focus trap and Escape come from the platform, focus returns to the opener. 520 px wide (`wide`: 860 px), 28 px padding, title in the display font, optional footer for actions. Below 640 px it is a bottom sheet with a drag handle and full-width stacked buttons.

### 4.8 Stepper (`Stepper.svelte`)

The six-step journey, the same everywhere: **Describe · Match · Agree · Do the job · Check · Pay**. Variants: `full` (segments with labels, under request headers), `strip` (segments with titles, on phones), `compact` (a tag "Step n of 6 · name" and mini segments, in cards). Done steps are navy with a check, the current step is yellow, the rest are pale. It exposes the position through an `aria-label`.

### 4.9 Logo (`Logo.svelte`)

Two houses: a tall navy one and a shorter yellow one with a door. `onNavy` swaps the navy house for the light mark; `wordmark={false}` shows the mark alone.

### 4.10 App shell (`src/features/shell/`)

- **AppBar:** navy, logo, nav items (icon-only on phones), role badge, notification bell with an unread dot, language toggle, role switch, account avatar.
- **AccountMenu:** a popover on web, a bottom sheet on phones; account name, "Assistant preferences", "Become a professional", sign out. On phones it also carries language and role.
- **NotificationCenter:** the bell's list; each item opens its request.
- **RoleSwitch / LanguageToggle:** segmented switches (EN | NL; Customer | Professional).
- **ProTabBar:** the professional's bottom tab bar on phones (Jobs, Free times, Profile & services, with a count on Jobs).
- **RequestRail:** the navy list of requests on web, with a "New request" button for customers.

---

## 5. Patterns

### 5.1 Status → look

`statusUi(code, role, other)` in `status-ui.ts` maps every server status code to a pill variant and label, a one-line summary, a title, a sort rank (yours first, then booked/in progress, then waiting, then done) and a journey step. Lists, rails and the page header all read from it, so the same request looks the same everywhere. Add new states there, never in a component.

### 5.2 The next step (`NextStep.svelte`, `StepView`)

One view model per request: kind (`your-turn`, `waiting`, `info`), a tag, a title, optional text, amount, banner and ledger, a **primary** button, an optional **secondary**, quiet links, a danger link and a helper, plus "After this" rows. Rendered in the web aside, the dock above the composer below 1280 px, and as the phone turn card. Disabled actions carry a `reason`. While the assistant is asking something the dock hides it, because the assistant is handling that step (the page's own buttons remain below).

### 5.3 The request page

1. **Header:** back link, title, the other person with an initials avatar, area, "Request options" menu, stepper, agreement strip (four facts: price, visit, address, payment).
2. **Scroll area:** the assistant's open question (if any) → the stage for the current step (plan, evidence, review, payment) → the activity panel, related jobs, photos → "Conversation" and the thread.
3. **Aside / dock:** next step, agreement rows, what comes after, cancel.
4. **Composer:** one box under everything.

### 5.4 The thread (`Thread.svelte`)

A `role="log"` with `aria-live="polite"`. Mine: right-aligned soft-blue bubble labelled "You". Theirs: initials avatar, bold first name, a small role label on first appearance, text up to 62 characters wide. The pre-match helper is "SamenKlaar assistant" with the logo; **assistants** are bot-named (see 6). System lines (a hairline with centred text) mark events such as "You chose Marieke for this job". Offer cards (price, visit, extra work) sit at the end with the offer's tag and a "Your turn / Waiting" pill. Only the last eight messages show until "Show earlier messages". A "replying" line appears after your answer to the assistant and expires after 25 seconds.

### 5.5 The composer (`Composer.svelte`)

A bordered white box under the thread, never an overlay. Modes (a segmented control above it): **Message**, **Price offer**, **Visit time** / **Change visit** (price and visit open an inline form with the note or date fields and a Send button). After a match a **recipient switch** (`Message <name>` | `Ask your assistant`) appears; while the assistant is waiting for an answer the placeholder is "Reply to your assistant…" and Enter sends to it. On plan, check and pay pages the **compact** form is used: the recipient switch on its own row, then the text box and a 44 px round send button on one line. Enter sends, Shift + Enter adds a line, 500 characters max. Drafts are saved per account, role and request in the browser tab.

### 5.6 Forms and validation

Label above, help below, error replaces help and is announced. Never disable a submit button without a visible reason (e.g. "Write at least 10 characters to continue"). Destructive actions are `.btn-danger-link` or a confirmation dialog stating the consequence (for example a free cancellation versus a request to our team).

### 5.7 Empty, loading and error states

`.empty` for nothing-yet (icon tile, one-sentence title, one sentence of explanation, one action); a spinner with "Loading…" for waits; a page-level banner or the "We couldn't load this request" card with Try again for failures. Errors say what happened in plain words and what to do.

### 5.8 Lists

Request and job cards show: title, other person, a status pill, one summary line, and price and visit day once there is a plan; your-turn items sort first. Professionals' job cards also show the visit date block and a relative confirm deadline ("Confirm within 1 h 30 min") with Confirm / Decline buttons.

---

## 6. The assistants

Every customer and professional has an assistant (docs: [AGENT_ARCHITECTURE.md](AGENT_ARCHITECTURE.md)). Its design:

- **Identity.** A short bot name (Pip, Juno, Kit, Remy, Nova, Finn, Mika, Luca, Noor, Teo, Ivy, Sky, Rio, Ada, Zed, Bo), a hash of the person's name, so it is the same on the home screen, in the intake chat before a match and after it (`src/lib/agent-names.ts` mirrors `internal/workflow/agent_names.go`; a test keeps them equal). The two assistants on a request never share a name. The **bot icon** marks it (never the lightning bolt). In the thread the avatar is the name's first letter on a navy tile and the name is followed by a small bot icon. In cards the avatar is the bot icon itself.
- **Introduction.** The first message in a request says who it is ("Hi, I'm Pip, your assistant for this job.") once.
- **The question card (`AssistantAsk`).** Soft-blue panel at the top of the scroll area, on every screen width: bot icon, name, a "Needs you" pill, the question, **quick replies** as chips, and "Or type your answer in the box below." One question at a time. For "approve the work?" it shows up to three of the professional's photos next to the question. A new question is scrolled into view once. While it is open the dock's next step hides, and the web aside's next step is demoted (outline button and a note that either way works).
- **Quick replies.** Chips with plain labels ("Yes, book it", "No, other times", a list of times). A chip sends its label and an id; typed free text is read by the assistant (and the AI provider when needed).
- **Privacy in the layout.** A person's chat with their assistant is private to them; the agents' negotiation lines ("Could you do this for €120?", "Agreed on €120.") are shared and show with each side's bot name.
- **Activity panel.** "Your agent" panel: the bot's name, a pill (Handling this / Needs you), and the last few steps with times; "Show everything it did".
- **Banner and lists.** When an assistant needs its person and they are not on that request: "<Name> needs you" with an "Open the request" link (an `info` banner: a routine question is not a warning). On the request and job lists the banner is hidden and the asking card carries a line instead: "<Name> is asking you in the chat". The bot's name travels with the open question from the server.
- **Preferences.** "Assistant preferences" dialog: pause, ceilings, days and hours, address, what to hand over (booking without asking, approving finished work). Payment is never hand-over-able and the dialog says so.
- **Voice.** First person, short, concrete: say what it did, what it needs, what is missing and why ("I can't approve the work yet: …"). Never promise a result it does not control; offer the way forward on the page when it cannot act.

---

## 7. Content and voice

- Sentence case for buttons, titles and labels ("Approve the work", not "Approve The Work").
- Address the reader as "you"; refer to the other person by **first name**; if an account has only an email, use the part before the @; a trading name written in capitals stays whole (`firstName()`).
- Buttons are verbs ("Send reply", "Book visit"); tags are states ("Your turn", "Waiting on Marieke").
- Money is formatted by `money()` (nl-NL style: `€ 95,00`, tabular numerals) in both languages. Dates and times use `Intl` with the active locale (`en-GB` / `nl-NL`) and the visit's own time zone.
- Error messages name the problem and the fix; never blame the person; never expose internals.
- Deadlines are relative ("within 1 h 30 min"), dates absolute.
- Banned: jargon (workflow, scope, entity), exclamation marks, emoji, "Please" as filler.
- Glossary (keep these consistent):

| English | Nederlands | Note |
| --- | --- | --- |
| request | aanvraag | "Your requests" → "Je aanvragen" |
| job (professional side) | klus | "Your jobs" → "Je klussen" |
| professional | vakprofessional | avoid mixing in "vakman" |
| Your turn | Jouw beurt | short enough for a tag |
| Waiting on {name} | Wacht op {name} | |
| Klaar | Klaar | the same in both languages, on purpose |
| Booked / Scheduled | Geboekt / Gepland | |
| In progress | Bezig | |
| Under review | In behandeling | |
| free times | vrije tijden | |
| working hours | werktijden | |
| day off | vrije dag | |
| Describe · Match · Agree · Do the job · Check · Pay | Beschrijven · Kiezen · Afspreken · Uitvoeren · Controleren · Betalen | the six steps |

---

## 8. Internationalisation

- UI strings are English source text passed through `t('…', { values })`; `{name}`-style placeholders are replaced by value. A missing Dutch entry falls back to English.
- Dutch lives in `src/lib/i18n.svelte.ts` and `src/lib/i18n-redesign.ts` (the v2 strings). **All Dutch is a draft until a native speaker reviews it.**
- Dynamic copy built from codes (assistant questions and activity, step views) keeps each full sentence as its own `t()` call so it can be translated whole.
- Server-written copy has its own en/nl catalogue: the assistants' chat dialogue (in the person's language), email subjects and bodies (the recipient's saved preference), policy text (published by operators). Agent notes in the shared thread carry a code and parameters so each reader sees them in their own language.
- `npm run test:i18n` fails if an operator string has no Dutch entry.
- The language toggle sits in the top bar (EN | NL) and persists on the account.

---

## 9. Accessibility

- A visible skip link; `:focus-visible` rings (2 px ink, 3 px offset; white on navy); text fields show focus by darkening their one border.
- Native elements first: `<button>`, `<dialog>`, `<details>`, `<label for>`; ARIA only where needed (`role="log"` on the thread, `role="alert"` on errors, `aria-live="polite"` on toasts and the thread, `aria-pressed`/`aria-selected` on segmented controls and tabs).
- Touch targets are 44 px on phones, 40 px on web (32 px for small buttons).
- State is never colour alone: pills use a dot, icon and text; errors use text and an icon.
- `prefers-reduced-motion` slows spinners and disables smooth scrolling.
- Every icon-only button has an `aria-label`; decorative icons are `aria-hidden`.
- Headings follow the page structure (one h1 per page; the request title); the stepper and agreement strip expose text alternatives.
- Check list before shipping a screen: keyboard-only pass, 200 % zoom, phone width, Dutch strings (longer words), long names and amounts, reduced motion.

---

## 10. Implementation rules

1. **Tokens only.** No raw hex, rgba or px radii in components; add a token to `src/app.css` if one is missing.
2. **Shared look in `app.css`, layout in the component.** `app.css` holds primitives (buttons, fields, pills, banners, containers). A component's `<style>` holds its own layout and uses `:global()` only to reach shared classes. Tailwind is imported with `source(none)`; nothing is generated from utility class names.
3. **Reuse before adding.** Pill, Banner, Modal, Stepper, Icon and the shared classes cover almost everything. A new component needs a reason a shared one cannot serve.
4. **States come from `status-ui.ts`/`step-view.ts`,** never from a component's own if-chains.
5. **One border per field, no halos.** Cards use a 1 px line or a soft fill, not both with a shadow.
6. **Text through `t()`.** Add the Dutch draft in the same change.
7. **Responsive in CSS,** with the breakpoints in section 3.
8. **Verify:** `npm run check` (0 errors), `npm run test:controller`, `npm run test:i18n`, then look at the page at phone width, 1000 px, 1280 px and a short window.

Adding an icon: add the 24 × 24 path to `ICONS` in `icons.ts` (stroke 2, round caps, no fill) and use `<Icon name="…"/>`; the name is type-checked.

---

## 11. How the shipped system differs from the original design brief

- **Tighter scale.** Body 14 px, controls 40/32 px (44 px on phones), page titles 28–40 px, against the larger sizes on the original boards.
- **No halo.** The light-blue outline on composers and your-turn cards is gone; your-turn is a fill. Fields have one border.
- **Assistants.** Bot names, the bot icon, the question card, quick replies, the recipient switch and the activity panel were not on the original boards; they are specified here.
- **Short-window header.** The tightened header on windows under 820 px tall is an addition.
- **Dutch** is a full set of drafts that needs native review.

Open design work: a native review of Dutch copy; a visual pass of the operator screens with real data; push notifications; and a decision on a dark theme (none exists).
