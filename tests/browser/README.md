# Browser regression fixtures

Run `npm exec vite -- --host 127.0.0.1 --port 5191 --strictPort`, then open:

- `http://127.0.0.1:5191/tests/browser/regressions.html`: mounts the real plan and agreed-ledger components with two identically labelled price adjustments. Both adjustments must render in both views, with a PASS message and no `each_key_duplicate` error. This was reproduced red before removing the duplicate row keys.
- `http://127.0.0.1:5191/tests/browser/profile.html`: mounts the real professional editor, customer profile, availability page and waitlist dialogs. Its fetch boundary uses synthetic local fixtures. It does not use real accounts, provider credentials or an application database.

Check the professional profile can save an existing lowercase currency through the dropdown. Save customer contact details and revisit the tab to check the synthetic value persists. Check availability offers weekly hours and days off without individual time controls. On the waitlist tab, open the delete dialog, cancel it, then confirm deletion of the disposable fixture and check its PASS message. Inspect desktop and phone widths.

These are manual render/interaction checks, not automated end-to-end tests. `npm run check` includes their Svelte and TypeScript contracts. The production Vite entry does not include these pages. Persistence, account boundaries, canonical locations, booking preservation and request deletion are covered by the Go HTTP/PostgreSQL tests.
