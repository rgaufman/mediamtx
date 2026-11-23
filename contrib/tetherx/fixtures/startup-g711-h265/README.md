# Actual stress13 source-session bootstrap

This **separate dataset** starts before the observed service restart and includes successful RTSP DESCRIBE/SETUP/PLAY, negotiated source flows and their **first captured RTP/SR anchors**. It is not the older RESS midstream trace, and its packet counts must never be merged with the overlapping long audio capture.

There are eight independent camera cases and sixteen configured main/sub paths. Cached-config URI hashes, control traffic and negotiated UDP ports resolve ownership privately. No source query was made for this derivation. All paths advertise **H265/PT98/90 kHz**; four main paths also advertise **PCMA/PT8/8 kHz**. The other four source DESCRIBEs contain no audio track. That is an initial source-SDP observation, not a receiver-selection explanation, a reason for the source omission, or a claim that availability cannot change later.

## Artifacts

| File | Contents |
|---|---|
| `events.jsonl.gz` | 15,004 source header events: 10,177 H265 RTP, 64 H265 SR, 4,731 PCMA RTP and 32 PCMA SR, with relative wire arrival and synthetic payload IDs. |
| `sessions.json` | Sanitized control chronology, advertised media, successful negotiated tracks and main/sub roles. No URL, auth, address or network port values. |
| `initial-anchors.json` | Actual first/last source headers, first SR, initial parameter-set/random-access eligibility and audio payload/clock cadence. |
| `observed-startup-audio.jsonl.gz` | Derived primary-audio observations: 2,338 unique full-PCM matches to the first closed startup files, with recorded local DTS and independently recovered original 8 kHz DTS ticks. Labels are not replay inputs. |
| `recorded-startup.json` | Codec and first-packet metadata from the eight full private startup MP4s. No media bytes or actual payload hashes. |
| `observed-initial-video.json` | Derived fingerprint-qualified first/second native VCL frame identities and raw source RTP PTS intervals. Actual hashes remain private. Source DTS/POC and physical cause remain unobserved. |
| `manifest.json` | Scope, invocation precision, raw-counter policy, counts and limitations. |
| `fixture.go` | Standard-library Go loader, opaque synthetic payload materializer and replay adapter. |

**Route by case and main/sub path.** They are independent input streams. Do not feed eight cameras, or a main/sub pair, into one decoder as if they were one source. Equivalent audio payloads within a case share synthetic IDs; this does not create extra input for a main-path recording.

## Timing fidelity

Raw RTP timestamp, sequence, PT and SSRC values—including valid zero—are unchanged. Matching SR RTPTime and sender packet/octet counters are also raw. Source SR NTP uses **one common whole-second shift across the entire A/V dataset** into a positive year-2000 epoch. Every original 32-bit fractional field and zero/nonzero whole-second classification remains exact. No first RTP or first SR is rebased to zero.

Wire arrivals retain exact captured precision as relative integer nanoseconds, even where capture timestamps regress across packet records. Event order remains original PCAP record order. `ordered_arrival_ns` is a separately labelled cumulative-maximum scheduling time for monotonic replay; it does not replace the raw arrival evidence or invent an application arrival. `invocation_first_observed_arrival_ns` retains the precise first observation of the new invocation; it is **not** an invented exact process-start time. The retained service start timestamp has one-second precision. Some new-session SYNs precede the first invocation observation by about 213 ms; they are retained rather than wrongly cropped as old traffic. Session control and first-media chronology remain available to the consuming test.

The actual initial **source-session headers** are observed. Exact original application callback arrival, first decoded-unit callback state and per-unit NTP are **not observed**. `unit_ntp_ns` remains null. The positive source epoch does not establish source-versus-host absolute epoch trust. A consuming test must declare receiver clock/NTP policy and any application initialization assumptions.

All observed PCMA packets have 316 unpadded samples, while consecutive packet RTP timestamps advance 320 ticks. The four-tick difference is source payload-versus-clock cadence; it is not evidence of a missing recorder packet or continuous audio payload coverage. Do not replace sample accounting with MP4 `media_duration`.

For all eight first/second recorded video frame pairs, private full-NAL fingerprint identity establishes the source RTP PTS interval. It equals the recorded PTS interval exactly. The first recorded frame has 100 ms composition offset and the second has zero, so its larger DTS step is `PTS step + first CTS − second CTS`. This is an observed output algebra/identity result, not a source DTS or physical-loss claim. These labels remain separate from replay inputs.

## Codec and replay limits

H265 metadata includes VPS/SPS/PPS, IRAP/random-access, VCL, aggregation and FU start/end eligibility. The parser and schema also support H264 and G711; **H264 is not claimed to occur in this particular capture**.

No original video, NAL, parameter-set or speech bytes are included. Synthetic payloads preserve size and explicit identity only and are **not valid H264/H265 bitstreams**. Use the eligibility metadata through an explicit test adapter, or independently generated synthetic codec assets for a real decoder-path test. This fixture alone does not reproduce codec initialization from the removed customer parameter sets.

```sh
GOTOOLCHAIN=local GOPROXY=off go test ./...
```

`ReadEvents` or `Replay(path, Sink)` delivers all observed header events. `AdvanceTo` is relative **wire** time, not producer arrival or Unit.NTP. Observed startup labels are separate and are never injected into the replay. Raw capture, original MP4s, source/auth material, actual payload hashes and reversal maps remain private outside this directory.
