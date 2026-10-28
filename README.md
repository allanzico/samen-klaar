# SamenKlaar

[Product](https://samenklaar.com) · [MIT license](LICENSE) · [Setup](docs/SETUP.md) · [PayPal integration](docs/PAYMENTS.md)

SamenKlaar connects customers with home-service professionals and keeps the request, price negotiation, booking, repair evidence and payment in one conversation. Each person has an assistant that acts within their permissions and asks when a decision needs them.

## The complete journey

1. Describe the problem, attach a photo and search by address, postcode or city.
2. Compare nearby professionals, service conditions, prices and available visits.
3. Negotiate a price, confirm the professional and choose a visit.
4. Approve the agreed amount in **PayPal**. A booking-time authorization holds the amount without paying the professional yet.
5. The professional submits repair photos and a specific work note. The documentation check requests corrections before sending an incomplete record to the customer.
6. The customer reviews completion and payment, or explicitly delegates both for that booking. A successful delegated review captures the buyer-approved PayPal authorization; unclear evidence requires human follow-up.

The application uses real PayPal sandbox authorization/capture, seller onboarding and webhooks. Live-money payments are disabled. The presentation uses fictional people, generated repair photos and sandbox wallets; it does not document a real repair. Photos cannot certify workmanship or safety.

## Run locally

Requirements: Docker with Compose, Make, OpenSSL and curl. Go, Node.js and PostgreSQL run in containers. A full external-provider journey needs your own Auth0, AI provider, Cloudflare R2, Mapbox and PayPal sandbox settings.

```sh
git clone https://github.com/allanzico/samen-klaar.git
cd samen-klaar
cp .env.example .env
make init-local
# Fill the provider settings in .secrets/local/app.env.
make deploy-local
```

Open [localhost:8090](http://localhost:8090). Follow [the setup guide](docs/SETUP.md) for provider callbacks, storage, accounts and sample data. Stop with `make stop-local`; data is retained. Deployment instructions for Render and a Docker host are in [DEPLOYMENT.md](docs/DEPLOYMENT.md).

## Verify without provider credentials

```sh
make check-flow
```

This builds the frontend and backend, runs Go tests/vet, frontend tests and type checks, then exercises persisted HTTP journeys against disposable PostgreSQL with provider fixtures. It does not load your credentials or move money. [Browser fixtures](tests/browser/README.md) cover interactive layouts separately.

## Source and assets

| Path | Contents |
| --- | --- |
| `src/` | Svelte 5 / TypeScript frontend, shared UI and state |
| `cmd/api/` | Go API and explicit operational commands |
| `internal/` | Workflow, agents, identity, provider adapters and seed data |
| `db/` | Schema and queries; versioned migrations live in `internal/adapters/postgres/migrations/` |
| `public/` | Logos and self-hosted fonts with their license notices |
| `assets/presentation/` | The generated repair-image pair, prompts, fictional work note, narration and captions |
| `docker/`, `render.yaml` | Reproducible build, local stack and deployment configuration |

Source and project-owned assets are distributed under the [MIT License](LICENSE); font notices are listed in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md). Provider accounts, credentials, customer uploads and private recordings are not distributed.

## Documentation

- [Product behaviour and current limits](docs/PRODUCT_SPEC.md)
- [Architecture](docs/ARCHITECTURE.md) and [assistant decisions](docs/AGENT_ARCHITECTURE.md)
- [PayPal payments](docs/PAYMENTS.md), [email](docs/EMAIL.md) and [design system](docs/DESIGN_SYSTEM.md)
- [Presentation and reproducible fixtures](docs/PRESENTATION.md)
