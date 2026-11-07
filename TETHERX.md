# TetherX camera validation and fork maintenance

This is the portable process for this fork. TetherBox deployment commands, private
device measurements and raw captures belong in the tetherbox-ai MediaMTX runbook
and experiment, not in public fixtures. Keep one operational runbook; link to its
evidence rather than copying evolving status into code comments.

## Approved surveillance integration

### Upstream sync and publication, 7 November 2026

PROVEN: merged upstream MediaMTX main `b04cc448bf577d205313faaedaded82e71bacaba`,
updating keyfunc3.9.1 and jwkset0.11.4. The dependency now pins published
gortsplib commit `20edd3ee7f2368633eba741152b6dffb73a33fc1`, which includes
upstream main `e4283d1721bc46f55a177bed164d7b59a51b635d` and our camera fixes.

OBSERVED: native Linux ARM64 Go1.27.2 race suites pass on an isolated admitted
Spark CPU job:12 affected packages, including auth/API, then all core TestAPI
cases. The native CGO-disabled Linux build also passes. The library's full
`go test -race ./...`, including root networking cases, passes independently.
No camera service or deployed release package was changed. RESS is excluded
until the owner restores availability; this validation used a quiet alternate.

The [upstream-sync ledger](contrib/tetherx/validation/upstream-sync-20261107.json)
records the exact upstream revisions, source archives and results. Fork defaults
are updated by fast-forward publication; the original upstream history is retained.

PROVEN: branch `feat/surveillance-hardening-20261107` preserves the local camera
work and includes recorder PR #6451, empty-file prevention #6438 and WebRTC
feedback #6347. Its initial gortsplib integration was commit
`71b21e5fd86fd84fdb87cd8375bc1e34d8488fae`, adding upstream #1178/#1181 on top
of normalized session jitter; the published sync above supersedes that pin. The recorder keeps exclusive file creation and
announces a segment only after its header is written. Existing session-expiry
API fields are retained.

WebRTC's proposed raw jitter field is retained, but mixed RTP clock ticks are
not a time measurement. `outboundRTPPacketsReportedJitterUs` converts each
reporting track before averaging. It is null without reports or with an unknown
reporting clock; measured zero remains zero, and the metric is omitted when
unavailable. RTSP normalization is delegated to the library rather than duplicated.

OBSERVED: local race suites cover the affected recorder, stream, playback,
recordstore, estimator, WebRTC protocol/server/forwarding, RTSP, metrics and API
paths. New compatibility and header/collision regressions pass. The Linux ARM64
main and API test binaries cross-compile. This is not a new hardware soak or a
provisioner release: deployed tx.13 packages/recipe are separate from this branch.

The initial library integration was tested locally before publication; the sync
above now uses its published successor and a normal remotely resolvable module
pin. No absolute replacement path is committed. Publish future library changes
before applications pin them for fresh network consumers. Local machine caches
and temporary workspaces are not part of the committed source.

See [the integration ledger](contrib/tetherx/validation/surveillance-hardening-20261107.json)
for upstream revisions, test scope and known environment limits. The older
tx.13 ledger below remains historical acceptance evidence, not acceptance of all
new behavior on physical cameras.

## Establish which code was tested

