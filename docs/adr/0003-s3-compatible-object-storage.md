# ADR-0003: S3-Compatible Object Storage in an EU Region

**Status**: Accepted
**Date**: 2026-06-16

## Context

Paddock stores large binary assets: original raw camera files (20–50 MB each), web-preview JPEGs, watermarked renditions, per-tier resolution variants, and ZIP bundles. Storage must satisfy several constraints simultaneously:

- **EU data residency**: the brief mandates EU hosting as a default; the compliance and legal persona explicitly evaluates data-residency posture before approving procurement.
- **Durability and availability**: originals are irreplaceable; the platform's audit promises are worthless if assets are lost.
- **Egress cost**: Paddock is a media-delivery product; sponsors and journalists download high-resolution files. Egress is a direct cost of revenue.
- **Local development and CI**: engineers and CI pipelines need object storage without depending on a live cloud account.
- **API portability**: the team must be able to switch storage providers without rewriting application code.

## Decision

We will code against the **S3 API** via `aws-sdk-go-v2`, targeting an EU-resident S3-compatible store in production, and a **MinIO container** in development and CI.

Production provider candidates (evaluated at deployment time, not locked in code):

| Provider | EU region | Notes |
|---|---|---|
| Hetzner Object Storage | EU (Falkenstein/Helsinki) | Cheap egress, EU-only |
| Scaleway Object Storage | EU (Paris/Amsterdam) | EU-resident, competitive pricing |
| Cloudflare R2 (EU jurisdiction) | EU bucket jurisdiction | Zero egress fees |
| AWS S3 `eu-*` regions | EU | Highest durability, highest egress cost |

The application layer sees only the S3 API; the endpoint URL and credentials are injected via environment variables. Switching providers requires no code change.

## Alternatives Considered

### AWS S3 with us-east-1 or unspecified region (lock-in)

**Pros:**
- Industry-standard durability (11 nines).
- Deepest ecosystem (Lambda triggers, Transfer Family, CloudFront).
- No ambiguity about API compatibility.

**Cons:**
- US-region default violates the EU data-residency requirement.
- AWS S3 egress pricing ($0.09–$0.085/GB) is high for a media-delivery product where downloads are the primary value exchange; at MVP burst targets this is the dominant variable cost.
- Locks the application and its data to a single vendor; switching requires data migration and contract renegotiation.

**Why rejected**: EU residency is non-negotiable, and egress economics are load-bearing for the business model. Portability protects both.

### Self-hosted MinIO in production

**Pros:**
- Lowest per-GB cost (only infrastructure cost).
- Complete data-residency control.
- Identical API to development environment.

**Cons:**
- Durability and availability depend entirely on the team's operational practice: erasure coding configuration, backup cadence, disk failure response, hardware procurement.
- At MVP team size, the operational burden of running a durable, highly available distributed object store is disproportionate; originals are irreplaceable and a storage failure is a catastrophic product failure.
- Does not change the egress cost profile unless a CDN is placed in front, which is an additional operational layer.

**Why rejected**: The MVP team should not own the durability and availability SLA of the object store. That is precisely what managed S3-compatible services provide. MinIO is appropriate in dev/CI where data loss is acceptable; it is not appropriate as the production tier for irreplaceable originals.

### Proprietary storage SDK (e.g., GCS client library)

**Pros:**
- Native client with full feature access for a specific provider.

**Cons:**
- Code is coupled to one provider's API surface; any migration requires application changes.
- GCS is not S3-compatible, adding a second API surface to learn and test.

**Why rejected**: The S3 API is the de facto standard for object storage and is supported by every credible EU-resident provider. Using a proprietary SDK would sacrifice portability for no benefit.

## Consequences

### Positive

- EU data residency is enforced by provider selection, not by application code; the compliance posture is structural.
- The application can be moved between S3-compatible providers (or to a cheaper provider as volume grows) by changing environment variables, with no code change and no data-model migration.
- Development and CI use an identical API (MinIO container) to production; there is no "works in prod only" surface area in storage code.
- Egress cost is managed at the infrastructure layer; provider selection can optimize for egress economics independently of application development.

### Negative

- S3-compatible API compatibility is not perfectly uniform across providers; some advanced features (object lock, bucket replication, lifecycle tiering) may differ. Application code must be written to the lowest common denominator or provider-specific features must be explicitly gated.
- MinIO in dev/CI does not reproduce all production provider behaviors (e.g., eventual consistency edge cases on some providers, region-specific endpoint behavior). Integration tests must document which behaviors are MinIO-specific.

### Neutral

- A CDN layer (Cloudflare, CloudFront, or provider-native CDN) in front of the bucket for public-watermarked gallery delivery is a separate infrastructure decision and does not affect the storage API choice.
