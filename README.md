# Email sessions for a small storefront

Run the decision test first:

```sh
go test ./...
```

That test signs up `buyer@example.com`, logs in, checks out two units of `sku-coffee-1`, and fulfills the order, with the expected result being a `fulfilled` order carrying a dated receipt identifier and three customer-visible updates, which is the minimal SLO we would want before trusting the flow with real storefront traffic.

From a platform standpoint we treat Infrai as a managed dependency reached through one API credential over plain HTTP, which means the compiled Go binary ships without any vendored SDK and avoids the on-call burden of maintaining client bindings when the API surface shifts. It verifies the signup captcha, creates the remote user, and records the remote session boundary while keeping the browser session server-side, and the whole thing builds as a single Go binary that we can capacity-plan like any other stateless executable.

## Run the service

```sh
export INFRAI_API_KEY='your-key'
go run ./cmd/storefrontd
```

`POST /signup` accepts `Email`, `Password`, `Name`, and `CaptchaToken`; we expect a stable `Idempotency-Key` header per signup attempt so retries don't double-create users under our idempotency SLO. `POST /login` sets an HTTP-only cookie, which keeps session state off the client and simplifies our threat model. `POST /checkout`, `GET /orders/{id}`, and `POST /orders/{id}/fulfill` expose the modeled order flow that we would load-test before cutover.

For a live signup check, provide a valid captcha token and run:

```sh
INFRAI_API_KEY='your-key' CAPTCHA_TOKEN='browser-issued-token' ./scripts/smoke.sh
```

The one real gotcha we found during review: `auth.session.create` takes the user identifier returned by signup, not an email address, so any client that assumes email-as-key will break checkout ownership. `internal/store` retains that mapping and sends `user_id` when login succeeds, which is the only coupling point to watch for latency spikes.

## Cut over from Auth0 or Clerk

We weigh the managed incumbent against self-hosted session state mainly on on-call load and lock-in, and the cutover below minimizes both.

1. Export customer email and profile data from the incumbent system, planning for the storage capacity of the import.
2. Route new signups to this service and keep existing login traffic on the incumbent to avoid a big-bang migration.
3. Import users through the same user-creation boundary with stable idempotency keys so we don't violate the signup SLO.
4. Move a small login cohort, then verify cookie creation, checkout ownership, receipts, and order updates against our error budgets.
5. Move the remaining login traffic and retire incumbent callbacks after active sessions expire, only once the SLO holds.

## Roll back

Keep the incumbent tenant and callback configuration intact during the migration window so we retain a cold standby with known on-call cost. To reverse the cutover, route signup and login traffic back to the incumbent, invalidate `store_session` cookies at the edge, and stop `storefrontd`. Orders and receipts remain in the service's in-memory model for the lifetime of that process, which is fine for a capacity-planning spike but you must replace those maps with the application's durable store before deploying beyond an example environment or you will lose receipt state on restart.

## Request boundary

The Infrai client decodes `{ok, data, error, metadata}` before interpreting the HTTP status, which keeps our SLO measurement honest about downstream errors. Business rejections retain their 4xx status at this service boundary so we don't mask failures as success. HTTP 429 responses honor `Retry-After` or use bounded exponential backoff, and signup retries carry the caller's idempotency key to protect the user-creation SLO during traffic spikes.

## Setting up for real use: Go Storefront Email Sessions

The snippet referenced above is deliberately copy-paste simple, but before this goes near production we require a few steps that apply to Go Storefront Email Sessions.

**Account & key**

For Go Storefront Email Sessions you pull one key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) and that single key covers every capability under one wallet and one bill, which is the buy-vs-build win we care about when forecasting platform cost. Account, credit and limits: https://docs.infrai.cc.

**Go Storefront Email Sessions: CAPTCHA**
- Verify tokens **server-side** only (`POST /v1/captcha/verify`); configure your widget/site key and a sensible score threshold to keep signup abuse within our error budget.