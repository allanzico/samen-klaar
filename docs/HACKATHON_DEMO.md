# SamenKlaar presentation

## Production presentation

Present the working product at `https://samenklaar.com`, using the fictional people and businesses below. The owner has chosen the production URL for this presentation. Preparation is an explicit CLI operation, never an application startup task. Existing people, requests, payments and edited profiles remain unchanged.

All ten professionals offer **Kitchen sink leak repair** in **Amsterdam**: inspect and repair or replace an accessible damaged sink trap or waste-pipe joint, then test it under running water. Each visit lasts 60 minutes. Concealed pipework, cabinet alterations and electrical work are excluded. Every comparison price includes one standard replacement PVC trap section and seals, as well as the published call-out fee; extra fittings and other conditions vary by profile.

| Person | Business | Total | Weekly availability, Amsterdam time | Distinguishing condition |
| --- | --- | ---: | --- | --- |
| Noor van Dijk | Helder Kraanwerk | €110 | Mon–Fri, 09:00–17:00 | One PVC trap section and standard seals included; bounded discounts |
| Milan de Wit | Westerwater Loodgieters | €75 | Tue/Thu, 09:00–13:00 | Extra adapters excluded; first four days unavailable; no discount |
| Sara Jansen | Avondstroom Installaties | €120 | Mon–Fri, 17:00–21:00 | Evening visits; extra return visits need a new agreement |
| Omar Bakker | Singel Sanitairservice | €120 | Sat/Sun, 10:00–16:00 | Weekend visits; removal of the old trap included |
| Lotte Meijer | Kraan & Kompas | €120 | Mon/Wed/Fri, 07:00–12:00 | Resealing consumables included; no discount |
| Daan Vos | Amstel Leidingwerk | €105 | Mon–Fri, 12:00–18:00 | First seven days unavailable; nonstandard connectors quoted before work |
| Priya Smit | Waterlijn Techniek | €140 | Mon–Sat, 08:00–20:00 | Standard adapters included; custom fittings need approval |
| Levi Bos | De Gouden Sifon | €105 | Wed/Fri, 14:00–18:00 | Additional materials need approval; human price and visit approval |
| Emma Vermeer | Oever Loodgieters | €135 | Mon–Wed, 09:00–15:00 | Standard washers included; first two days unavailable; no discount |
| Yusuf de Boer | Kade & Kraan | €160 | Mon–Fri, 08:00–18:00 | Other repairs excluded; assistant paused |

The comparison pool contains ten candidates; the normal matching flow presents its highest-ranked results. Do not replace ranking with a presentation-only shortcut.

The receiving professional is **Noor van Dijk**, sign-in **`noor-van-dijk@samenklaar.test`**. The separate customer is **Alex de Vries**, sign-in **`alex-de-vries@samenklaar.test`**. These reserved addresses are login identities, not mailboxes. Passwords and Auth0 import files belong only in ignored `.secrets/` files. Names have no presentation/test prefixes.

