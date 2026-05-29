# Next steps / issues at hand

## Current status

The pre-checkout cart/search hardening agenda is complete.

## MVP readiness checklist

Customer-facing MVP gaps:

- Add customer order APIs:
  - `GET /v1/orders`;
  - `GET /v1/orders/{id}`.
- Add a checkout expiry worker:
  - expire stale active checkout sessions;
  - release reserved inventory automatically.
- Keep manual payment for MVP, but wrap it as an explicit manual/dev payment provider so payment assumptions do not leak across the service boundary.
- Complete Swagger coverage for the customer-facing flow.
- Add basic public API hardening:
  - rate limit login;
  - rate limit registration;
  - rate limit checkout/payment confirmation if exposed publicly;
  - add a CORS allowlist before browser clients connect directly.
- Add an end-to-end customer flow test:
  - browse product;
  - add to cart;
  - checkout;
  - pay manually;
  - verify the order is visible in customer order list/detail.
- Add buyer-facing price-change signaling before checkout UX is finalized.
- Replace placeholder order numbers (`ord-` + UUID) with a support-friendly order-number scheme.
- Document checkout idempotency behavior in Swagger, especially replay behavior for terminal checkout sessions.
- Add order status history writes inside checkout/payment/order transitions:
  - order created as `pending_payment`;
  - payment confirmation moves order to `placed` / `paid`;
  - checkout cancellation moves order to `cancelled`;
  - checkout expiry moves order to `expired`.
- Enforce active buyer organization checks on buyer checkout/cart/payment routes before public usage.
- Clean up buyer-facing checkout/payment response shape:
  - hide internal inventory reservation IDs/details from customer responses;
  - keep reservation internals for merchant/admin/internal APIs.
- Add minimal checkout/payment/order observability:
  - structured logs around key transitions;
  - checkout/session/order/payment IDs in logs;
  - clear logs for inventory/payment/state failures.
- Cap the `Idempotency-Key` header length on `/checkout` before it reaches the DB (currently `strings.TrimSpace` then passed through unbounded).
- Reduce cart write amplification: `CreateCart` uses `ON CONFLICT DO UPDATE SET buyer_org_id = EXCLUDED.buyer_org_id` purely to force `RETURNING *`, but the no-op write still fires `trg_carts_updated_at` on every fetch. Switch to `ON CONFLICT DO NOTHING` + `SELECT` fallback.
- Tighten `SetCartShopGroupSelected` to one DB roundtrip after the mutation instead of `GetCartByBuyerOrgID` + `GetCartDetails` followed by an in-memory group scan.

Merchant-facing MVP gaps:

- Add merchant order APIs:
  - `GET /v1/merchant/orders`;
  - `GET /v1/merchant/orders/{id}`.
- Add merchant order status transition APIs:
  - start with a simple fulfillment flow such as `pending -> processing -> shipped -> delivered`;
  - define cancellation rules before shipping.
- Expose merchant order item details:
  - product and variant snapshots;
  - quantity;
  - buyer/customer snapshot;
  - payment status;
  - fulfillment status.
- Add inventory adjustment APIs:
  - manual stock correction;
  - optionally add stock movement reasons/audit trail later.
- Add merchant dashboard basics:
  - pending order count;
  - low-stock list;
  - recent orders.
- Complete Swagger coverage for merchant-facing APIs.
- Add merchant-side end-to-end test:
  - create product;
  - set inventory;
  - customer checkout;
  - merchant sees order;
  - merchant updates fulfillment status.

Platform-user MVP gaps:

- Add platform organization management APIs:
  - list organizations;
  - view organization details;
  - suspend/reactivate organizations if needed for MVP operations.
- Add platform user/member management APIs:
  - list platform users;
  - invite/create platform users;
  - assign/update platform roles;
  - deactivate platform users.
- Add platform merchant review/moderation basics:
  - view merchant organizations;
  - inspect merchant catalog status;
  - hide/suspend problematic products or merchants if required.
- Add platform order support lookup:
  - search orders by order number, customer email, merchant, or checkout/session ID;
  - view order/payment/reservation state for support debugging.
- Add platform customer support lookup:
  - search customers by email;
  - view customer organizations/memberships and recent orders.
- Add operational inventory/order visibility:
  - find failed checkouts;
  - find stale reserved checkout sessions;
  - find negative or invalid inventory states.
- Complete Swagger coverage for platform-facing APIs.
- Add platform-side authorization tests:
  - platform admin can access platform routes;
  - merchant/customer users cannot access platform routes;
  - non-admin platform roles only get allowed capabilities once roles are split.

