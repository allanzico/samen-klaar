# Browser regression fixtures

All examples below use fictional locations and test identities. Keep personal information from bug reports, chats, screenshots and live accounts out of repository fixtures and verification notes.

Run `npm exec vite -- --host 127.0.0.1 --port 5191 --strictPort`, then open:

- `http://127.0.0.1:5191/tests/browser/regressions.html`: mounts the real plan and agreed-ledger components with two identically labelled price adjustments. Both adjustments must render in both views, with a PASS message and no `each_key_duplicate` error. This was reproduced red before removing the duplicate row keys.
- `http://127.0.0.1:5191/tests/browser/profile.html`: mounts the real professional editor, customer profile, availability page and waitlist dialogs. Its fetch boundary uses synthetic local fixtures. It does not use real accounts, provider credentials or an application database.

Check the professional profile can save an existing lowercase currency through the dropdown. Save customer contact details and revisit the tab to check the synthetic value persists. Check availability offers weekly hours and days off without individual time controls. On the waitlist tab, open the delete dialog, cancel it, then confirm deletion of the disposable fixture and check its PASS message. Inspect desktop and phone widths.

These are manual render/interaction checks, not automated end-to-end tests. `npm run check` includes their Svelte and TypeScript contracts. The production Vite entry does not include these pages. Persistence, account boundaries, canonical locations, booking preservation and request deletion are covered by the Go HTTP/PostgreSQL tests.


## Real booking journey against disposable PostgreSQL

`TestBookingBrowser` serves the production build, HTTP handlers, matching, agent runner, weekly schedule and workflow store. The session, geocoder, AI and PayPal HTTP responses are local fixtures. It never loads `.env` or calls real payment, geocoding or email services. Its database URL must use `127.0.0.1` and the database name `browser_booking`; the HTTP server binds only to loopback.

With PostgreSQL 16 binaries on your path, create an isolated cluster (use a free port):

```sh
booking_data=$(mktemp -d /tmp/samenklaar-booking.XXXXXX)
initdb -D "$booking_data" -A trust -U booking_test --no-locale -E UTF8
pg_ctl -D "$booking_data" -l "$booking_data/server.log" -o '-h 127.0.0.1 -p 55439 -k /tmp' start
createdb -h 127.0.0.1 -p 55439 -U booking_test browser_booking
npm run build
RUN_BOOKING_BROWSER=1 TEST_DATABASE_URL='postgres://booking_test@127.0.0.1:55439/browser_booking?sslmode=disable' go test ./internal/httpapi -run '^TestBookingBrowser$' -count=1 -v -timeout=30m
```

Open `http://127.0.0.1:8098`. Start a leaking-sink request, answer intake and search for `1234 AB`. Select `1234 AB, Teststad`. Wait through background refreshes and reload: the professional results must remain visible, with no assigned job or address question. Inspect the service details, then press **Choose Fixture and continue**. Only then follow the assistant. Search for `Voorbeeldstraat 1`, choose the full synthetic address and confirm it. Approve the price if asked, give availability and select **At booking** before choosing a visit. Reload first to verify the timing selection persists. Choosing a time must open the loopback **Simulated PayPal approval** page. Its **Approve simulated hold** button returns through the production authorization callback; no external wallet or money is involved. Verify professional confirmation and reload persistence. At every question there should be one answer area and a collapsed **Plan and conversation**, with no separate manual booking form. Check phone and desktop widths and expand the supporting details to verify they remain accessible. Scroll over the conversation: `window.scrollY` must change while `#request-thread` has visible overflow and no bounded scroll height. The current question or confirmation card remains sticky. Minimize and reopen the question.

Stop the Go test with Ctrl-C, then stop the disposable database with `pg_ctl -D "$booking_data" stop`. Remove that temporary cluster when finished. This harness is opt-in and skipped during ordinary test runs; it is not an authentication mode in the application.

Verified 21 October 2026: the complete customer journey reached a professional-confirmed visit, survived reload, and produced no browser console warnings or errors. The address and visit questions were inspected at desktop and 390px phone width. `npm run test:format` covers the historical `Agreed on € NaN` regression, while Go tests cover canonical new amounts and recovery from ambiguous raw addresses.


Verified 21 October 2026 (booking hold regression): postcode/city selection and full-address selection succeeded on the first attempt. **At booking** remained selected after reload. Choosing a visit opened the local approval page automatically; its return recorded a verified authorization and triggered professional confirmation. The database then showed `at_booking`, matching authorized/current scope, `confirmed`, `paid=false`, `status=authorized` and no capture ID. This verifies the application and provider protocol fixture, not an external PayPal wallet.

