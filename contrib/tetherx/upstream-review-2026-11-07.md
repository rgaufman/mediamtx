# Surveillance fork review — 7 November 2026

OBSERVED review snapshot: 7 November 2026,12:08 AEDT. Screened50 open MediaMTX
PRs and 8 open gortsplib PRs; inspected relevant descriptions/discussions and
targeted implementation diffs. This is a merge-candidate assessment, not a claim
that all those branches were executed or proved compatible together. No candidate
below was merged, deployed or given another device soak during this review.

**Subsequent owner approval:** #6438, gortsplib #1178/#1181 and #6347 were
approved for local fork integration and commit after testing. They are now on
the surveillance integration branches, with recorder #6451 composed into the
MediaMTX baseline first. The original assessment below remains a dated review;
the [integration ledger](validation/surveillance-hardening-20261107.json) records
the resulting source and tests. No upstream merge or device deployment is claimed.

The WebRTC adaptation adds a nullable clock-normalized microsecond field alongside
the proposed raw statistic. Conflict resolution retains local session-expiry
fields and recorder `O_EXCL` behavior. The older local RTSP jitter calculation is
replaced by the library field. Test portability uses the real loopback interface
and keeps Linux inotify checks in a Linux-only test while HTTP pagination checks
run on every platform; assertions were not relaxed to obtain a pass.
The [open-PR head snapshot](validation/upstream-open-2026-11-07.json) records
the exact 58 head SHAs. Recheck a changed head or combined target tree rather than
repeating the entire search for an unchanged candidate.

## Application fit and next candidates

PROVEN from the current TetherBox configuration builder: RTSP ingestion, fMP4
recording and WebRTC are used; `hlsAlwaysRemux` is false; SRT/RTMP/MoQ are disabled.
The generator does not enable MediaMTX's playback server, whose default is false.
Operator overrides can change this priority. Our own spool/timeline reader still
cares about valid recorder files even when MediaMTX playback is disabled.
This is a source/configuration-builder review, not a fresh fleet configuration audit.

