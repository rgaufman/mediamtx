package rtsp

import (
	"testing"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func auditNTP(elapsed time.Duration) uint64 {
	return uint64(3900000000)<<32 + uint64(float64(elapsed)/float64(time.Second)*4294967296)
}

// Synthetic regression; no customer serial/camera is attributed to network delay.
func TestClockQueueDelayDoesNotTriggerCorrection(t *testing.T) {
	for _, reports := range []bool{false, true} {
		t.Run(map[bool]string{false: "no-reports", true: "healthy-reports"}[reports], func(t *testing.T) {
			start := time.Unix(1700000000, 0)
			now := start
			wallClockNow = func() time.Time { return now }
			defer func() { wallClockNow = time.Now }()
			c := newWallClock(90000)
			for frame := range 18000 {
				media := time.Duration(frame) * time.Second / 30
				now = start.Add(media + media/1000) // queue grows 600 ms over ten minutes.
				ts := uint32(frame * 3000)
				pkt := &rtp.Packet{Header: rtp.Header{Timestamp: ts, SSRC: 1}}
				c.process(pkt)
				if reports && frame%150 == 0 {
					c.senderReport(&rtcp.SenderReport{SSRC: 1, NTPTime: auditNTP(media), RTPTime: ts})
				}
				require.Equal(t, ts, pkt.Timestamp)
			}
			require.False(t, c.active)
			require.Zero(t, c.correction)
		})
	}
}

// TetherBox 9abcd9bd738fc20c46aa, camera 6aad85cd4d716f7d3efbee7c:
// measured video drift approximately +1600 ppm against sender NTP.
func TestClockSenderDriftAndQueue(t *testing.T) {
	for _, ppm := range []int64{-1600, 1600} {
		t.Run(map[int64]string{-1600: "slow", 1600: "fast"}[ppm], func(t *testing.T) {
			start := time.Unix(1700000000, 0)
			now := start
			wallClockNow = func() time.Time { return now }
			defer func() { wallClockNow = time.Now }()
			c := newWallClock(90000)
			var last uint32
			for frame := range 18000 {
				elapsed := time.Duration(frame) * time.Second / 30
				now = start.Add(elapsed + elapsed/1000)
				// Start near wraparound to exercise both signed RTP differences and rollover.
				ts := uint32(0xffff0000 + int64(frame)*3000*(1000000+ppm)/1000000)
				pkt := &rtp.Packet{Header: rtp.Header{Timestamp: ts, SSRC: 7}}
				c.process(pkt)
				if frame%150 == 0 {
					c.senderReport(&rtcp.SenderReport{SSRC: 7, NTPTime: auditNTP(elapsed), RTPTime: ts})
				}
				if frame > 0 {
					require.Positive(t, int32(pkt.Timestamp-last))
				}
				duplicate := &rtp.Packet{Header: rtp.Header{Timestamp: ts, SSRC: 7}}
				c.process(duplicate)
				require.Equal(t, pkt.Timestamp, duplicate.Timestamp)
				last = pkt.Timestamp
			}
			require.True(t, c.active)
			require.InDelta(t, float64(ppm), c.ppmValue, 1)
			// Reports lag by at most five seconds; no queue delay is incorporated.
			require.InDelta(t, -float64(ppm)*600/1e6, c.duration(c.correction).Seconds(), 0.02)
			frozen := c.correction
			now = now.Add(31 * time.Second)
			c.process(&rtp.Packet{Header: rtp.Header{SSRC: 7, Timestamp: c.lastIn + 3000}})
			require.Equal(t, frozen, c.correction, "stale reports must freeze existing correction")
			// Resume with a new anchor; missing-report time is never extrapolated.
			c.senderReport(&rtcp.SenderReport{SSRC: 7, NTPTime: auditNTP(700 * time.Second), RTPTime: c.lastIn})
			require.Equal(t, frozen, c.target)
			c.disable()
			c.senderReport(&rtcp.SenderReport{SSRC: 7, NTPTime: auditNTP(705 * time.Second), RTPTime: c.lastIn + 450720})
			require.Equal(t, frozen, c.target, "sample-count repair must disable original-RTP evidence")
		})
	}
}

func TestClockInvalidSenderReports(t *testing.T) {
	start := time.Unix(1700000000, 0)
	now := start
	wallClockNow = func() time.Time { return now }
	defer func() { wallClockNow = time.Now }()
	c := newWallClock(90000)
	c.process(&rtp.Packet{Header: rtp.Header{SSRC: 1}})
	c.senderReport(&rtcp.SenderReport{SSRC: 2, NTPTime: auditNTP(0)})
	require.False(t, c.reportSet, "foreign SSRC")
	for i := range 120 {
		now = start.Add(time.Duration(i) * 5 * time.Second)
		// Clock steps by one second per report: reject rather than retime.
		c.senderReport(&rtcp.SenderReport{
			SSRC: 1, NTPTime: auditNTP(time.Duration(i) * 6 * time.Second), RTPTime: uint32(i * 450000),
		})
	}
	require.False(t, c.active)
	c.senderReport(&rtcp.SenderReport{SSRC: 1, NTPTime: auditNTP(0)})
	require.False(t, c.active, "reordered report")
	require.Zero(t, c.correction)
}
