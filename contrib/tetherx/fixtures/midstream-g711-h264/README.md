# Current midstream timing fixture

**Revision 2 is the default. Revision 1's RTP/sequence/SSRC rebasing and near-zero SR NTP normalization are SUPERSEDED.** Generic RTP/RTCP timing counters are not treated as private identity and are preserved raw.

This is a sanitized header/timing trace from the **current 20:43–20:46 continuation window**. It is **not** the historical 20:10 incident and **not** a true startup capture. The decoder's initial clock state and every per-unit NTP value are **unobserved**. Do not silently reconstruct either from capture arrival or sender reports.

The representative TCP trace has the main codec configuration, also observed on a UDP flow. That equivalence does not identify which socket the recorder consumed. Two equivalent UDP audio traces use the same synthetic audio IDs. No socket was selected for its measured clock rate.

## Files

| File | Meaning |
|---|---|
| `events.jsonl.gz` | 48,075 ordered interleaved header events: 46,430 video RTP, 1,437 audio RTP, 180 video SR and 28 audio SR. |
| `equivalent-audio-events.jsonl.gz` | 2,874 observed audio header events from two equivalent transports. Equal IDs materialize equal synthetic payloads. These are alternatives, not extra packets to add to the representative stream. |
| `observed-audio-outcomes.jsonl.gz` | Derived observations: 1,023 uniquely retained audio IDs with validated local ffprobe DTS joins and 414 IDs absent from all 18 current recording exports. These labels are **not replay inputs**. |
| `recorded-files.json` | Eighteen normalized file IDs, relative file metadata and audio packet counts. Global mtxi DTS is not included or inferred. |
| `manifest.json` | Scope, normalization, counts and known limitations. |
| `fixture.go` | Go standard-library loader, synthetic payload materializer and replay adapter seam. |

The three missing runs contain 138 packets each: IDs `a000161`–`a000298`, `a000649`–`a000786`, and `a001129`–`a001266`. Each packet has 1,000 PCMU samples, so each run contains `138 × 1000 / 8000 = 17.25` seconds of payload. This is an observed label from exact private PCM identity, not a synthetic payload property or a claim that an unprimed replay will reproduce the discards.

## What is preserved

- Relative capture first-byte, frame-completion and TCP-prefix-available arrival times, at the source capture's microsecond precision, represented as integer nanoseconds. The primary event order is TCP stream order; `ordered_arrival_ns` is monotonic. Packet completion and first-byte arrivals are separate evidence.
- Payload size, marker, **raw uint16 sequence counters and raw uint32 RTP timestamp counters**, including their observed offsets, zeros and wrap conditions. Audio stays **PT0/SSRC10**. Video stays **PT99/SSRC0**. First-packet counters are not reset to zero. Sender packet/octet counters are also raw.
- **Matching SR RTPTime remains raw in exactly the same domain as RTP timestamps.** One common A/V **whole-second constant shift** moves source SR NTP seconds into a positive year-2000 epoch. Every original 32-bit fractional field is unchanged, including zero/nonzero and whole-second classification. No first SR is reset to NTPTime zero. Exact raw source fields and the reversal shift remain in private provenance outside this directory.
- H264 packetization/NAL-type eligibility metadata, FU start/end flags and SPS/PPS/IDR presence flags. No original NAL, SPS, slice or frame bytes are present.
- Synthetic IDs and observed audio equivalence. A payload ID describes occurrence identity only; it is not an original media hash.
- Validated retained-packet joins: normalized file ID, ffprobe packet/audio indexes and original **within-file** DTS/PTS/duration values. Cross-file global DTS requires separately validated mtxi data.

No source addresses, network port numbers, URLs, credentials, camera identifiers, original audio/video bytes or actual media-payload hashes are included. Raw captures, recordings, actual hashes and reversal mappings remain outside this fixture directory.

## Go-local replay

No external dependencies or network access are required:

```sh
GOTOOLCHAIN=local GOPROXY=off go test ./...
```

Use `ReadEvents` directly, or implement `Sink` and call `Replay("events.jsonl.gz", sink)`. `AdvanceTo` receives virtual **capture** time. `RTP` receives the observed header metadata and deterministic opaque synthetic bytes of the observed size. `SenderReport` receives normalized source SR fields. Neither method supplies observed unit NTP or an observed initial decoder state.

The synthetic bytes are **not codec-valid H264** and are not the observed PCMU. A timing test should use the H264 eligibility metadata through an explicit test adapter. A real codec-path test must provide independent synthetic codec fixtures and explain that substitution. Do not feed these opaque bytes to a real H264 parser and describe its result as replaying the original media.

Declare any priming history, decoder initial state, clock anchoring or unit-NTP policy in the consuming test. This fixture alone cannot validate those assumptions. Compare a replay result with the separate observed outcome labels only after stating them; the loader never injects the 414 missing labels into the stream.

The positive year-2000 SR epoch is a deliberate common translation, not an observation that the source and host trust or agree on an absolute epoch. **Absolute source-versus-host epoch trust is unqualified.** Capture-relative virtual time is not observed host/per-unit NTP. The translation preserves fractional bits and relative A/V phase, but it does not qualify tests whose result depends on the original absolute source-versus-host wallclock epoch.
