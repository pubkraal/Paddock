# ADR-0010: Delivery via Short-Lived Presigned S3 URLs; Licence Event Logged at Issue Time

**Status**: Accepted
**Date**: 2026-06-16

## Context

Asset delivery is the primary value exchange in Paddock: an accredited user, having accepted a licence, downloads a rendition of an asset. This event must be audited completely (the brief mandates a full licence-event log — who/what/when/which-licence/IP) and delivered efficiently (sponsors and journalists download high-resolution files; egress is a direct cost).

There are two questions:

1. **How does the byte transfer happen?** Does it flow through the application tier, or does the client fetch directly from the storage layer?
2. **When is the audit record written?** At the moment the download is authorized, at the moment bytes start flowing, or at the moment the transfer completes?

These two questions are coupled: if bytes flow through the application tier, the app can confirm that the transfer occurred and can record its exact IP at completion. If the client fetches directly from S3, the app never sees the bytes and can only record the intent (the authorization decision) and the IP at authorization time.

## Decision

After entitlement check and licence acceptance, `cmd/web` will:

1. Verify the requesting user's entitlement tier for the asset and rendition.
2. Verify embargo state (the asset is published and the requestor's tier can see it).
3. Record a **Licence Event** in the database: user identity, asset ID, rendition requested, licence-text version accepted, requesting IP, timestamp. This write is the audit record.
4. Generate a **short-lived presigned S3 URL** (TTL: 15–60 minutes, to be tuned) for the specific rendition.
5. Redirect the client to the presigned URL; the client fetches bytes directly from S3 or a CDN in front of it.

The audit record is written at **issue time** (step 3), not at completion of the byte transfer. The IP recorded is the IP of the user's request to the application (step 3), not the IP of the subsequent S3 fetch (which may differ if the user is behind a CDN or corporate proxy).

## Alternatives Considered

### Stream bytes through the application tier (proxy download)

**Pros:**
- The application observes every byte transfer; it can write the audit record at transfer completion, with confirmed delivery.
- The IP in the audit record is the IP of the actual byte fetch, not only the authorization request.
- Download can be aborted if entitlement is revoked mid-transfer (edge case, but possible with embargo revocation).

**Cons:**
- Every download flows through the `cmd/web` process: CPU, memory, and network bandwidth are consumed per download. At MVP burst targets (multiple sponsors downloading hi-res files simultaneously), the web tier becomes a bandwidth bottleneck regardless of how many replicas are running.
- Egress costs double: data flows from S3 to the app tier, and then from the app tier to the client. Cloud providers typically charge egress from the object store to the internet; routing via the app tier incurs this cost twice (storage-to-app, then app-to-client) unless the app tier is co-located with the storage.
- The Go HTTP server must hold open connections for the duration of large file transfers; long-lived connections reduce the server's ability to handle concurrent short requests (gallery loads, search queries, admin operations).

**Why rejected**: Egress economics and web-tier resource contention are structurally incompatible with a media-delivery product at the burst targets in the brief. Streaming bytes through the app tier is the right architecture for a small internal tool; it is the wrong architecture for a platform designed to serve 60 sponsors downloading hi-res files simultaneously.

### Hybrid (proxy for audit certainty; presigned for performance)

**Pros:**
- High-security downloads (internal, compliance-critical) could stream through the app for confirmed delivery; public-watermarked or lower-tier downloads could use presigned URLs.

**Cons:**
- Two distinct delivery code paths with different audit semantics. The licence-event schema must accommodate both models, and reporting must distinguish between "issued" and "confirmed delivered" events.
- Adds complexity proportional to the value: the cases where the stronger audit is genuinely required (litigation, GDPR subject-access requests) can be served by the issue-time record combined with S3 server access logs if needed.
- More code paths mean more surface for bugs in the critical entitlement and audit layer.

**Why rejected**: The complexity cost is not justified. The accepted trade-off (see Consequences) is clearly scoped and acceptable for the use cases in the brief.

## Consequences

### Positive

- All download egress flows directly from S3 (or a CDN in front of it) to the client; the `cmd/web` tier is not a bandwidth bottleneck.
- Egress costs are borne once (storage to client), not twice. Using Cloudflare R2 (zero egress) or a CDN in front of the bucket reduces download cost further.
- The web tier can scale for request throughput (authorization checks, gallery rendering) without scaling for byte throughput (download bandwidth).
- Presigned URL generation is a fast cryptographic operation; it does not add latency to the download initiation path.
- The audit record is written transactionally with the entitlement check; if the entitlement check fails, no licence event is written and no URL is issued.

### Negative

- **The audit record captures intent to download (authorization), not confirmed delivery.** A user who receives a presigned URL and does not complete the download (browser crash, network interruption) still has a licence event in the log. This is a deliberate, accepted weakening versus a strict "confirmed the bytes were received" reading of the audit primitive.
- **The IP address in the licence event is the IP of the authorization request to `cmd/web`, not the IP of the subsequent S3 fetch.** These may differ if the user is behind a corporate HTTP proxy (which will forward the presigned URL request without the proxy), or if the client makes the authorization request from a mobile device but the S3 fetch from a different network path. This is a documented, accepted limitation.
- Presigned URLs, once issued, are valid for their TTL regardless of subsequent entitlement revocation. If a licence is revoked after a presigned URL is issued but before it expires, the user can still fetch the bytes within the TTL window. TTL must be kept short (15–60 minutes) to bound this exposure window.
- S3 server access logs (available from all S3-compatible providers) can be used to reconstruct actual download events if needed for a dispute, but this is an operational capability, not a built-in product audit feature.

### Neutral

- The licence event schema records: `user_id`, `asset_id`, `rendition`, `licence_text_version`, `requesting_ip`, `issued_at`, `presigned_url_ttl`. This is sufficient for the brief's audit requirements and for GDPR subject-access requests.
