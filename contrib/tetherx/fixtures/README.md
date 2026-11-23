# Real timing fixtures

These are canonical copies promoted from the completed private investigation.
They contain RTP/SR timing and sanitized outcome metadata, not camera URLs,
credentials, identities, original picture/speech bytes or private content hashes.
Raw counters and SR bits are preserved; they must not be normalized to invent a
startup clock relationship. `SHA256-MANIFEST.json` records the frozen file bytes.

| Dataset | Observations | Limits |
|---|---|---|
| `midstream-g711-h264` | 48075 RTP/SR events;1437 PCMU packets and separate outcomes1023 retained/414 absent | Midstream initial decoder state and per-unit NTP are unknown; observed loss labels are not inputs. |
| `startup-g711-h265` | 15004 events across 16 paths; source audio available on 4 main paths;316 samples/320 RTP ticks | Wire arrival is not producer receipt; synthetic video payloads are invalid codec data; separate startup output labels are not producer events. |

Run `../check-fixtures.sh` from this directory, or the root command documented in
`TETHERX.md`. The two nested modules use only the Go standard library. Tests check
real counter, wrap, event/outcome and payload-coverage invariants. They do not
prove that replaying an unprimed recorder reproduces the original failure.

Existing source-side sender-clock and codec regressions under
`internal/staticsources/rtsp/testdata` remain useful; the recorder PR's controlled
valid-codec tests cover startup, offsets, pauses, local reference changes and
guard behavior. Do not add a second implementation of those test drivers here.

HYPOTHESIS / missing evidence: a future occurrence of the historical clock failure
needs a bounded capsule of actual post-source-repair PTS, Unit.NTP, monotonic
producer receipt, initialization state and paired track identity. tcpdump alone
cannot supply those missing callbacks. Collect it at that boundary, preserve
overlapping closed output immediately, then require old-fails/new-passes locally.
The old invalid H264 picture remains private; a later clean capture cannot be its
regression substitute. No public image fixture is included.