PROVEN: recorder PR [#6451](https://github.com/bluenviron/mediamtx/pull/6451)
is commit `f950574408b4eb14871eb88e497509b95d8786ce` against upstream
`b04cc448bf577d205313faaedaded82e71bacaba`. The deployed tx.13 recipe instead
starts at `ede14930ebd989401257d6f0729676712db36705`, applies its registered
patches, vendors the pinned dependencies and builds with Go 1.27.2. Six reviewed
production file hashes matched the recorder PR. The local WIP branch in this
checkout is not automatically that release. Never transfer a passing result
between branches or binaries based only on a version string.

The [machine-readable tx.13 ledger](contrib/tetherx/validation/tx13.json) records
the build inputs, six production hashes, binary hash, measured results and limits.

Record source commit, patch hashes, dependency commits, compiler, architecture,
build flags, binary hash and configuration. The current provision manifest binds
source, registered patches, Go version/architecture and binary SHA. Record the
declared build flags separately as well: they are not independent fields in that
manifest. A changed recorded input is a cache miss; a flag/environment change
must deliberately update the identity/release rather than reuse the version tag.

## Choose the owning project before writing a fix

| Behavior | Owner and reason |
|---|---|
| fMP4 segment selection, startup pre-roll, pending LPCM, filenames, UUID ranges | MediaMTX recorder: gortsplib does not write these files or choose boundaries. |
| MediaMTX reader ingress timestamps and locally replaced NTP | MediaMTX Stream/Reader/estimator: these paths also serve non-RTSP publishers. |
| RTSP parsing, transport negotiation, interleaving, RTP depacketization, client/server reception statistics | gortsplib: shared protocol behavior belongs below application adapters. |
| Generic H264/H265 syntax helpers or DTS extraction | mediacommon; application recovery/drop/recording policy stays in MediaMTX. |
| HLS publication and part scheduling internals | gohlslib; MediaMTX owns configuration and integration. |
| Downstream camera clock-repair policy | Currently MediaMTX's RTSP source adapter. Prove a generic library defect separately before proposing a gortsplib API. |

PROVEN precedent: the maintainer redirected normalized jitter from MediaMTX
[#6399](https://github.com/bluenviron/mediamtx/pull/6399) to gortsplib
[#1209](https://github.com/bluenviron/gortsplib/pull/1209). The deployed recipe
now reads the library's microsecond field rather than recomputing it locally.
The older local WIP branch can still contain the earlier calculation; do not
reintroduce it when updating the pinned library. This precedent does not move
recorder file-lifecycle policy into an RTSP library.

## Diagnose before rebuilding

1. Freeze binary/process/source identities and actual configured codecs. Track
   IDs come from SDP/SETUP and MP4 metadata, not log format-array order.
2. Compare a bounded direct-source ffmpeg read with the existing proxy and closed
   recording over overlapping coverage. Keep requested tracks/transport equal.
   Preserve matching closed clips immediately, before spool retention removes
   them; perform expensive analysis offline afterwards.
3. Keep timestamped packets or decoded-frame PTS plus sample counts. WAV/PCM alone
   concatenates samples across gaps. Do not insert silence or use async resampling
   while diagnosing. Declared packet duration is not actual payload coverage.
4. Classify source availability, wire-arrival pauses, missing sequence positions,
   received-but-unrecorded IDs, malformed media and incomplete captures separately.
   A failed ffmpeg read can implicate the source/session/network, not necessarily
   physical camera hardware. A clean later sample cannot explain an older failure.
5. Bound wall time, output bytes, CPU/RSS and free space. Record final exits,
   signals, tcpdump drops and file sizes/hashes. A forced-kill container tail is a
   diagnostic limitation. Keep URLs, credentials and real pictures/speech private.

Trace clocks independently:

`raw RTP/SR -> source repair/PTS decoder -> local estimator -> prequeue receipt -> recorder -> persisted DTS/NTP`

PROVEN: with `ReplaceNTP=true`, camera SR NTP is not the recorder unit's NTP.
Monotonic ingress belongs before queuing; consumer time confuses backlog with
source freshness. Never infer unobserved producer NTP or initial state from output
labels or an interpolated proxy SR.

## Reuse evidence and gate each iteration

The [fixture suite](contrib/tetherx/fixtures/README.md) preserves two real timing
traces without camera media/identities. It runs without device access or project
dependencies:

```sh
MTX_GO=/path/to/go contrib/tetherx/check-fixtures.sh
```

Use three layers: output-only MP4 observations, incoming RTP/SR observations, and
controlled valid-codec Stream/Reader/Recorder tests. Missing producer clocks stay
unknown. Sanitized synthetic video bytes in the timing fixtures are not valid
video access units. Fixture integrity tests are not historical recorder replay.

Before a deployment, in a disposable prepared source tree matching the candidate:

```sh
go test -p 2 -run '^$' ./internal/recorder ./internal/stream ./internal/playback ./internal/recordstore ./internal/ntpestimator
go test -p 2 -count=1 ./internal/recorder ./internal/stream ./internal/playback ./internal/recordstore ./internal/ntpestimator
go test -race -p 2 -count=1 -timeout 12m ./internal/recorder ./internal/stream ./internal/playback ./internal/recordstore ./internal/ntpestimator
```

Pin `GOTOOLCHAIN=local` and the intended compiler. For a prepared vendored recipe
tree add `-mod=vendor`; do not regenerate vendor after applying vendor patches.
Verify focused test names exist (`go test -list`) so an empty `-run` match cannot
masquerade as a pass. The compile-only gate catches mismatched patch signatures;
the unchanged original fMP4 test and full suites catch semantic timestamp errors.

Require an old-fails/new-passes assertion over real output IDs, DTS, payloads and
keyframes. Cover both startup orders, B-frames/negative DTS, short filenames,
source pause/resume across rotation, known LPCM duration, write retry/duplicates,
prequeue backlog, local reference changes and absolute/stale/media-jump guards.
Use the existing shared recorder analyzers and pause-case driver. An observer ACK
is not a recorder queue barrier. Identify intentionally unclosed lookahead IDs.

Build once after these gates. Cache by the frozen input identity. Repeat a long
soak only when data-plane code, dependencies, configuration or the relevant
failure model changes; docs or a fixture-loader check do not require another
MediaMTX build or device restart. A changed media pipeline must still receive its
targeted integration and restart acceptance.

OBSERVED acceptance for tx.13: stress n=4 over 45 minutes after a deliberate
restart, 432/432 keyframe-first clips and 281827/281827 complete captured G711
packets retained. A separate 47m10s synthetic native ARM64 run retained 22581 audio
and 70558 video settled samples at original DTS, 49/49 video-bearing files
keyframe-first; only the final lookahead per track was excluded. These results do
not establish global zero audio holes, source transport losslessness or absolute
A/V synchronization. Private operational evidence remains outside this fork.

## Finish an iteration, including package cleanup

1. Save the failing fixture, expected IDs and raw result before changing code.
2. Keep one patch registry and manifest identity; do not duplicate patch lists in
   the application, cache selector and build script.
3. After local tests, build once for each required architecture and verify its
   binary/manifest, activation and restart evidence. Stage only the intended tree.
4. Keep a validated replacement and any explicitly required rollback build per
   platform/architecture. A newer version number is not proof of validation.
5. In tetherx-provision, preview `bin/prune-obsolete-packages --only mediamtx`,
   record the exact paths/bytes, then repeat with `--apply` for that scoped action.
   It cannot be combined with CPU-variant pruning. Do not run a general package purge for a MediaMTX task.
   Delete superseded binary/manifests together only after the replacement passes
   the current recipe identity, checksum and ELF checks. Missing/corrupt/mismatched
   replacements preserve older packages. Keep ARM caches until ARM replacements
   exist; a native lab binary is not automatically a provisioner package.
6. Publish the final source/validation ledger and known limits. Do not change a
   validated timing algorithm just to tidy it up without rerunning its semantic
   regressions. Do not promote an unchanged artifact through another long soak.

See the [dated upstream review](contrib/tetherx/upstream-review-2026-11-07.md)
for merge candidates, application-specific deferrals and cleanup findings.
The [our-PR index](contrib/tetherx/pull-requests.md) links every authored PR in
the MediaMTX stack with its status and downstream context.
