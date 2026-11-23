package rtsp

import (
	"math"
	"sync"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

const (
	wallClockDeadband     = 50 * time.Millisecond
	wallClockMaxSlew      = 0.005
	wallClockReportMaxAge = 30 * time.Second
	wallClockEvidenceSpan = 30 * time.Second

	// Arrival is only a witness: it never sets a correction, it vetoes one. Its drift is the
	// minimum of arrival minus media time per window (network queueing only delays a packet,
	// so a window's minimum is its least-delayed packet), against the first window's minimum.
	wallClockArrivalWindow = 10 * time.Second
	// A change between two windows no clock rate can produce (500 ms in 10 s is 50000 ppm) is
	// a pause or a discontinuity; the arrival baseline absorbs it.
	wallClockArrivalStep = 500 * time.Millisecond
	// How far the sender-report correction may disagree with arrival before the sender's NTP
	// clock is distrusted. Two things explain a disagreement: a growing network queue (250 ms
	// plus up to 1200 ppm, at most 1 s; a queue past 1 s would itself push tracks outside the
	// recorder's 1 s cross-track window) and a camera NTP clock that runs off real time
	// (300 ppm, several times a free-running crystal; observed sender NTP remained
	// within about 200 ppm while video RTP ran +4200 ppm).
	wallClockArrivalBase       = 250 * time.Millisecond
	wallClockArrivalQueueRate  = 1200e-6
	wallClockArrivalQueueLimit = time.Second
	wallClockArrivalClockRate  = 300e-6
)

var wallClockNow = time.Now

// wallClock corrects only drift demonstrated by the sender's RTP/NTP mapping.
// Arrival times cannot distinguish oscillator drift from a growing network queue.
// Sender reports measure media time against the sender's clock independently of
// transport delay. Missing, stale or implausible reports freeze the correction.
// This trusts the sender's NTP clock; it cannot repair a sender whose RTP and NTP
// clocks share the same error. Fixed-sample audio repair is independent and takes
// precedence, since original sender reports cannot describe its rewritten RTP.
//
// Sender NTP is checked against a windowed arrival witness before correction.
// The coarse-sender regression fixture demonstrates near-real-time RTP arrivals
// paired with quantized sender reports that would accumulate seconds of false
// correction. Disagreement beyond the queue/clock allowance distrusts that
// sender for the session and slews the correction back to its native timeline.
type wallClock struct {
	mu                 sync.Mutex // UDP RTP and RTCP callbacks can run concurrently.
	clockRate          int64
	onStart            func(float64, time.Duration)
	onReport           func(float64, time.Duration)
	onDistrust         func(disagreement, allowed, after time.Duration)
	initialized        bool
	distrusted         bool
	firstArrival       time.Time
	arrivalMedia       int64 // input ticks since the session's first packet
	windowStart        time.Time
	windowMin          time.Duration
	arrivalBase        time.Duration
	arrivalDrift       time.Duration
	arrivalReady       bool // a window has closed after the baseline window
	arrivalBaseSet     bool
	disabled           bool
	ssrc               uint32
	lastIn, lastOut    uint32
	correction, target int64
	active             bool
	lastLog            time.Time
	reportSet          bool
	firstNTP, prevNTP  uint64
	prevRTP            uint32
	reportTicks        int64
	reportBase         int64
	lastReport         time.Time
	ppmValue           float64
}

func newWallClock(rate int) *wallClock {
	if rate <= 0 {
		return nil
	}
	return &wallClock{clockRate: int64(rate)}
}

func (c *wallClock) ticks(d time.Duration) int64 {
	return int64(math.Round(d.Seconds() * float64(c.clockRate)))
}

func (c *wallClock) duration(ticks int64) time.Duration {
	return time.Duration(float64(ticks) / float64(c.clockRate) * float64(time.Second))
}

func ntpElapsed(a, b uint64) time.Duration {
	// Signed difference also handles the NTP seconds era rollover for short spans.
	return time.Duration(float64(int64(a-b)) / 4294967296 * float64(time.Second))
}

func (c *wallClock) senderReport(report *rtcp.SenderReport) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.disabled || c.distrusted || !c.initialized || report.SSRC != c.ssrc || report.NTPTime == 0 {
		return
	}
	now := wallClockNow()
	if !c.reportSet || now.Sub(c.lastReport) > wallClockReportMaxAge {
		c.resetReport(report, now)
		return
	}
	elapsed := ntpElapsed(report.NTPTime, c.prevNTP)
	media := c.duration(int64(int32(report.RTPTime - c.prevRTP)))
	// Reject duplicate/reordered reports without throwing away good evidence.
	if elapsed <= 0 {
		return
	}
	// Pauses, restarts and implausible NTP/RTP jumps invalidate the evidence.
	if elapsed > wallClockReportMaxAge || media <= 0 ||
		math.Abs(float64(media-elapsed)) > float64(elapsed)*0.01 {
		c.resetReport(report, now)
		return
	}
	c.reportTicks += int64(int32(report.RTPTime - c.prevRTP))
	c.prevNTP, c.prevRTP, c.lastReport = report.NTPTime, report.RTPTime, now
	span := ntpElapsed(report.NTPTime, c.firstNTP)
	if span < wallClockEvidenceSpan {
		return
	}
	drift := span - c.duration(c.reportTicks)
	c.ppmValue = -float64(drift) / float64(span) * 1e6
	if !c.active && drift.Abs() < wallClockDeadband {
		return
	}
	target := c.reportBase + c.ticks(drift)
	if !c.arrivalReady {
		// No arrival witness yet: nothing moves until there is one.
		return
	}
	after := now.Sub(c.firstArrival)
	allowed := min(wallClockArrivalQueueLimit,
		wallClockArrivalBase+time.Duration(wallClockArrivalQueueRate*float64(after))) +
		time.Duration(wallClockArrivalClockRate*float64(after))
	if disagreement := c.duration(target) - c.arrivalDrift; disagreement.Abs() > allowed {
		c.distrusted = true
		c.target = 0
		if c.onDistrust != nil {
			c.onDistrust(disagreement, allowed, after)
		}
		return
	}
	c.target = target
	if !c.active {
		c.active = true
		c.lastLog = now
		if c.onStart != nil {
			c.onStart(c.ppmValue, drift)
		}
	}
}

