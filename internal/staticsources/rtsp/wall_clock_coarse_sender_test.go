package rtsp

import (
	"encoding/json"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

// This real trace checks observable correction without depending on guard internals.
func TestWallClockCoarseSenderTrace(t *testing.T) {
	var capture struct {
		Tracks []struct {
			ClockRate int         `json:"clock_rate"`
			SSRC      uint32      `json:"ssrc"`
			Packets   [][]float64 `json:"packets"`
			Reports   [][]float64 `json:"reports"`
		} `json:"tracks"`
	}
	data, err := os.ReadFile("testdata/whole-second-sender-ntp.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &capture))
	require.Len(t, capture.Tracks, 1)
	track := capture.Tracks[0]
	type event struct {
		at        float64
		packet    bool
		timestamp uint32
		ntp       uint64
	}
	events := make([]event, 0, len(track.Packets)+len(track.Reports))
	for _, p := range track.Packets {
		events = append(events, event{at: p[0], packet: true, timestamp: uint32(p[1])})
	}
	for _, r := range track.Reports {
		events = append(events, event{at: r[0], ntp: uint64(r[1]), timestamp: uint32(r[2])})
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].at < events[j].at })
	start := time.Unix(0, 0)
	now := start
	wallClockNow = func() time.Time { return now }
	defer func() { wallClockNow = time.Now }()
	clock := newWallClock(track.ClockRate)
	clock.onStart = func(float64, time.Duration) {}
	clock.onReport = func(float64, time.Duration) {}
	var worst time.Duration
	for _, e := range events {
		now = start.Add(time.Duration(e.at * float64(time.Second)))
		if e.packet {
			header := rtp.Header{SSRC: track.SSRC, Timestamp: e.timestamp}
			clock.process(&rtp.Packet{Header: header})
			worst = max(worst, clock.duration(clock.correction).Abs())
		} else {
			clock.senderReport(&rtcp.SenderReport{SSRC: track.SSRC, NTPTime: e.ntp, RTPTime: e.timestamp})
		}
	}
	t.Logf("worst correction=%v, final correction=%v", worst, clock.duration(clock.correction))
	require.Less(t, worst, 500*time.Millisecond)
	require.Zero(t, clock.correction)
}
