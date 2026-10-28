# Recorded product walkthrough

The approved film is **2 minutes 58 seconds**. It follows one request through actual product screens: describe a broken sink trap, add a photo, find nearby professionals, negotiate €105, confirm the visit and assistant permission, authorize a PayPal sandbox hold, submit professional repair documentation, then review the finished report and captured payment.

The people and businesses are fictional. The generated repair-image pair, prompts, fictional work note, approved narration and English captions are included in [assets/presentation/](../assets/presentation/). No real repair or live-money settlement is claimed. Source footage, credentials, account sessions and private recordings are excluded from the public repository.

The customer is Alex de Vries (`alex-de-vries@samenklaar.test`); the receiving professional is Noor van Dijk / Helder Kraanwerk (`noor-van-dijk@samenklaar.test`). Those reserved sign-in addresses are separate from independently created PayPal sandbox wallets. Use your own credentials and distinct authenticated customer/professional browser sessions.

## Reproduce the scenario

Use [SETUP.md](SETUP.md) to run locally and seed ten comparable Amsterdam sink-repair professionals and a customer. The normal matching flow ranks that pool and presents its best results; no presentation-only matching shortcut is enabled. Professionals have different prices, materials policies, assistant limits, weekly hours and days off, defined in `internal/seed/demo.go`.

Use Amsterdam or `1071 XX` for discovery. `Museumstraat 1, Amsterdam` is a public geocoding example, not a claimed customer home or work site. Select the canonical Mapbox result. Check that the selected service includes the replacement trap and seals before agreeing a price.

Prepare one Personal sandbox buyer and a separate Business sandbox receiver; connect the receiver through the professional profile before recording. Use a funded buyer wallet and the platform app's sandbox configuration. Follow [PAYMENTS.md](PAYMENTS.md); the ordinary consent, authorization and evidence gates apply.

For an explicitly authorized hosted presentation, the seed CLI also requires `ENABLE_DEMO_SEED=1`, `EXPECTED_DEMO_ORIGIN` equal to the target HTTPS origin, and `SEED_CONFIG_FILE` / `SEED_OUTPUT_DIR` pointing to private files and an owner-only directory. Run `/api seed-demo`, then `/api import-seed-demo-auth0` using the release image with the intended app configuration. Never add these seed switches to the running web service. Repeated seeds preserve edited profiles and schedules; use a new request for a new recording instead of presenting an old paid job as a new transaction.

## Submission materials

The public repository must include the root license, source, static assets and run instructions. GitHub detects the root MIT license and displays it in the repository sidebar. The final YouTube upload should use the approved 2:58 film, English narration/captions and public visibility for judging. The upload description should link the product and public repository, explain PayPal's authorization/capture role, and disclose sandbox payments and generated repair images. Uploading and publishing the film are separate from preparing its local files.
