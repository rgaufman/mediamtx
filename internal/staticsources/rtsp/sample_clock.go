package rtsp

import (
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/logger"
)

// sampleClockReportEvery is how often a track that is being retimed says so again, so the
// correction stays visible in the log for as long as it runs, not only at its start.
const sampleClockReportEvery = 10 * time.Minute

var sampleClockNow = time.Now

// sampleClockUnderAdvances is how many packets in a row must under-advance before the track is
// retimed, so a single timestamp glitch does not switch a sound stream over for the session.
const sampleClockUnderAdvances = 3

// sampleClock times fixed-width audio (G.711, LPCM, G.726) by its payload samples
// when the sender's RTP timestamps contradict those samples and their arrival rate.
//
// For these codecs a packet's sample count is its payload bit length divided by the negotiated
// bits per sample, and RFC 3550 timestamps advance by that count per packet: more across a silence
// gap or a lost packet, never less. A timestamp step smaller than the previous packet's
// samples (times the sequence step) means the packets overlap in time, which a real sender
// cannot produce. Measured on TetherBox 912d47ac8953c20ed67f, two Uniview cameras
// ("VCP IPC Realtime stream", PCMU/8000) do exactly that: an IPC3618SB-ADF28KM-I0 (firmware
// GIPC-B6203.10.6.230515) stamps every packet 0, and another advances 256 per 512-sample
// packet; both deliver 8000 samples per wall second. MediaMTX then timed the audio at 0x or
// 0.5x real time and the recorder reset the recording every ~5 s.
//
// Parking on 579515f6fb98aad67f59 has the opposite fault: G726-16 and G711 deliver
// 320 samples every 40 ms but stamp them 1024 ticks apart, then jump backwards
// 15360 ticks once per second. Three consecutive over-advances also trigger
// repair when their arrival interval agrees with the payload sample count.
// Real silence gaps and TCP bursts fail that arrival check and remain untouched.
//
// After either fault is confirmed, each packet's timestamp is
// rewritten as the previous one plus the previous packet's samples (times the sequence step,
// so a lost packet keeps its slot) for the rest of the session. A stream whose timestamps
// advance correctly is never touched, and the retiming is logged when it starts and every
// sampleClockReportEvery while it runs.
type sampleClock struct {
	bitsPerSample int
	clockRate     int
	onStart       func(samplesPerPacket uint32, timestampStep uint32)
	onReport      func(retimed uint64)

	initialized bool
	active      bool
	under       int
	prevSeq     uint16
	prevIn      uint32
	prevOut     uint32
	prevSamples uint32
	retimed     uint64
	lastReport  time.Time
	prevArrival time.Time
	over        int
	overStarted time.Time
	overSamples uint64
}

// newSampleClock returns nil for a format whose sample count cannot be read off its payload.
func newSampleClock(forma format.Format) *sampleClock {
	switch forma := forma.(type) {
	case *format.G711:
		return &sampleClock{bitsPerSample: 8 * forma.ChannelCount, clockRate: forma.ClockRate()}
	case *format.LPCM:
		return &sampleClock{bitsPerSample: forma.BitDepth * forma.ChannelCount, clockRate: forma.ClockRate()}
	case *format.G726:
		switch forma.BitRate {
		case 16, 24, 32, 40:
			return &sampleClock{bitsPerSample: forma.BitRate / 8, clockRate: forma.ClockRate()}
		}
	}
	return nil
}

func (c *sampleClock) process(pkt *rtp.Packet) {
	if c.bitsPerSample <= 0 {
		return
	}
	// A packed G726 codeword cannot straddle RTP packets. Do not invent a
	// fractional sample when an invalid payload does not align to its codewords.
	if len(pkt.Payload)*8%c.bitsPerSample != 0 {
		return
	}
	samples := uint32(len(pkt.Payload) * 8 / c.bitsPerSample)
	now := sampleClockNow()

	if !c.initialized {
		c.initialized = true
		c.prevArrival = now
		c.remember(pkt, pkt.Timestamp, samples)
		return
	}

	step := pkt.SequenceNumber - c.prevSeq

	// duplicate or reordered packet: place it relative to the newest one, keep the state.
	if step == 0 || step >= 0x8000 {
		if c.active {
			back := uint32(c.prevSeq - pkt.SequenceNumber)
			pkt.Timestamp = c.prevOut - back*samples
			c.retimed++
		}
		return
	}

	in := pkt.Timestamp

	if !c.active {
		expected := int64(c.prevSamples) * int64(step)
		if samples > 0 && c.prevSamples > 0 && int64(int32(in-c.prevIn)) < expected {
			c.under++
		} else {
			c.under = 0
		}

		// A forward timestamp gap can be legitimate silence suppression. Only
		// count an over-advance when consecutive packets arrive at their actual
		// sample rate. Parking's G726-16 sends 320 samples every 40 ms but stamps
		// them 1024 ticks apart, then jumps backwards once each second.
		if step == 1 && samples > 0 && expected > 0 && int64(int32(in-c.prevIn)) > 2*expected {
			if c.over == 0 {
				c.overStarted = c.prevArrival
				c.overSamples = 0
			}
			c.over++
			c.overSamples += uint64(c.prevSamples)
		} else {
			c.over = 0
		}
		var expectedArrival time.Duration
		if c.clockRate > 0 {
			expectedArrival = time.Duration(c.overSamples) * time.Second / time.Duration(c.clockRate)
		}
		elapsed := now.Sub(c.overStarted)
		overAdvances := c.clockRate > 0 && c.over >= sampleClockUnderAdvances &&
			elapsed >= expectedArrival*65/100 && elapsed <= expectedArrival*135/100
		if c.over >= sampleClockUnderAdvances && !overAdvances {
			c.over = 0
		}

		if c.under >= sampleClockUnderAdvances || overAdvances {
			c.active = true
			c.lastReport = sampleClockNow()
			c.onStart(c.prevSamples, in-c.prevIn)
		}
	}

	if c.active {
		pkt.Timestamp = c.prevOut + c.prevSamples*uint32(step)
		c.retimed++

		if now := sampleClockNow(); now.Sub(c.lastReport) >= sampleClockReportEvery {
			c.lastReport = now
			c.onReport(c.retimed)
		}
	}

	c.remember(pkt, in, samples)
	c.prevArrival = now
}

