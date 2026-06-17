# ADR-0006: ImageMagick (Shell-Out) for Image Processing on the Worker Tier

**Status**: Accepted
**Date**: 2026-06-16

## Context

The worker tier must process every ingested camera file into multiple derivative outputs:

- **Thumbnails** (small JPEGs for gallery grids)
- **Web previews** (medium-resolution JPEGs for portal viewing)
- **Resolution-capped renditions** (per entitlement tier: e.g., 1200px long-edge for Sponsor-Commercial, full-resolution for Media-Editorial)
- **Watermark compositing** (text overlays, sponsor-logo PNG overlays on the Public-Watermarked tier)

At MVP burst targets (1,000 images/hour/event) and Y1 targets (10,000/hour), this processing is CPU- and memory-intensive and bursty. The processing runs exclusively on the worker tier (`cmd/worker`) and never on the serving path.

The choice of image processing library or tool has implications for: CGo dependency (build complexity, cross-compilation, Docker image size), memory profile per image, per-image latency, and operational familiarity.

## Decision

We will use **ImageMagick** via shell-out (`exec.Command("convert", ...)`) for all image processing on the worker tier.

The worker must:

- Cap the number of concurrent ImageMagick invocations via a semaphore to prevent memory exhaustion during burst. The concurrency limit is tunable via environment variable.
- Apply back-pressure at burst: River's worker concurrency configuration limits the number of processing jobs running simultaneously; this is the primary back-pressure mechanism.
- Monitor per-invocation resource use in production and adjust the concurrency cap to protect the worker process.

## Alternatives Considered

### govips / libvips (CGo binding)

**Pros:**
- Significantly faster than ImageMagick for most operations (vips processes pipelines in a single pass with lower memory overhead per image).
- Lower per-image RAM consumption; more images can be processed concurrently within the same memory budget.
- Well-maintained Go binding (`govips`).

**Cons:**
- Requires CGo, which complicates cross-compilation and adds a C toolchain dependency to the build environment.
- Requires `libvips` to be installed in the Docker image; increases image size and introduces a system-level dependency that must be kept up to date for security patches.
- CGo disables some Go race detector functionality; CGo-related crashes are harder to diagnose than pure Go panics.
- Less familiar to the team than ImageMagick.

**Why rejected**: The CGo build complexity and system dependency are disproportionate for MVP. govips is the right move if profiling shows ImageMagick throughput is insufficient at Y1 burst targets; this is a recognized, well-scoped migration path.

### Pure-Go imaging libraries (e.g., `imaging`, `disintegration/imaging`, `nfnt/resize`)

**Pros:**
- No CGo; pure Go compilation with no system dependencies.
- Simplest possible build and deployment.

**Cons:**
- Pure-Go decoders for camera formats (RAW, high-quality JPEG from professional cameras) are incomplete or absent; ImageMagick's codec coverage is vastly wider.
- Memory-hungry for large images: Go's garbage collector must retain entire decoded image buffers; pure-Go libraries do not offer streaming/pipeline processing.
- Slower than ImageMagick for most real-world operations at the resolutions Paddock processes.
- Watermark compositing with arbitrary sponsor-logo PNGs (transparency, positioning, scaling) is poorly supported in most pure-Go libraries.
- At Y1 burst targets (10,000 images/hour), pure-Go processing is likely to become a bottleneck before govips would.

**Why rejected**: Codec coverage and memory characteristics are insufficient for professional camera files at the burst targets in the brief.

## Consequences

### Positive

- ImageMagick is widely understood; the team and future engineers will be familiar with its command-line interface and options.
- No CGo; the Go binary compiles without a C toolchain. Docker images require only the `imagemagick` system package.
- Shell-out invocations are naturally isolated: a processing bug or resource spike in a single ImageMagick process does not corrupt the Go heap.
- Watermark compositing with arbitrary PNG overlays (sponsor logos) is a well-documented ImageMagick operation.

### Negative

- Each image spawns a child process; the process-spawn overhead (fork/exec) is non-trivial at high concurrency. At burst, the concurrency cap must be tuned carefully to balance throughput against process-spawn latency.
- Concurrency must be actively bounded by a semaphore in application code; an unbounded burst of ingest jobs would spawn unbounded ImageMagick processes, exhausting system memory and potentially OOM-killing the worker.
- Shell-out is harder to unit-test than a pure-Go function call; tests that exercise image processing paths require either a real ImageMagick installation in CI or an interface that allows the exec call to be swapped for a test double. The image processing step must be behind an interface (`ImageProcessor`) to enable unit testing.
- If Y1 throughput requirements (10,000 images/hour) cannot be met with the concurrency cap that fits within the worker's memory budget, migration to govips/libvips will be required. This is a known, accepted risk.

### Neutral

- The `ImageProcessor` interface wrapping the shell-out is defined in `internal/ingest`; swapping to govips later is a new implementation of that interface, not a change to callers.
