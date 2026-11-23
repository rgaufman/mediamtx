package rtsp

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

// TetherBox 9abcd9bd738fc20c46aa, camera 6aad85cd4d716f7d3efbee7c.
// Live follow-up: 900-second UDP control/candidate had 203/118 late samples;
// the candidate over TCP had 15 segments, zero late samples/loss/reset reports.
// Production UDP loss also confirmed on linked camera 6aad85cd4d716f7d3efbee7f
// on the same TetherBox. Device 6a88c31b222d76d3858ea46d is forced to TCP.
// Exact loss location remains unproven; see tetherbox/doc/drafts/
// 2026-10-21-uniview-clock-recheck/summary.json for counters and future rechecks.
// Fresh 600-second direct capture on 2026-10-21: video +1610 ppm versus
// sender NTP, audio near nominal. Fixture contains timing and byte counts only.
func TestCapturedSenderClockIndependentOfQueue(t *testing.T) {
	data, err := os.ReadFile("testdata/uniview-sender-clock-20261021.json")
	require.NoError(t, err)
	var trace struct {
		Seconds int `json:"seconds"`
		Medias  []struct {
			Type    string `json:"type"`
			Rate    int    `json:"clock_rate"`
			Channel int    `json:"channel"`
		} `json:"medias"`
		Records []struct {
			Channel   int     `json:"channel"`
			Arrival   float64 `json:"arrival"`
			Timestamp *uint32 `json:"timestamp"`
			Reports   []struct {
				NTP float64 `json:"ntp"`
				RTP uint32  `json:"rtp"`
			} `json:"reports"`
		} `json:"records"`
	}
	require.NoError(t, json.Unmarshal(data, &trace))
	require.Equal(t, 600, trace.Seconds)
	for _, media := range trace.Medias {
		if media.Type != "audio" && media.Type != "video" {
			continue
		}
		t.Run(media.Type, func(t *testing.T) {
			var reference []uint32
			for _, queue := range []bool{false, true} {
				now := time.Unix(0, 0)
				wallClockNow = func() time.Time { return now }
				defer func() { wallClockNow = time.Now }()
				clock := newWallClock(media.Rate)
				var outputs []uint32
				var first, last uint32
				var start, end float64
				for _, record := range trace.Records {
					at := record.Arrival
					if queue {
						at += (at - trace.Records[0].Arrival) / 1000
					}
					now = time.Unix(0, int64(at*float64(time.Second)))
					if record.Channel == media.Channel {
						for _, r := range record.Reports {
							seconds := uint64(r.NTP)
							ntp := seconds<<32 + uint64((r.NTP-float64(seconds))*4294967296)
							clock.senderReport(&rtcp.SenderReport{NTPTime: ntp, RTPTime: r.RTP})
						}
					}
					if record.Channel != media.Channel || record.Timestamp == nil {
						continue
					}
					packet := &rtp.Packet{Header: rtp.Header{Timestamp: *record.Timestamp}}
					clock.process(packet)
					if len(outputs) == 0 {
						first = packet.Timestamp
						start = record.Arrival
					}
					last = packet.Timestamp
					end = record.Arrival
					outputs = append(outputs, packet.Timestamp)
					if media.Type == "audio" {
						require.Equal(t, *record.Timestamp, packet.Timestamp)
					}
				}
				if media.Type == "video" {
					require.True(t, clock.active)
					require.InDelta(t, 1610, clock.ppmValue, 40)
					lead := float64(int32(last-first))/float64(media.Rate) - (end - start)
					require.Less(t, time.Duration(lead*float64(time.Second)).Abs(), 80*time.Millisecond)
					require.InDelta(t, -0.95, clock.duration(clock.correction).Seconds(), 0.08)
					t.Logf("queue=%v ppm=%.1f correction=%v corrected_lead=%v", queue, clock.ppmValue, clock.duration(clock.correction), time.Duration(lead*float64(time.Second)))
				} else {
					require.False(t, clock.active)
				}
				if queue {
					require.Equal(t, reference, outputs, "queue growth changed output timestamps")
				} else {
					reference = outputs
				}
			}
		})
	}
}
