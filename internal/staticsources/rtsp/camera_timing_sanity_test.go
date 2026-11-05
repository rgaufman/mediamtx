package rtsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

type timingSanityMedia struct {
	Type      string `json:"type"`
	Codec     string `json:"codec"`
	ClockRate int    `json:"clock_rate"`
	Channels  int    `json:"channels"`
	Channel   int    `json:"channel"`
}

type timingSanityRecord struct {
	Channel   int     `json:"channel"`
	Arrival   float64 `json:"arrival"`
	Seq       uint16  `json:"seq"`
	Timestamp *uint32 `json:"timestamp"`
	Payload   int     `json:"payload"`
	Reports   []struct {
		NTP float64 `json:"ntp"`
		RTP uint32  `json:"rtp"`
	} `json:"reports"`
}

func timingSanityFormat(media timingSanityMedia) format.Format {
	switch strings.ToUpper(media.Codec) {
	case "PCMU", "PCMA":
		return &format.G711{
			MULaw: strings.EqualFold(media.Codec, "PCMU"), SampleRate: media.ClockRate, ChannelCount: media.Channels,
		}
	case "G726-16":
		return &format.G726{BitRate: 16}
	case "MPEG4-GENERIC":
		return &format.MPEG4Audio{}
	default:
		return &format.Generic{}
	}
}

// Additional observed recording/metadata issues, independent of the clock assertions:
// TetherBox 72623e34c28478c3f077, camera 6ab64166c4cc047f21a3153c.
// Saved video did not decode cleanly; this is separate from the raw RTP clock result.
// TetherBox 86a83db499f4490b5026, camera 6a881c45480d0ba826fa2fe2.
// Cloud reports Generic audio, but current SDP advertises video and application metadata only.
// TetherBox 86a83db499f4490b5026, camera 6a881c45480d0ba826fa2fe2.
// Saved video did not decode cleanly; this is separate from the raw RTP clock result.
// TetherBox f1087abf2afbc6c9d86e, camera 6a3cf25373fc3b71811a42f8.
// Cloud reports Generic audio, but current SDP advertises video and application metadata only.
// TetherBox f1087abf2afbc6c9d86e, camera 6a3cf25373fc3b71811a42f8.
// Saved video did not decode cleanly; this is separate from the raw RTP clock result.
// TetherBox b5dbc3c46b4fad26c422, camera 69d530f28ad9165cb2c104f0.
// Cloud reports Generic audio, but current SDP advertises video and application metadata only.
// TetherBox b5dbc3c46b4fad26c422, camera 69d530f28ad9165cb2c104f0.
// Saved video did not decode cleanly; this is separate from the raw RTP clock result.
// TetherBox b7ae71aa1a1b30d180da, camera 6477194d4caa4148439d3d16.
// Cloud reports Generic audio, but current SDP advertises video and application metadata only.
// TetherBox 0d5187491e31f977f207, camera 69d534a1eee0ad8870de9a6c.
// Saved video did not decode cleanly; this is separate from the raw RTP clock result.
// TetherBox e70fd4f8ba0747becc2d, camera 6a7bfdc3b221d5d0213d4553.
// Saved video did not decode cleanly; this is separate from the raw RTP clock result.

// Source provenance for future rechecks (internal references, not fixture payload):
// uniview-pkc5601-c4z-led44-wh-pcmu-16000-20261021.json:
// TetherBox 72623e34c28478c3f077, camera 6ab64166c4cc047f21a3153c.
// uniview-ipc3638ss-adf28kmc-i1-no-audio-20261021.json:
// TetherBox 86a83db499f4490b5026, camera 6a881c45480d0ba826fa2fe2.
// uniview-ipc3618le-adf28kc-dl-no-audio-20261021.json:
// TetherBox f1087abf2afbc6c9d86e, camera 6a3cf25373fc3b71811a42f8.
// uniview-pkc2641-z100-ir-p-no-audio-20261021.json:
// TetherBox b5dbc3c46b4fad26c422, camera 69d530f28ad9165cb2c104f0.
// hikvision-ds-2cd2343g0-iu-g726-16-8000-20261021.json:
// TetherBox a22461692a390c6baf00, camera 5fedc93b4caa416bfe7066d8.
// dahua-dh-ipc-hdbw3541ep-as-mpeg4-generic-16000-20261021.json:
// TetherBox b026cc3f4340bbf7b3c6, camera 645e00dd4caa417579cc7d03.
// merit-lilin-z7r8182x3-p-pcmu-8000-20261021.json:
// TetherBox e03965fb62b5069024fc, camera 6ac79c1b4652c3ba9038688e.
// mobotix-mx-vd2a-2-ir-va-no-audio-20261021.json:
// TetherBox b7ae71aa1a1b30d180da, camera 6477194d4caa4148439d3d16.
// uniview-pkc2641-z100-ir-p-pcmu-16000-20261021.json:
// TetherBox 7f513c1f71c93f51fb93, camera 69d5299ddf13e0def6afc243.
// uniview-ipc2b18se-adzkmc-wp-i1-mpeg4-generic-48000-20261021.json:
// TetherBox 0d5187491e31f977f207, camera 69d534a1eee0ad8870de9a6c.
// uniview-nvr301-08x-p8-pcmu-8000-20261021.json:
// TetherBox e70fd4f8ba0747becc2d, camera 6a7bfdc3b221d5d0213d4553.