func (c *sampleClock) remember(pkt *rtp.Packet, in uint32, samples uint32) {
	c.prevSeq = pkt.SequenceNumber
	c.prevIn = in
	c.prevOut = pkt.Timestamp
	c.prevSamples = samples
}

// timestampRepairSource is the RTSP client as rtsp.ToStream sees it, with a sampleClock in
// front of every G.711 / LPCM / G.726 format and a wallClock in front of every audio and video format
// (sample repair disables sender-report correction for that track). Everything downstream
// (recorder, HLS, WebRTC, RTSP readers) receives the repaired timestamps.
type timestampRepairSource struct {
	*gortsplib.Client
	log           logger.Writer
	walls         map[*description.Media][]*wallClock
	preserveNTP   bool
	absoluteTimes sync.Map // Only populated during absolute-timestamp RTP callbacks.
}

type packetAbsoluteTime struct {
	media     *description.Media
	ntp       time.Time
	available bool
}

// PacketNTP returns the incoming packet's original sender mapping. ToStream
// queries it synchronously inside the RTP callback, after the RTP timestamp has
// been repaired. Passing the repaired value into Client.PacketNTP would apply
// the correction to the original sender-report coordinates a second time.
func (s *timestampRepairSource) PacketNTP(media *description.Media, packet *rtp.Packet) (time.Time, bool) {
	if !s.preserveNTP || (media.Type != description.MediaTypeAudio && media.Type != description.MediaTypeVideo) {
		return s.Client.PacketNTP(media, packet)
	}
	if value, ok := s.absoluteTimes.Load(packet); ok {
		saved := value.(packetAbsoluteTime)
		if saved.media == media {
			return saved.ntp, saved.available
		}
	}
	// A packet outside its callback lifetime cannot be mapped safely after repair.
	return time.Time{}, false
}

func (s *timestampRepairSource) OnPacketRTP(medi *description.Media, forma format.Format, cb gortsplib.OnPacketRTPFunc) {
	if medi.Type != description.MediaTypeAudio && medi.Type != description.MediaTypeVideo {
		s.Client.OnPacketRTP(medi, forma, cb)
		return
	}

	codec := forma.Codec()
	rate := forma.ClockRate()

	clock := newSampleClock(forma)
	if clock != nil {
		clock.onStart = func(samplesPerPacket uint32, timestampStep uint32) {
			s.log.Log(logger.Warn, "sample clock: %s %d Hz RTP timestamps advance %d per %d-sample packet; "+
				"timing the track by its sample count from now on",
				codec, rate, int32(timestampStep), samplesPerPacket)
		}
		clock.onReport = func(retimed uint64) {
			s.log.Log(logger.Warn, "sample clock: %s %d Hz still timed by sample count, %d packets retimed",
				codec, rate, retimed)
		}
	}

	wall := newWallClock(rate)
	if s.walls == nil {
		s.walls = make(map[*description.Media][]*wallClock)
	}
	if wall != nil {
		s.walls[medi] = append(s.walls[medi], wall)
		// Registered before PLAY, after every format is installed by ToStream.
		s.Client.OnPacketRTCP(medi, func(packet rtcp.Packet) {
			if report, ok := packet.(*rtcp.SenderReport); ok {
				for _, track := range s.walls[medi] {
					track.senderReport(report)
				}
			}
		})
	}
	if wall != nil {
		wall.onStart = func(ppm float64, drift time.Duration) {
			s.log.Log(logger.Warn, "wall clock: %s %d Hz RTP clock runs %+.0f ppm against sender NTP "+
				"(%v adrift); slewing its timestamps using sender reports", codec, rate, ppm, drift)
		}
		wall.onReport = func(ppm float64, correction time.Duration) {
			s.log.Log(logger.Warn, "wall clock: %s %d Hz still slewed using sender reports, RTP clock %+.0f ppm, correction %v",
				codec, rate, ppm, correction)
		}
		wall.onDistrust = func(disagreement, allowed, after time.Duration) {
			s.log.Log(logger.Warn, "wall clock: %s %d Hz sender NTP disagrees with arrival by %v (allowed %v after %v); "+
				"distrusting sender reports for this session", codec, rate, disagreement, allowed, after)
		}
	}

	if clock == nil && wall == nil {
		s.Client.OnPacketRTP(medi, forma, cb)
		return
	}

	s.Client.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
		if s.preserveNTP {
			ntp, available := s.Client.PacketNTP(medi, pkt)
			s.absoluteTimes.Store(pkt, packetAbsoluteTime{medi, ntp, available})
			defer s.absoluteTimes.Delete(pkt)
		}
		if clock != nil {
			clock.process(pkt)
		}
		if wall != nil {
			if clock != nil && clock.active {
				wall.disable()
			}
			wall.process(pkt)
		}
		cb(pkt)
	})
}
