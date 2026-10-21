# Browser regression fixtures

Run `npm exec vite -- --host 127.0.0.1 --port 5191 --strictPort`, then open:

- `http://127.0.0.1:5191/tests/browser/regressions.html`: mounts the real plan and agreed-ledger components with two identically labelled price adjustments. Both adjustments must render in both views, with a PASS message and no `each_key_duplicate` error. This was reproduced red before removing the duplicate row keys.
- `http://127.0.0.1:5191/tests/browser/profile.html`: mounts the real professional editor, customer profile, availability page and waitlist dialogs. Its fetch boundary uses synthetic local fixtures. It does not use real accounts, provider credentials or an application database.

Check the professional profile can save an existing lowercase currency through the dropdown. Save customer contact details and revisit the tab to check the synthetic value persists. Check availability offers weekly hours and days off without individual time controls. On the waitlist tab, open the delete dialog, cancel it, then confirm deletion of the disposable fixture and check its PASS message. Inspect desktop and phone widths.

These are manual render/interaction checks, not automated end-to-end tests. `npm run check` includes their Svelte and TypeScript contracts. The production Vite entry does not include these pages. Persistence, account boundaries, canonical locations, booking preservation and request deletion are covered by the Go HTTP/PostgreSQL tests.


## Real booking journey against disposable PostgreSQL

`TestBookingBrowser` serves the production build, HTTP handlers, matching, agent runner, weekly schedule and workflow store. The session, geocoder and AI are local fixtures. It never loads `.env` or calls real payment, geocoding or email services. Its database URL must use `127.0.0.1` and the database name `browser_booking`; the HTTP server binds only to loopback.

With PostgreSQL 16 binaries on your path, create an isolated cluster (use a free port):

```sh
booking_data=$(mktemp -d /tmp/samenklaar-booking.XXXXXX)
initdb -D "$booking_data" -A trust -U booking_test --no-locale -E UTF8
pg_ctl -D "$booking_data" -l "$booking_data/server.log" -o '-h 127.0.0.1 -p 55439 -k /tmp' start
createdb -h 127.0.0.1 -p 55439 -U booking_test browser_booking
npm run build
RUN_BOOKING_BROWSER=1 TEST_DATABASE_URL='postgres://booking_test@127.0.0.1:55439/browser_booking?sslmode=disable' go test ./internal/httpapi -run '^TestBookingBrowser$' -count=1 -v -timeout=30m
```

Open `http://127.0.0.1:8098`. Start a leaking-sink request, answer intake, choose Amsterdam, then follow the assistant. Search for Damrak to get two synthetic full-address candidates. Select Damrak 1, confirm it, approve the price, give availability and choose a visit. Verify professional confirmation and reload persistence. At every question there should be one answer area and a collapsed **Plan and conversation**, with no separate manual booking form. Check phone and desktop widths and expand the supporting details to verify they remain accessible.

Stop the Go test with Ctrl-C, then stop the disposable database with `pg_ctl -D "$booking_data" stop`. Remove that temporary cluster when finished. This harness is opt-in and skipped during ordinary test runs; it is not an authentication mode in the application.

Verified 21 October 2026: the complete customer journey reached a professional-confirmed visit, survived reload, and produced no browser console warnings or errors. The address and visit questions were inspected at desktop and 390px phone width. `npm run test:format` covers the historical `Agreed on € NaN` regression, while Go tests cover canonical new amounts and recovery from ambiguous raw addresses.