// Each replay starts with fresh repair-clock state; long-running inherited
// correction state is not covered by these captures.
// These are 60-second source captures, taken before MediaMTX or FFmpeg can
// change timestamps. Payloads are replaced with zero bytes of the recorded
// length: only sample counts, sequence numbers, RTP clocks and arrivals matter.
// Video/metadata-only sources remain negative controls, never audio successes.
func TestCollectedCameraTimingSanity(t *testing.T) {
	fixtures, err := filepath.Glob("testdata/camera-timing-sanity/*.json")
	require.NoError(t, err)
	require.Len(t, fixtures, 11)
	for _, path := range fixtures {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			var fixture struct {
				Seconds int                  `json:"seconds"`
				Medias  []timingSanityMedia  `json:"medias"`
				Records []timingSanityRecord `json:"records"`
			}
			require.NoError(t, json.Unmarshal(data, &fixture))
			require.Equal(t, 60, fixture.Seconds)
			oldSampleNow, oldWallNow := sampleClockNow, wallClockNow
			now := time.Unix(0, 0)
			sampleClockNow = func() time.Time { return now }
			wallClockNow = func() time.Time { return now }
			t.Cleanup(func() { sampleClockNow = oldSampleNow; wallClockNow = oldWallNow })
			for _, media := range fixture.Medias {
				if media.Type != "audio" && media.Type != "video" {
					continue
				}
				require.Positive(t, media.ClockRate)
				var sample *sampleClock
				if media.Type == "audio" {
					sample = newSampleClock(timingSanityFormat(media))
				}
				if sample != nil {
					sample.onStart = func(uint32, uint32) {}
					sample.onReport = func(uint64) {}
				}
				wall := newWallClock(media.ClockRate)
				wall.onStart = func(ppm float64, drift time.Duration) {
					t.Logf("wall correction began: ppm=%f drift=%v", ppm, drift)
				}
				wall.onReport = func(float64, time.Duration) {}
				packets, sampleChanges, wallChanges := 0, 0, 0
				var previous *timingSanityRecord
				for _, record := range fixture.Records {
					if record.Channel == media.Channel {
						now = time.Unix(0, int64(record.Arrival*float64(time.Second)))
						for _, report := range record.Reports {
							seconds := uint64(report.NTP)
							ntp := seconds<<32 + uint64((report.NTP-float64(seconds))*4294967296)
							wall.senderReport(&rtcp.SenderReport{NTPTime: ntp, RTPTime: report.RTP})
						}
					}
					if record.Channel != media.Channel || record.Timestamp == nil {
						continue
					}
					now = time.Unix(0, int64(record.Arrival*float64(time.Second)))
					original := *record.Timestamp
					packet := &rtp.Packet{
						Header: rtp.Header{SequenceNumber: record.Seq, Timestamp: original}, Payload: make([]byte, record.Payload),
					}
					if sample != nil {
						// Independently verify each consecutive fixed-width source
						// packet advances by the preceding payload's sample count.
						if previous != nil && record.Seq-previous.Seq == 1 {
							expected := uint32(previous.Payload / media.Channels)
							if strings.EqualFold(media.Codec, "G726-16") {
								// G726-16 at 8 kHz has four 2-bit samples per byte.
								expected = uint32(previous.Payload * 4 / media.Channels)
							}
							require.Equal(t, expected, original-*previous.Timestamp, "source payload/timestamp disagreement")
						}
						sample.process(packet)
						if packet.Timestamp != original {
							sampleChanges++
						}
					}
					afterSample := packet.Timestamp
					wall.process(packet)
					if packet.Timestamp != afterSample {
						wallChanges++
					}
					previous = &record
					packets++
				}
				require.Greater(t, packets, 10)
				require.Zero(t, sampleChanges, "sample-count repair changed a conforming source")
				require.Zero(t, wallChanges, "wall-clock repair changed this nominal-rate capture")
				t.Logf("%s %s/%d packets=%d sample_changes=%d wall_changes=%d",
					media.Type, media.Codec, media.ClockRate, packets, sampleChanges, wallChanges)
			}
		})
	}
}
