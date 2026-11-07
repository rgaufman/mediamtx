# Our MediaMTX-stack pull requests

OBSERVED account-author search as of 2026-11-07 12:21:33 AEDT: **7 PRs: 6 MediaMTX and 1 gortsplib; all open**. Includes open, closed and merged states in the search; no authored PRs found for gohlslib or mediacommon at this snapshot. Author: `rgaufman`.

| Repository / pull request | Status | Context |
|---|---|---|
| [bluenviron/gortsplib #1209 — Add normalized session RTP jitter in microseconds](https://github.com/bluenviron/gortsplib/pull/1209) | open | Shared microsecond jitter statistics; used by the pinned downstream library fork. |
| [bluenviron/mediamtx #6451 — recorder: retain fMP4 samples across track offsets and reference changes](https://github.com/bluenviron/mediamtx/pull/6451) | open | Recorder retention; tx.13 behavior and tests. Overlaps #6305; source-gap and historical-input limits are documented. |
| [bluenviron/mediamtx #6401 — fix(hooks): preserve explicit shell scripts](https://github.com/bluenviron/mediamtx/pull/6401) | open | Explicit shell hooks; downstream implementation exists. |
| [bluenviron/mediamtx #6400 — fix(h264): strip invalid CAVLC padding](https://github.com/bluenviron/mediamtx/pull/6400) | open | Narrow H264 CAVLC trailer sanitation; downstream implementation exists. |
| [bluenviron/mediamtx #6399 — feat(api): expose normalized RTSP jitter](https://github.com/bluenviron/mediamtx/pull/6399) | open | Thin API field: maintainer requested shared jitter calculation in gortsplib #1209; do not duplicate normalization. |
| [bluenviron/mediamtx #6398 — feat(rtsp): add source media filtering](https://github.com/bluenviron/mediamtx/pull/6398) | open | Select source media before SETUP; downstream implementation exists. |
| [bluenviron/mediamtx #6397 — fix(confwatcher): retain burst updates](https://github.com/bluenviron/mediamtx/pull/6397) | open | Configuration burst updates; maintainer implementation feedback must be considered when rebasing. |

PROVEN ownership distinction: #6399 exposes MediaMTX API data; #1209 owns the shared RTSP statistic. #6451 owns recorder file lifecycle and MediaMTX queue/local-reference policy. Open upstream status and deployed downstream status are different facts.

Refresh with the authenticated author search for all states, follow every page, and verify total_count/incomplete_results. Keep historical closed and merged rows; do not infer merge from an issue being closed. The complete account index is kept in the private operational workspace; this fork index includes only the four named public media upstreams.