| Candidate | Assessment for this application | Gate before merging |
|---|---|---|
| MediaMTX [#6438](https://github.com/bluenviron/mediamtx/pull/6438), prevent empty files on init failure | **Consider first.** Invalid camera SPS or a failed header write should not leave an empty segment or announce it to the spool hook. | Compose against #6451's exclusive creation; preserve `O_EXCL`, never remove a pre-existing colliding file, and test invalid SPS, partial writes, callback order and existing-file preservation. |
| gortsplib [#1178](https://github.com/bluenviron/gortsplib/pull/1178), mismatched FMTP PT | **Conditional camera compatibility.** Narrow fallback for affected Hikvision-style single-format SDP; potentially useful for AAC discovery. | Use a real affected SDP, retain strict ambiguous/multiple-format rejection, and test source selection/decode. No affected-unit assertion from the PR description alone. |
| gortsplib [#1181](https://github.com/bluenviron/gortsplib/pull/1181), missing SETUP Transport | **Conditional legacy TCP-camera compatibility.** Assumes requested interleaved IDs only when the header is absent over TCP. | Capture the affected response; test second-track channel allocation, collision detection, missing versus malformed headers and unchanged UDP/multicast rejection. |
| MediaMTX [#6347](https://github.com/bluenviron/mediamtx/pull/6347), WebRTC RTCP feedback | **Useful observability candidate.** WebRTC is active; receiver loss, NACK and PLI can distinguish viewer trouble from source/recorder loss. | Check API/metrics compatibility, synchronization and counters. NACK requests are not permanent packet loss; raw jitter needs the correct clock units. |
| gortsplib [#1157](https://github.com/bluenviron/gortsplib/pull/1157), signing SEI preservation | **Conditional ONVIF signing support.** Prevents splitting opaque SEI data containing inner start codes. | Need signing-camera evidence and existing Annex-B/bundled-NAL controls. The author documents a residual `SEI + start code + IDR` ambiguity; do not blanket-enable as a generic corruption fix. |
| gortsplib [#1191](https://github.com/bluenviron/gortsplib/pull/1191), auth policy | **Site-policy option**, not an audio fix. Defaults retain current compatibility. | Expose opt-in policy only for sites that require it; refusing MD5/Basic globally can disconnect legacy cameras. |

These are HYPOTHESIS / candidate benefits until the corresponding fixture and
combined branch pass. Prefer one small composed change and its targeted tests,
not a batch of timing changes followed by another expensive blind soak.

## Relevant but lower priority or incompatible with a blind merge

| PR | Decision and reason |
|---|---|
| [#6447](https://github.com/bluenviron/mediamtx/pull/6447), bounded playback reads | Good hardening **if playback is enabled**. Bounds admitted segment reads to 16; cooperative context checks do not interrupt an already blocked filesystem open. Combine with #6439/#6195 only once: they change the same parsing signatures. |
| [#6439](https://github.com/bluenviron/mediamtx/pull/6439), skip unreadable playback segments | Useful with playback. `/get` stops at a bad following segment; it does not heal it or prove a gapless export. Keep all-broken-range behavior and warnings. |
| [#6195](https://github.com/bluenviron/mediamtx/pull/6195), playback header cache | Profile actual `/list` use first. Verify open-file/mtime invalidation and bounded memory; our short volatile spool differs from long archives. |
| [#6014](https://github.com/bluenviron/mediamtx/pull/6014), seek indexes/durations | More valuable for long standalone exports than minute clips. Changes on-disk layout/close behavior; test partial files, atom offsets, per-track indexes and every spool/archive consumer. |
| [#6262](https://github.com/bluenviron/mediamtx/pull/6262), keyframe-aligned parts | Parts are not segments. Can lengthen part/flush duration and remains bounded by max-part size. Our manifest logic assumes the configured 1 s flush allowance; validate that contract before opting in. |
| [#6305](https://github.com/bluenviron/mediamtx/pull/6305), local correction recovery | Overlaps #6451; do not stack both recorder implementations. Its `~N` collision filenames also require matching discovery/deletion/playback and TetherBox parser changes. Our current collision guard preserves old files but a new collision can fail; it does not implement this wider allocation policy. |
| [#6255](https://github.com/bluenviron/mediamtx/pull/6255), PTS sanitizer | **Hold.** Rewrites timestamps, unlike preserving media DTS in our recorder fix. Need discontinuity/B-frame/marker/wrap fixtures and explicit A/V/RTCP/proxy mapping expectations before adding a second timeline repairer. |
| [#5896](https://github.com/bluenviron/mediamtx/pull/5896), [#5714](https://github.com/bluenviron/mediamtx/pull/5714), extractor recovery | Alternatives, not additive fixes. Continuing drops frames until another keyframe; warn/reset is not losslessness. Need recorder epoch/boundary and protocol tests before changing failure semantics. |
| [#5999](https://github.com/bluenviron/mediamtx/pull/5999), global disk quota | Do not introduce a second deletion authority without the event/archive retention contract. Newest-file protection does not protect every pending event segment. |
| [#5940](https://github.com/bluenviron/mediamtx/pull/5940), wall-aligned rotation | No current need; introduces short first segments and another wall-clock dependency next to recorder corrections. |
| [#5934](https://github.com/bluenviron/mediamtx/pull/5934), fallback sources; [#6448](https://github.com/bluenviron/mediamtx/pull/6448), combined offline tracks | Conditional offline-screen features. Our builder does not configure them. Source swap/PTS changes touch the same stream code as receipt timing; test both codecs, keyframe activation, missing audio and browser discontinuities. |
| [#4189](https://github.com/bluenviron/mediamtx/pull/4189), GOP cache | Potentially faster viewer startup, but replay/receipt age and per-camera memory ownership matter. Profile realistic long GOPs and many-camera/Pi memory before adopting. |
| [#6302](https://github.com/bluenviron/mediamtx/pull/6302), always-remux demand cleanup | Current `hlsAlwaysRemux: false` avoids this trigger. Keep a regression if that setting changes. |
| [#6336](https://github.com/bluenviron/mediamtx/pull/6336), GC tuning docs | Useful measurement guidance; do not copy laptop GOGC values onto constrained Pis without RAM/latency measurements. |

MoQ/SRT-specific work, optional language/multicast support and routine dependency
bumps were screened but are not recommendations to merge into the current camera
fix. Incoming WebRTC-audio ordering is also a different direction from RTSP-camera
ingest and WebRTC viewing. No security-advisory conclusion is made from titles.

## Changes already represented downstream

PROVEN: #6398 source media filtering, #6400 narrow H264 padding sanitation,
#6401 explicit-shell handling and #6397 configuration-burst handling have
downstream implementations. Open PR state does not mean those changes are absent
from the recipe. Reconcile behavior/tests rather than applying a second patch.
#6397 also has maintainer implementation feedback; preserve the reviewed
notification/delete/close behavior when rebasing.

PROVEN: [the #6399 discussion](https://github.com/bluenviron/mediamtx/pull/6399#issuecomment-6396623300)
explicitly redirects shared jitter aggregation to gortsplib. Our pinned library
commit `df6c90d8e37f` exposes the field and the provision patch consumes it.
The recorder changes are different: file boundaries, pending samples, recording
UUIDs and MediaMTX reader receipt timing do not belong to gortsplib. A reusable
codec helper may belong to mediacommon; source-repair policy is not automatically
an RTSP-library defect.

## Cleanup findings

| Finding | Decision |
|---|---|
| Same jitter aggregation in application and library | Already removed in the shipped recipe's API adapter. Older local WIP still has the legacy calculation against an older dependency; reconcile it when updating that branch, not by transplanting the old implementation back into a release. |
| Patch filenames/hashes registered in multiple places | Current `MEDIAMTX_PATCHES` registry already supplies both application stages and build identity. Keep it as the sole list. |
| Build flags outside the manifest fields | PROVEN: source/patch/compiler/architecture/hash fields are recorded, but effective `GOFLAGS`/`GOAMD64` and build flags are not separate identity fields. HYPOTHESIS / hardening candidate: pin the environment and version/hash that contract on the next deliberate build change. No incorrect current artifact was observed; this audit does not invalidate existing packages or manufacture new manifests. |
| Test pause matrices and MP4 analyzers | Recorder PR already shares its pause driver/analysis helpers. Reuse them; do not add another arithmetic-only model in place of recorded-file assertions. |
| Operational narratives in code comments | Older WIP sender-clock tests contain deployment anecdotes and device identifiers. Keep public code comments about invariants and sanitized case IDs; preserve detailed measurements in the private ledger when preparing an upstream patch. |
| Source `maxBasetime` introductory comment | The old distance-limit wording needs qualification: fresh audio can extend lookback. This is a stale-comment cleanup candidate, not grounds to change validated timing behavior. |
| Repeated projection/freshness branches | Similar-looking branches have different held/pending and ingress/media state. Extract only after proving equivalent contracts with existing pause/backlog guards; no speculative refactor or new build in this review. |
| Temporary fixtures/helpers as sole record | Promoted two real timing datasets and standalone tests into this fork. Root `TETHERX.md` is the portable process; private device procedures/evidence stay in one external operational runbook. |
| Old package caches | Scoped cleanup removed two obsolete Ubuntu 24.04 amd64 binaries plus manifests after seeding/verifying the identical current static amd64 replacement. Removed 78771856 bytes, added 39404653 bytes, net 39367203 bytes freed. ARM caches retained without a validated tx.13 replacement. |
| Safe repeatable cleanup entry point | Added `--only mediamtx` to the existing pruner, sharing its validation/deletion/report logic. A regression proves unrelated obsolete Debian/Python caches survive; incompatible CPU-pruning flags are rejected before deletion. |

OBSERVED validation for this review: the two real timing fixture modules pass
in 0.733/0.531 seconds of test execution; package-pruner specs pass
12 examples/0 failures in 1.69 seconds, including two new scope guards. The new
scope regression failed on the old CLI before the change. No production timing code changed, no Go
application build, no deployment and no extra device soak was needed. Documentation
and promoted fixtures are local uncommitted fork changes, respecting the session's
no-commit rule. Additional upstream merges remain candidates, not completed work.