## Next major workstream: checkout/order system

Before coding checkout/orders, define:

- order schema;
- order item snapshot fields;
- checkout validation rules;
- selected cart item behavior;
- inventory reservation/decrement strategy;
- when stock is reserved;
- whether carts reserve stock;
- reservation expiry behavior;
- insufficient stock behavior;
- single-warehouse vs multi-warehouse fulfillment;
- when orders are created relative to payment;
- idempotency model for checkout;
- merchant/shop-group split order behavior;
- post-checkout cart cleanup.

Customer-facing schema work already has customers and carts/cart items. Still add or review:

- orders and order items;
- checkout/reservation tables;
- customer addresses/shipping tables, if customer-owned shipping addresses are needed.

Reservation implementation should include:

- an inventory reservations table or equivalent persisted reservation record;
- an atomic stock reservation query using `SELECT ... FOR UPDATE` or an equivalent guarded update;
- release/expire reservation behavior;
- order creation from reservation;
- tests for concurrency, insufficient stock, expiry, and tenant boundaries;
- a decision on whether recovery/expiry jobs are needed in the MVP.

Deferred checkout/order follow-ups:

- Replace placeholder order numbers (`ord-` + UUID) with an operationally friendly order-number scheme before customer/support workflows rely on them.
- Decide and document strict idempotency semantics for terminal checkout sessions. Current direction: a replay with the same idempotency key returns the original checkout, even if it was later cancelled or expired.
- Add a buyer-facing price-change signal before checkout UX is finalized. Checkout currently snapshots `CurrentUnitPrice`, so a cart price change between add-to-cart and checkout is not explicitly surfaced.
- Add warehouse fallback in `CheckoutSelectedCartItemsTx`. `ListInventoryCandidatesForCheckoutItem` is currently called with `PageLimit: 1`; if the chosen warehouse loses a concurrent race, the whole checkout aborts even if another warehouse for the same merchant could fulfill. Fetch top-N candidates and try each before failing with `ErrInsufficientInventory`.
- Differentiate failure modes in `ReserveInventoryForCheckout`. Today the query returns no rows (mapped to `ErrInsufficientInventory`) for any of: variant not in org, warehouse not in org, inventory inactive, or insufficient stock. Split the tenant/active checks from the stock check so logs can distinguish them.
- Move `defaultCheckoutReservationTTL` (currently a 30-minute hardcoded const in `tx_checkout.go`) into configuration, with per-merchant override as a future option.
- Batch `ReleaseReservedInventory` calls in `releaseCheckoutReservations` instead of looping one roundtrip per reservation item. Fine until carts grow large; address before B2B-sized carts.
- Plan a separate webhook route group with HMAC verification before adding any non-manual payment provider. Do not reuse the customer-session-protected `/payments/{id}/confirm` handler for provider callbacks.
- Surface cancel-and-replace explicitly in the checkout response. When a buyer POSTs `/checkout` and an active session with a different fingerprint is silently cancelled in favor of the new selection, the response should signal that the prior reservation was released.

## Future async task service candidates: `hibiken/asynq`

Use Asynq for retryable side effects and expensive background work. Keep correctness-critical state transitions inside DB transactions.

Good candidates:

- Search document rebuilds, especially fan-out cases like category rename, attribute value/label update, bulk import/update, and full reindex.
  - Category/attribute trigger fan-out is not a current bug, but it is the main search-document scale caution.
  - Category rename can rebuild many product search documents.
  - Attribute value/label rename can rebuild many product search documents.
  - Today this happens synchronously inside the DB transaction.
  - Acceptable at current catalog size; later, move this to an async queue with Asynq.
- S3/object cleanup: temp upload deletion, orphaned final asset deletion, unused asset cleanup after product updates.
- Video processing: eventually move ffmpeg transcode out of request path and expose upload/job status.
- Order/checkout side effects after the checkout transaction commits:
  - order confirmation email;
  - merchant notifications;
  - invoice/receipt generation;
  - analytics events;
  - non-critical cart cleanup;
  - payment webhook reconciliation jobs.
- Checkout/session expiry jobs:
  - enqueue or schedule an Asynq worker that finds stale active checkout sessions and calls the checkout reservation expiry transaction;
  - expiry is implemented as a transaction/service primitive, but no background worker currently invokes it.
  - before wiring the worker, ensure the expiry path only expires sessions whose `expires_at <= NOW()`.
- Inventory/background reconciliation:
  - release expired reservations;
  - detect invalid/negative inventory states;
  - low-stock notifications.
