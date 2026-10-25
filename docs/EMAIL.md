# Transactional email

The application uses SMTP. Keep credentials and real recipient addresses in ignored configuration, never in examples, tests or verification notes.

```dotenv
SMTP_ADDR=mail.example.test:2525
SMTP_USERNAME=example-user
SMTP_PASSWORD=replace-with-provider-password
SMTP_FROM=notifications@example.test
SMTP_FROM_NAME=SamenKlaar
```

`SMTP_FROM_NAME` is optional and defaults to SamenKlaar. `SMTP_FROM` can also contain a formatted sender address; the explicit name setting overrides its display name. Credentials must be provided together. Invalid endpoints, sender addresses and header-injecting names fail startup and `check-config`.

Port 465 uses implicit TLS. Other ports, including 587 and 2525, use STARTTLS. Remote relays must support encryption; only loopback test relays can run without it. There is no mailer selector or separate encryption variable.

When translating framework-style settings, combine `MAIL_HOST` and `MAIL_PORT` into `SMTP_ADDR`, rename username/password and sender address/name to the keys above, and remove `MAIL_MAILER` and `MAIL_ENCRYPTION` once they match this SMTP/TLS behavior. The runtime reads only the canonical `SMTP_*` keys.

Values are literal; use the actual display name rather than a framework expression such as `${APP_NAME}`. For Mailtrap live sending, use its documented `api` SMTP username and a sending token authorized for the sender domain. Sandbox credentials and domain permissions are separate.

## Send one diagnostic

With Go installed:

```sh
APP_CONFIG_FILE=.env.production go run ./cmd/api test-email recipient@example.test
```

The recipient can instead be supplied on stdin to keep it out of shell history and process arguments. The built production image supports the same `/api test-email [recipient]` command.

This sends one clearly labelled message through the application's SMTP adapter. It does not connect to the application database, run notification workers or send queued messages. Output acknowledges SMTP acceptance without printing credentials, recipient addresses or raw provider errors. Acceptance is not proof of inbox delivery: check the recipient inbox/spam folder and the provider's delivery log.

## Deploy

- Docker: set the canonical values in ignored `.env.production`; `make init-prod`/`make deploy-prod` import missing or empty mail settings into `.secrets/prod/app.env`. Existing nonempty deployment values stay authoritative and must be edited there when rotating credentials.
- Render: add these settings to the application's linked environment group and redeploy. Import only mail settings when other production settings are managed separately; never commit a credential file. SMTP settings stay server-side.
- Deploying a configured sender enables ordinary notification workers, including delivery of still-actionable queued notifications. The diagnostic command alone does not enable or drain that queue.

Use a live sending relay and an authorized sender domain for real inbox delivery. A sandbox relay captures messages in its own test inbox. SPF, DKIM and DMARC are configured with the mail provider/domain, not inferred by the application.
