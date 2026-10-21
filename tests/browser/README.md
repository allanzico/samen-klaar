# Browser regression fixtures

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

Open `http://127.0.0.1:8098`. Start a leaking-sink request, answer intake and search for `1234 AB`. Select `1234 AB, Teststad`, then follow the assistant. Search for `Voorbeeldstraat 1`, choose the full synthetic address and confirm it. Approve the price if asked, give availability and select **At booking** before choosing a visit. Reload first to verify the timing selection persists. Choosing a time must open the loopback **Simulated PayPal approval** page. Its **Approve simulated hold** button returns through the production authorization callback; no external wallet or money is involved. Verify professional confirmation and reload persistence. At every question there should be one answer area and a collapsed **Plan and conversation**, with no separate manual booking form. Check phone and desktop widths and expand the supporting details to verify they remain accessible. Scroll over the conversation: `window.scrollY` must change while `#request-thread` has visible overflow and no bounded scroll height. The current question or confirmation card remains sticky. Minimize and reopen the question.

Stop the Go test with Ctrl-C, then stop the disposable database with `pg_ctl -D "$booking_data" stop`. Remove that temporary cluster when finished. This harness is opt-in and skipped during ordinary test runs; it is not an authentication mode in the application.

Verified 21 October 2026: the complete customer journey reached a professional-confirmed visit, survived reload, and produced no browser console warnings or errors. The address and visit questions were inspected at desktop and 390px phone width. `npm run test:format` covers the historical `Agreed on € NaN` regression, while Go tests cover canonical new amounts and recovery from ambiguous raw addresses.


Verified 21 October 2026 (booking hold regression): selecting `1234 AB` → `1234 AB, Teststad` and the full synthetic street address succeeded on the first attempt. **At booking** remained selected after reload. Choosing a visit opened the local approval page automatically; its return recorded a verified authorization and triggered professional confirmation. The database then showed `at_booking`, matching authorized/current scope, `confirmed`, `paid=false`, `status=authorized` and no capture ID. This verifies the application and provider protocol fixture, not an external PayPal wallet.

At 390 × 844 and desktop width, the expanded history used document scrolling with visible thread overflow and no nested conversation viewport. The active question stayed sticky; minimizing and reopening it worked. Browser console warnings/errors: none. Screenshots from the run are in `/private/tmp/samenklaar-hold-proof-20261021/`. Regression coverage includes stale poll failures after successful location/price actions, one checkout redirect for an assistant booking, saved timing through approval, refusal to forge an authorization, and recovery/capture of a single original hold only after distinct completion and payment approvals.
