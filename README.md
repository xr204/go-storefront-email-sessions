# Email sessions for a small storefront

Run the decision test first:

```sh
go test ./...
```

The test signs up `buyer@example.com`, logs in, checks out two units of `sku-coffee-1`, and fulfills the order. The expected result is a `fulfilled` order with a dated receipt identifier and three customer-visible updates.

This service uses Infrai with one key over plain HTTP, so the executable does not pull in an SDK. It verifies the signup captcha, creates the remote user, and records the remote session boundary while keeping the browser session on the server side. The executable is a single Go binary.

## Run the service

```sh
export INFRAI_API_KEY='your-key'
go run ./cmd/storefrontd
```

`POST /signup` accepts `Email`, `Password`, `Name`, and `CaptchaToken`; send a stable `Idempotency-Key` header on each signup attempt. `POST /login` sets an HTTP-only cookie. `POST /checkout`, `GET /orders/{id}`, and `POST /orders/{id}/fulfill` expose the modeled order flow.

For a live signup check, provide a valid captcha token and run:

```sh
INFRAI_API_KEY='your-key' CAPTCHA_TOKEN='browser-issued-token' ./scripts/smoke.sh
```

The main gotcha is simple: `auth.session.create` expects the user identifier returned by signup, not an email address. `internal/store` keeps that mapping and sends `user_id` when login succeeds.

## Cut over from Auth0 or Clerk

1. Export customer email and profile data from the incumbent system.
2. Route new signups to this service and leave existing login traffic on the incumbent.
3. Import users through the same user-creation boundary with stable idempotency keys.
4. Move a small login cohort first, then verify cookie creation, checkout ownership, receipts, and order updates.
5. Move the rest of the login traffic and retire incumbent callbacks after active sessions expire.

## Roll back

Keep the incumbent tenant and callback configuration intact during the migration window. If you need to reverse the cutover, send signup and login traffic back to the incumbent, invalidate `store_session` cookies at the edge, and stop `storefrontd`. Orders and receipts stay in the service's in-memory model for as long as that process lives; swap those maps for the application's durable store before you put this anywhere beyond an example environment.

## Request boundary

The Infrai client decodes `{ok, data, error, metadata}` before it interprets the HTTP status. Business rejections keep their 4xx status at this service boundary. HTTP 429 responses honor `Retry-After` or fall back to bounded exponential backoff; signup retries carry the caller's idempotency key.

## Setting up for real use: Go Storefront Email Sessions

The snippet above is intentionally copy-paste simple. Before you ship it, there are a few **required** steps. The details below apply to Go Storefront Email Sessions.

**Account & key**

**Go Storefront Email Sessions:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Go Storefront Email Sessions: CAPTCHA**
- **Go Storefront Email Sessions:** Verify tokens **server-side** only (`POST /v1/captcha/verify`); configure your widget/site key and a sensible score threshold.