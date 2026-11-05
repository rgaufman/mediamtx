package rtsp

import (
	"testing"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

type wallClockRun struct {
	starts  []float64
	reports int
	// worst |arrival - output media time - first arrival offset| after the settling period
	worstError time.Duration
	// the same for the timestamps as the camera sent them
	worstUncorrected time.Duration
	changed          bool
}

// simulate feeds `duration` of one-packet frames from a camera whose RTP clock runs `ppm` fast
// (positive) or slow against elapsed time, stamping `step` ticks per frame at `rate` Hz, with
// arrival jitter of 0-30 ms. Errors are measured after `settle`.
func simulate(t *testing.T, rate int, step uint32, ppm float64, duration, settle time.Duration) wallClockRun {
	start := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	now := start
	wallClockNow = func() time.Time { return now }
	defer func() { wallClockNow = time.Now }()

	clock := newWallClock(rate)
	require.NotNil(t, clock)
	run := wallClockRun{}
	clock.onStart = func(ppm float64, _ time.Duration) { run.starts = append(run.starts, ppm) }
	clock.onReport = func(float64, time.Duration) { run.reports++ }

	interval := time.Duration(float64(step) / float64(rate) / (1 + ppm*1e-6) * float64(time.Second))
	in := uint32(4294000000) // wraps during the run
	var outMedia, inMedia int64
	var prevOut, prevIn uint32
	var reportAt time.Duration

	for i := 0; time.Duration(i)*interval < duration; i++ {
		elapsed := time.Duration(i) * interval
		jitter := time.Duration((i*7919)%31) * time.Millisecond
		now = start.Add(elapsed + jitter)

		pkt := &rtp.Packet{Header: rtp.Header{SequenceNumber: uint16(i), Timestamp: in}}
		clock.process(pkt)
		if elapsed >= reportAt {
			clock.senderReport(&rtcp.SenderReport{NTPTime: auditNTP(elapsed), RTPTime: in})
			reportAt = elapsed + 5*time.Second
		}
		if pkt.Timestamp != in {
			run.changed = true
		}

		if i == 0 {
			prevOut, prevIn = pkt.Timestamp, in
		} else {
			outMedia += int64(int32(pkt.Timestamp - prevOut))
			inMedia += int64(int32(in - prevIn))
			prevOut, prevIn = pkt.Timestamp, in
		}

		if elapsed >= settle {
			corrected := elapsed - time.Duration(float64(outMedia)/float64(rate)*float64(time.Second))
			raw := elapsed - time.Duration(float64(inMedia)/float64(rate)*float64(time.Second))
			run.worstError = max(run.worstError, corrected.Abs())
			run.worstUncorrected = max(run.worstUncorrected, raw.Abs())
		}
		in += step
	}
	return run
}

// Video of a Uniview camera on 9abcd9bd738fc20c46aa: 3000 ticks per frame at 90 kHz while the
// frames arrive at 30.048 per second, +1595 ppm (bin/check_recording --rtp-clock 600).
func TestWallClockLocksAFastVideoClock(t *testing.T) {
	run := simulate(t, 90000, 3000, 1595, 30*time.Minute, 3*time.Minute)

	require.Greater(t, run.worstUncorrected, 2500*time.Millisecond) // the uncorrected tracks part
	require.Less(t, run.worstError, 150*time.Millisecond)
	require.Len(t, run.starts, 1)
	require.InDelta(t, 1595, run.starts[0], 300)
	require.Equal(t, 2, run.reports) // at 10 and 20 minutes after it started
}

// Video of 68eb66ab585cfffc318c6282 on 912d47ac8953c20ed67f: 7500 ticks per frame, -278 ppm.
func TestWallClockLocksASlowVideoClock(t *testing.T) {
	run := simulate(t, 90000, 7500, -278, 2*time.Hour, 10*time.Minute)

	require.Greater(t, run.worstUncorrected, 1900*time.Millisecond)
	require.Less(t, run.worstError, 150*time.Millisecond)
	require.Len(t, run.starts, 1)
	require.Less(t, run.starts[0], 0.0)
}

// The audio of the same 9abcd camera: 384-sample PCMU packets at 16 kHz, -5 ppm. Within the
// deadband for this half hour, so its timestamps are never touched.
func TestWallClockLeavesAnAccurateClockAlone(t *testing.T) {
	run := simulate(t, 16000, 384, -5, 30*time.Minute, 0)

	require.False(t, run.changed)
	require.Empty(t, run.starts)
}

// A camera that stops sending for 30 s and resumes where its RTP clock left off is a pause, not
// a clock rate: no slewing, timestamps untouched, and sender evidence is re-anchored.
func TestWallClockTreatsAPauseAsAStep(t *testing.T) {
	start := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	now := start
	wallClockNow = func() time.Time { return now }
	defer func() { wallClockNow = time.Now }()

	clock := newWallClock(90000)
	var starts int
	clock.onStart = func(float64, time.Duration) { starts++ }
	clock.onReport = func(float64, time.Duration) {}

	for frame := range uint32(3 * 60 * 30) {
		at := time.Duration(frame) * time.Second / 30
		if frame >= 60*30 {
			at += 30 * time.Second
		}
		now = start.Add(at)
		pkt := &rtp.Packet{Header: rtp.Header{Timestamp: frame * 3000}}
		clock.process(pkt)
		if frame%150 == 0 {
			clock.senderReport(&rtcp.SenderReport{NTPTime: auditNTP(at), RTPTime: frame * 3000})
		}
		require.Equal(t, frame*3000, pkt.Timestamp)
	}
	require.False(t, clock.active)
	require.Zero(t, starts)
}

func TestWallClockKeepsAFramesPacketsTogether(t *testing.T) {
	start := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	now := start
	wallClockNow = func() time.Time { return now }
	defer func() { wallClockNow = time.Now }()

	clock := newWallClock(90000)
	clock.onStart = func(float64, time.Duration) {}
	clock.onReport = func(float64, time.Duration) {}

	// a camera approximately 0.7% fast, so the lock is slewing while frames of several packets go through
	for frame := range uint32(3000) {
		var first uint32
		for part := range 3 {
			now = start.Add(time.Duration(frame) * 33100 * time.Microsecond)
			pkt := &rtp.Packet{Header: rtp.Header{Timestamp: frame * 3000}}
			clock.process(pkt)
			if part == 0 && frame%150 == 0 {
				clock.senderReport(&rtcp.SenderReport{NTPTime: auditNTP(now.Sub(start)), RTPTime: frame * 3000})
			}
			if part == 0 {
				first = pkt.Timestamp
			}
			require.Equal(t, first, pkt.Timestamp, "frame %d part %d", frame, part)
		}
	}
	require.True(t, clock.active)
}