func (c *wallClock) resetReport(report *rtcp.SenderReport, now time.Time) {
	c.reportSet = true
	c.firstNTP, c.prevNTP, c.prevRTP = report.NTPTime, report.NTPTime, report.RTPTime
	c.reportTicks = 0
	c.lastReport = now
	c.reportBase = c.correction
	c.target = c.correction
}

func (c *wallClock) disable() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.disabled = true
	c.target = c.correction
}

// witness tracks how far the input timestamps have drifted from arrival. It follows the same
// envelope as lastIn: only advancing timestamps count, so B-frame reordering is not drift.
func (c *wallClock) witness(now time.Time, step int64) {
	if step <= 0 {
		return
	}
	c.arrivalMedia += step
	offset := now.Sub(c.firstArrival) - c.duration(c.arrivalMedia)
	c.windowMin = min(c.windowMin, offset)
	if now.Sub(c.windowStart) < wallClockArrivalWindow {
		return
	}
	windowMin := c.windowMin
	c.windowStart = now
	c.windowMin = time.Duration(math.MaxInt64)
	if !c.arrivalBaseSet {
		c.arrivalBase, c.arrivalBaseSet = windowMin, true
		return
	}
	drift := windowMin - c.arrivalBase
	if jump := drift - c.arrivalDrift; jump.Abs() > wallClockArrivalStep {
		c.arrivalBase += jump
		drift = c.arrivalDrift
	}
	c.arrivalDrift, c.arrivalReady = drift, true
}

func (c *wallClock) process(pkt *rtp.Packet) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := wallClockNow()
	if !c.initialized || c.ssrc != pkt.SSRC {
		c.initialized = true
		c.ssrc = pkt.SSRC
		c.lastIn, c.lastOut = pkt.Timestamp, pkt.Timestamp
		c.reportSet = false
		c.correction, c.target = 0, 0
		c.active = false
		c.distrusted = false
		c.firstArrival, c.windowStart = now, now
		c.arrivalMedia = 0
		c.windowMin = time.Duration(math.MaxInt64)
		c.arrivalBaseSet, c.arrivalReady = false, false
		c.arrivalDrift = 0
		return
	}
	if pkt.Timestamp == c.lastIn {
		pkt.Timestamp = c.lastOut
		return
	}
	in := pkt.Timestamp
	step := int64(int32(in - c.lastIn))
	c.witness(now, step)
	fresh := c.reportSet && now.Sub(c.lastReport) <= wallClockReportMaxAge
	// A distrusted clock returns to the camera's own timestamps whether or not reports arrive.
	if !c.disabled && (c.distrusted || fresh) && step > 0 {
		limit := int64(math.Max(1, wallClockMaxSlew*float64(step)))
		delta := max(-limit, min(limit, c.target-c.correction))
		c.correction += delta
	}
	pkt.Timestamp = uint32(int64(in) + c.correction)
	if step > 0 {
		c.lastIn, c.lastOut = in, pkt.Timestamp
	}
	if c.active && now.Sub(c.lastLog) >= sampleClockReportEvery {
		c.lastLog = now
		if c.onReport != nil {
			c.onReport(c.ppmValue, c.duration(c.correction))
		}
	}
}