At 390 × 844 and desktop width, the expanded history used document scrolling with visible thread overflow and no nested conversation viewport. The active question stayed sticky; minimizing and reopening it worked. Browser console warnings/errors: none. Screenshots are temporary local artifacts; do not retain captures of real accounts or personal locations as test evidence. Regression coverage includes stale poll failures after successful location/price actions, one checkout redirect for an assistant booking, saved timing through approval, refusal to forge an authorization, and recovery/capture of a single original hold only after distinct completion and payment approvals.


Verified 21 October 2026 (professional confirmation): repeated agent runs with saved address, budget and automatic visit booking leave the request unassigned. The browser stays on the professional’s service, price and details through background updates and reload; **Choose Fixture and continue** is the explicit selection. Only after clicking it does the address question appear. Before plan approval, **Request options → Choose another professional** returns to review. Returning clears both parties’ old active assistant questions, and old persisted questions cannot interrupt an unassigned request. The workflow rejects assistant-originated professional selection.

## Professional dashboard

Open `http://127.0.0.1:5191/tests/browser/dashboard.html` on the fixture Vite server. It renders the real professional jobs page at desktop and 390px phone widths with synthetic data; the phone view uses Dutch. Check separate EUR/USD earnings, pending payments and visit/job counts. The normal, empty and error controls switch fixtures. Error simulates one failed stats request; Retry restores the overview while the jobs list remains available. This fixture contacts no payment, email or AI provider.

Verified 25 October 2026: English desktop and Dutch 390px phone layouts render the separate currency totals and work counts. A failed refresh leaves the jobs list and previous totals visible, Retry restores the overview, and the empty state shows zero totals. Browser console warnings/errors: none. Fixtures use synthetic data only.

## Booking autonomy

Serve Vite on loopback and open `/tests/browser/autonomy.html`. This fixture mounts the real request page with synthetic data and local session persistence; it calls no payment or AI provider. Check that neither review mode starts selected, automatic mode shows the exact amount and booking hold explanation, reload retains the choice, booking displays the turn-off control, and turning it off restores manual review. Check the provider-unavailable scenario and `?nl` at a phone width. Initial implementation could not use the browser extension; native Chrome inspection on 26 October now covers the desktop and embedded mobile layout described below.

The fixture now starts at price/options review. `?mobile-check` embeds the real request page at 390px in Dutch. Check that the progress bar and compact action strip stay visible on scroll while the radio choices and amount scroll away; Make a price offer opens the offer form. Desktop and embedded mobile scrolling were inspected with Chrome computer use on 26 October 2026. The fixture remains synthetic and makes no provider/payment calls.

The `?scenario=professional` view checks the gap between the review explanation and service card, with a transparent floating wrapper and no gray shadow. `?scenario=offer` shows a price offer awaiting the professional personally; Change my offer explains the potential wait before sending. `?scenario=checking` shows automatic completion review without an Approve action, while `?scenario=unclear` puts corrections with the professional. Combine a scenario with `mobile-check` for the Dutch phone layout. Desktop states and 390px professional-review scrolling were inspected in native Chrome on 26 October 2026. No live job or payment was changed during these checks.

## Professional completion check

Open `/tests/browser/completion.html` on the loopback Vite server. Its synthetic saved note is intentionally vague. Check and send must show correction guidance while keeping the form editable. Add a truthful fictional repair/check description to the note and check again; this fixture simulates a passed check and shows Sent for approval. Improve photos or work note reopens that record. `?mobile-check` embeds the Dutch 390px view, `?offline` simulates a check outage, and `?customer` shows the customer waiting while the professional improves the record. The mock assessment is only a visual fixture; real HTTP/storage, privacy, race handling and payment behavior are tested separately. Native Chrome inspected desktop correction guidance and mobile note correction/resubmission on 26 October 2026. No real files or external services are used.

For the finished-job report, use `?paid` (professional) or `?paid&customer` (customer), with `&mobile-check` for the Dutch 390px layout. The saved note and capture reference are explicitly synthetic; no file is uploaded and no provider is called. The accepted note should be visible without opening history, with earlier documentation collapsed separately. Check the agreed service and amount, separate completion/payment-approval details and the payment reference. Real stage-rendering regressions also cover refunds, missing legacy fields and Dutch copy through `npm run test:interaction`.