- Email delivery:
  - welcome emails;
  - password reset;
  - order confirmation;
  - merchant/admin notifications.
- Idempotency/cache cleanup if more idempotency state is persisted later.
- Admin maintenance jobs:
  - rebuild all product search documents;
  - reprocess product assets;
  - recalculate derived counters/totals if added later.

Do not queue yet:

- cart item add/update/remove;
- normal product create/update DB transaction;
- auth/session renewal;
- request-critical checkout validation;
- anything the client must know succeeded before returning `200`/`201`.

## Storefront search API improvements

Current storefront search is available through:

```http
GET /v1/products?q=<search-term>&limit=20
GET /v1/products?q=<search-term>&limit=20&cursor=<nextCursor>
```

`product_search_documents` is intentionally internal and should not be exposed directly. The storefront should search through `GET /v1/products?q=...`, which returns normal product response objects.

Future improvements for richer storefront search:

- category filter;
- price range filter;
- merchant/shop filter;
- featured filter;
- sort options;
- autocomplete/typeahead endpoint;
- search suggestions;
- facets/filter counts;
- search result highlighting/snippets if needed;
- search analytics events once observability/analytics exists.

## Deferred tasks from `CLAUDE.md`

These are intentionally deferred and should be revisited at the trigger points below.

| Area | Decision | When to revisit |
|---|---|---|
| Rate limiting | Not implemented yet. Preferred library: `go-chi/httprate`. Initial targets: login endpoints per IP, registration endpoints per IP/email. | When the core feature set stabilizes and abuse patterns become observable. |
| Email enumeration via registration `409` | Registration currently returns `409` if email is taken. This is acceptable for now and should be mitigated with rate limiting. | Same time as rate limiting. |
| Cookie support | API currently uses Bearer tokens only. BFF may need HttpOnly cookies for web clients. | When BFF is built. |
| CSRF protection | No CSRF middleware today. Safe while auth uses Bearer tokens, but critical if cookie auth is introduced. | Before cookie-based auth is introduced. |
| CORS explicit origin allowlist | No CORS middleware today. Also expose `X-Session-Expires-At` via `Access-Control-Expose-Headers` when browser clients need it. | Before browser clients connect directly to this API. |
| Search evolution | PostgreSQL FTS exists now; Meilisearch/Elasticsearch remains a future option if search needs outgrow Postgres. | When storefront search requirements exceed current FTS capabilities. |
| Observability stack | Current app has structured `slog` + request logging, but no full metrics/tracing/dashboard stack. Plan Prometheus metrics, OpenTelemetry traces, Grafana dashboards, and Jaeger or Tempo for trace storage. Checkout/order should be the first major flow instrumented with logs, metrics, and spans. | During or immediately after checkout/order MVP, before production readiness. |
| Multi-org support | Current sessions store `active_organization_id`. Future design should use per-request `X-Organization-Id`, validate membership per request, cache membership checks, and keep session org as fallback/last-used hint. Do not build a DB-write “set active org” endpoint. | When multi-org merchant accounts are supported. |
| RLS activation | RLS policies exist (customer + merchant schemas, with `FORCE ROW LEVEL SECURITY`), but the `ApplyRLSContext` middleware is not installed in `route.go`, so `applyRLS` is a no-op and the app runs as `app_system` / `BYPASSRLS`. Activating RLS requires wiring the middleware into the buyer/merchant route groups. Before flipping it on, benchmark `rlsDBTX`'s per-query `BEGIN` / `SET LOCAL ROLE` / `COMMIT` overhead — every non-tx query becomes two extra roundtrips when RLS context is present. | Before production multi-tenant data is live. |

## Later cleanup

- Finish Swagger annotations for any handlers not yet covered, using Swagger as the API documentation source.
- Revisit package/type renames as a deliberate cleanup pass, if the current names still feel unclear after checkout/order work.
- Revisit handler interfaces that mostly mirror one concrete implementation; keep concrete constructors unless multiple implementations or mocks are needed.
- Run a broader coverage review once checkout/order/customer-facing flows are in place.
- Remove or wire up unused sqlc queries: `CountSelectedCartItemsForCheckout` (`db/query/cart_checkout.sql`) and `CancelActiveCheckoutSessionsForBuyer` (`db/query/checkout_session.sql`). Both are generated but have no Go callers (`tx_checkout` uses the per-session release path instead).
- Confirm the storefront product path shape `GET /v1/products/{org_id}/{slug_or_id}` matches the intended storefront model. The current path forces the frontend to know the merchant org before fetching a product detail, which precludes a platform-wide aggregated storefront without a separate lookup endpoint.