Use **Amsterdam** or **1071 XX, Amsterdam** for location search. Mapbox resolved both, plus the public address **Museumstraat 1, Amsterdam**, during preparation. That address belongs to the [Rijksmuseum](https://www.rijksmuseum.nl/en/visit/practical-info/address-and-directions); it is a public geocoding reference, not a claimed business office, customer home or actual repair site. Select the returned canonical address in the application. No private home address belongs in the repository or recording.

## Prepare accounts

Local container commands are `make seed-demo` and `make import-seed-demo-auth0`. The hosted presentation uses the same release binary with its existing production application settings loaded, plus these explicit CLI-only settings:

```text
ENABLE_DEMO_SEED=1
EXPECTED_DEMO_ORIGIN=https://samenklaar.com
SEED_CONFIG_FILE=/secure/path/seed.env
SEED_OUTPUT_DIR=/secure/path/presentation-identities
```

Create the output directory with owner-only access. The seed configuration contains `SEED_PASSWORD`, `AUTH0_MANAGEMENT_TOKEN` and `AUTH0_CONNECTION_ID`; obtain fresh credentials for the intended tenant. Run `/api seed-demo`, then `/api import-seed-demo-auth0` in the release container, or the equivalent compiled binary with `APP_CONFIG_FILE` pointing at the intended ignored application configuration. The container entry point is `/api`; it has no interactive shell. Use a one-off container/job with the secret files and writable output directory mounted. Do not add these seed switches to the running web service. The import preserves existing users, resumes known jobs and does not send completion emails. Database seeding preserves profiles, working hours and assistant preferences on repeat runs. New availability starts from the preparation date and follows weekly hours plus days off.

The seed creates profiles and schedules; it does not connect PayPal accounts, invent payment readiness, create bookings, authorize holds or send notifications. Complete the normal seller connection before filming. Repeating the seed is not a destructive reset: begin each rehearsal with a new request. Do not reuse an old paid job as evidence for a new transaction.

### Refresh access for preparation from a Mac

The Render Blueprint deliberately makes `samenklaar-db` private (`ipAllowList: []`). The local `.env.production` also contains a private/container database hostname. A failed local connection is not evidence that the running production database needs a new password. The saved Auth0 import token was separately confirmed expired on 26 October 2026.

1. Open Render → **samenklaar-db** → **Connect**, or the connection details on its **Info** page. Copy the **External Database URL** into ignored `.secrets/local/presentation.env` as `DATABASE_URL=...`. Keep TLS enabled for the external connection. Preserve the running service's internal database configuration.
2. For preparation from this Mac, open the database's **Networking / Access Control** section and temporarily allow only the Mac's current public IPv4 address with `/32`, preserving any existing rules. Remove that temporary rule when preparation is finished. A job running inside Render's network can instead use the internal URL without changing external access. See [Render's connection and access-control documentation](https://render.com/docs/postgresql-creating-connecting).
3. Open Auth0 → select the existing production tenant → **Applications → APIs → Auth0 Management API → API Explorer**. Obtain a fresh short-lived token for this one-off import using the existing authorized import application. Replace `AUTH0_MANAGEMENT_TOKEN` in ignored `.secrets/local/seed.env`. If sign-in uses a custom domain, set `AUTH0_MANAGEMENT_URL` there to the canonical tenant Management API URL (`https://<tenant-domain>/api/v2`); keep `AUTH0_ISSUER_URL` unchanged so account identities still match sign-in. Keep `AUTH0_CONNECTION_ID` and `SEED_PASSWORD` unchanged. The importer reads connection settings and creates/polls the import job; its token needs `read:connections`, `create:users` and `read:users`. See [Auth0's migration guide](https://auth0.com/blog/how-to-migrate-users-to-auth0-a-technical-guide/). Do not change the normal login application's client secret or callback URL.

The local runner must load the external `DATABASE_URL` from `presentation.env` into its process environment before loading `.env.production`, because the application's config loader intentionally preserves existing process values. Do not paste either credential into chat or commit it. Recheck connectivity and token access before running the seed/import.

### Matched presentation images

The selected version-2 pair in local `.deploy/presentation-assets/sink-repair/` shows a **large open break in the old U-bend**, followed by a **new intact replacement U-bend** between the same existing pipes. Use `01-before-broken-trap-v2.png` for intake. Upload **both** that image and `02-after-replaced-trap-v2.png` as the professional's completion evidence: private customer intake photos are separate and are not sent to the evidence reviewer. The original drip/dry pair is superseded because absence of dripping did not establish a repair. Both selected images were generated with the built-in image tool; the second edits the first to preserve scene continuity. The folder contains provenance, exact prompts and a matching work-note template. Noor's initial published materials policy includes one replacement PVC trap section and standard seals in the €110 total; other parts still need separate approval. Confirm this inclusion in the selected agreement before booking. Disclose that these are generated presentation fixtures. They do not demonstrate a real repair or water test. One deployed sandbox walkthrough reached delegated completion approval and payment on 27 October, after a completion-check retry; consistent clean rehearsals are still required. The template leaves real test observations blank rather than inventing them. Keep normal evidence and payment gates unchanged.

## PayPal preparation

Use the existing platform sandbox REST application and two separate wallets:

1. A **Personal** buyer account, Netherlands, EUR, with enough sandbox balance for several rehearsals (for example €5,000).
2. A **Business** receiving account, Netherlands, EUR, connected through Noor's SamenKlaar profile. Keep this wallet separate from both the buyer and the platform's API merchant.

Create them in Developer Dashboard → Sandbox → Accounts. PayPal documents [Personal accounts as buyers and Business accounts as merchants](https://developer.paypal.com/sandbox-testing/accounts). The receiving PayPal email may differ from the SamenKlaar sign-in email. A pre-created Business sandbox wallet avoids the account-upgrade detour during preparation; it does not imply every real professional must own a registered company.

Only Noor needs a receiving wallet for this presentation. The other nine profiles support service comparison and scheduling scenarios; they are not all payment-ready. Finish PayPal onboarding, email confirmation and receiving-permission checks before recording. Keep generated passwords out of chat, screenshots, the public repository and the video. The product URL is production; money movement remains **PayPal sandbox**.

## Recording plan: 2 minutes 50 seconds

One story: **a leaking sink becomes an agreed visit, documented completion and a verified PayPal payment**. Use English narration, actual browser footage and readable captions. Record one job consistently across both roles, with logins, professional setup and onboarding completed beforehand.

| Time | On-screen action | Voice-over draft |
| --- | --- | --- |
| 00:00–00:12 | Customer request screen; brief product title | A leaking sink should not become a week of messages. SamenKlaar brings finding a professional, agreeing the work and paying for it into one conversation. |
| 00:12–00:36 | Describe the broken trap in ordinary words; show the before photo; answer intake; choose the canonical Amsterdam location | Alex has a broken waste-pipe section beneath the kitchen sink. The assistant asks what it needs to understand the job, then finds professionals serving the area. |
| 00:36–01:00 | Compare visible candidates; open Noor's scope, materials and exclusions; make a bounded counteroffer | The choice is more than a price. Alex can compare the work included, materials, exclusions and available visits. Noor's assistant can negotiate within Noor's own limits. Alex still chooses the professional and agrees the offer. |
| 01:00–01:26 | Explicit completion/payment delegation; choose visit; actual PayPal checkout and authorized hold | Alex chooses whether to review the finished work personally or delegate that review and payment release. PayPal authorizes the agreed amount now. The appointment must wait for that authorization; payment has not been captured yet. |
| 01:26–01:50 | “After the visit” transition; professional work note and relevant before/after photos; pre-submission check | After the visit, Noor records what was repaired and uploads the supporting photos. The assistant checks this documentation before it reaches Alex. Missing or unclear proof goes back to the professional for correction. |
| 01:50–02:20 | Actual documented completion review; delegated approval and capture, if the evidence passes | When the documentation supports the agreed work, the authorized assistant can approve completion and release the existing PayPal hold. Unclear evidence still needs a person. The system records who approved, what was paid and the payment reference. |
| 02:20–02:38 | Customer finished report; professional paid outcome and earnings | Both sides now have the same record: the agreed scope, visit, photos, work note, approvals and payment. Alex can see the finished job, and Noor can see the paid result. |
| 02:38–02:50 | End card with product and URL | Two assistants coordinate the work. People set the limits. PayPal connects the agreement to the payment. SamenKlaar: from request to done. |

Rehearse against the actual UI before locking narration. Keep the recorded amount consistent if negotiation changes it. The seed uses 24-hour notice and weekly availability: arrange the visit and capture schedule beforehand, then show an honest time jump rather than changing server time or bypassing visit rules.

The positive completion sequence needs relevant, consistent photos of the same sink joint and a specific factual work note. Obtain consent for a staged repair and retain image-use rights. Unrelated stock sink photos cannot demonstrate that this work was performed. Do not weaken the evidence checker to make the story pass. Do not narrate autonomous release as successful unless the real provider review and sandbox capture both succeed.

Record clean customer and professional browser sessions at a readable desktop size. Remove unrelated tabs, notifications and private autofill suggestions. Use restrained role labels and zooms, an English voice-over and subtitles. The video should identify the transaction as a sandbox demonstration without adding test-mode banners to the product. Preserve the raw takes locally for revisions. Deliver the first cut as a 1080p MP4 plus SRT captions and this script; review that cut before publishing.

The final approved narration voice is **`01-british-aristocratic-v2.wav`**, an original designed male voice with polished aristocratic British diction and Regency-drama delivery. The owner selected this version as final; do not substitute Charon or the earlier auditions. Reuse the designed voice identifier and delivery settings saved locally in ignored `.deploy/demo-artifacts/voice-options/british-aristocratic-voice.json` and `01-british-aristocratic-v2.json`. No customer content is sent for voice generation.

## Readiness gate

Before recording, finish three consecutive clean walkthroughs through the deployed build. Confirm location selection works on the first attempt, candidate review survives reload, counteroffers do not auto-book, the explicit delegation choice persists, and PayPal authorization gates booking. Then verify the professional documentation check, actual completion approval, one capture to the intended receiver, the finished report and reload persistence. Check customer receipt, professional earnings and PayPal transaction state agree. Test one unclear-evidence case separately so the human fallback is known to work.

## Submission

The [official hackathon rules](https://paypalaihackathon.devpost.com/rules), checked 26 October 2026, require a working PayPal-and-AI project, a public open-source repository, English materials or translations, and actual product footage in a publicly visible YouTube video under three minutes. Submit by **12 November 2026, 21:00 Amsterdam time**. Keep the working project available through the judging period ending **15 December 2026**. The repository already contains a license. Prepare a clear project description, private judge-access instructions and screenshots alongside the video. Never put provider keys into public instructions.
