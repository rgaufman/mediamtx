package rtsp

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

type sampleClockLog struct {
	starts  []uint32
	reports []uint64
}

func newTestSampleClock(t *testing.T, forma format.Format) (*sampleClock, *sampleClockLog) {
	clock := newSampleClock(forma)
	require.NotNil(t, clock)
	log := &sampleClockLog{}
	clock.onStart = func(samples uint32, _ uint32) { log.starts = append(log.starts, samples) }
	clock.onReport = func(retimed uint64) { log.reports = append(log.reports, retimed) }
	return clock, log
}

func feed(clock *sampleClock, seqs []uint16, timestamps []uint32, payload int) []uint32 {
	out := make([]uint32, len(seqs))
	for i := range seqs {
		pkt := &rtp.Packet{
			Header:  rtp.Header{SequenceNumber: seqs[i], Timestamp: timestamps[i]},
			Payload: make([]byte, payload),
		}
		clock.process(pkt)
		out[i] = pkt.Timestamp
	}
	return out
}

// The first 13 audio packets of a Uniview IPC3618SB-ADF28KM-I0 (fw GIPC-B6203.10.6.230515),
// captured raw on TetherBox 912d47ac8953c20ed67f, camera 68eb66ab585cfffc318c6291,
// by `bin/check_recording --rtp-clock-raw`:
// PCMU/8000, 256-byte payloads, sequence numbers advancing, every timestamp 0.
func TestSampleClockRetimesTimestampsThatNeverAdvance(t *testing.T) {
	clock, log := newTestSampleClock(t, &format.G711{PayloadTyp: 0, MULaw: true, SampleRate: 8000, ChannelCount: 1})

	seqs := []uint16{15533, 15534, 15535, 15536, 15537, 15538, 15539, 15540, 15541, 15542, 15543, 15544, 15545}
	out := feed(clock, seqs, make([]uint32, len(seqs)), 256)

	// the first three keep the camera's 0 while the under-advance is counted
	require.Equal(t, []uint32{0, 0, 0}, out[:3])
	for i := 3; i < len(out); i++ {
		require.Equal(t, uint32(256*(i-2)), out[i], "packet %d", i)
	}
	require.Equal(t, []uint32{256}, log.starts)
}

// The other camera on 912d47ac8953c20ed67f (68eb66ab585cfffc318c6282), read the same way over
// 600 s: 512-byte PCMU packets whose timestamps advance 256 each, 8000 samples per wall second.
func TestSampleClockRetimesTimestampsAdvancingAtHalfRate(t *testing.T) {
	clock, log := newTestSampleClock(t, &format.G711{PayloadTyp: 0, MULaw: true, SampleRate: 8000, ChannelCount: 1})

	in := []uint32{1050083840, 1050084096, 1050084352, 1050084608, 1050084864, 1050085120}
	out := feed(clock, []uint16{1, 2, 3, 4, 5, 6}, in, 512)

	require.Equal(t, []uint32{1050083840, 1050084096, 1050084352, 1050084352 + 512, 1050084352 + 1024, 1050084352 + 1536}, out)
	require.Equal(t, []uint32{512}, log.starts)
}

func TestSampleClockIgnoresOneTimestampGlitch(t *testing.T) {
	clock, log := newTestSampleClock(t, &format.G711{PayloadTyp: 0, MULaw: true, SampleRate: 8000, ChannelCount: 1})

	in := []uint32{0, 160, 160, 480, 640, 800}
	out := feed(clock, []uint16{1, 2, 3, 4, 5, 6}, in, 160)

	require.Equal(t, in, out)
	require.Empty(t, log.starts)
}

func TestSampleClockLeavesAdvancingTimestampsAlone(t *testing.T) {
	clock, log := newTestSampleClock(t, &format.G711{PayloadTyp: 127, MULaw: true, SampleRate: 16000, ChannelCount: 1})

	seqs := []uint16{65534, 65535, 0, 1, 3} // wraps, and one packet lost
	in := []uint32{4294966000, 4294966320, 344, 664, 1304}
	out := feed(clock, seqs, in, 320)

	require.Equal(t, in, out)
	require.Empty(t, log.starts)
}

func TestSampleClockKeepsTheSlotOfALostPacket(t *testing.T) {
	clock, _ := newTestSampleClock(t, &format.G711{PayloadTyp: 8, SampleRate: 8000, ChannelCount: 1})

	out := feed(clock, []uint16{10, 11, 12, 13, 14, 16, 15}, []uint32{0, 0, 0, 0, 0, 0, 0}, 160)

	// 16 follows a lost 15 two slots on; 15 then arrives late and is placed one packet before 16.
	require.Equal(t, []uint32{0, 0, 0, 160, 320, 640, 480}, out)
}

func TestSampleClockCountsLPCMByBitDepthAndChannels(t *testing.T) {
	clock, log := newTestSampleClock(t, &format.LPCM{PayloadTyp: 96, BitDepth: 16, SampleRate: 48000, ChannelCount: 2})

	out := feed(clock, []uint16{1, 2, 3, 4, 5}, []uint32{7, 7, 7, 7, 7}, 1920)

	require.Equal(t, []uint32{7, 7, 7, 487, 967}, out)
	require.Equal(t, []uint32{480}, log.starts)
}

func TestSampleClockKeepsReporting(t *testing.T) {
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	sampleClockNow = func() time.Time { return now }
	defer func() { sampleClockNow = time.Now }()

	clock, log := newTestSampleClock(t, &format.G711{PayloadTyp: 0, MULaw: true, SampleRate: 8000, ChannelCount: 1})
	feed(clock, []uint16{1, 2, 3, 4, 5}, []uint32{0, 0, 0, 0, 0}, 256)
	require.Empty(t, log.reports)

	now = now.Add(sampleClockReportEvery)
	feed(clock, []uint16{6}, []uint32{0}, 256)
	require.Equal(t, []uint64{3}, log.reports)
}

func TestSampleClockIgnoresCompressedAudio(t *testing.T) {
	require.Nil(t, newSampleClock(&format.MPEG4Audio{}))
}

// Raw Parking camera RTP, 2026-10-19, captured before MediaMTX/FFmpeg. The
// G711 trial reproduced the identical timestamps with 320-byte payloads.
func TestSampleClockRepairsCapturedParkingSawtooth(t *testing.T) {
	data, err := os.ReadFile("testdata/parking-g726-sawtooth.json")
	require.NoError(t, err)
	var capture struct {
		Records []struct {
			Channel   int     `json:"channel"`
			Arrival   float64 `json:"arrival"`
			Sequence  uint16  `json:"seq"`
			Timestamp uint32  `json:"timestamp"`
			Payload   int     `json:"payload"`
		} `json:"records"`
	}
	require.NoError(t, json.Unmarshal(data, &capture))
	for _, codec := range []string{"G726", "G711"} {
		t.Run(codec, func(t *testing.T) {
			var forma format.Format = &format.G726{PayloadTyp: 102, BitRate: 16}
			if codec == "G711" {
				forma = &format.G711{PayloadTyp: 0, SampleRate: 8000, ChannelCount: 1}
			}
			clock, log := newTestSampleClock(t, forma)
			var now time.Time
			sampleClockNow = func() time.Time { return now }
			defer func() { sampleClockNow = time.Now }()
			var previous, first uint32
			var firstArrival float64
			var maxLead float64
			packets := 0
			for _, record := range capture.Records {
				if record.Channel != 2 {
					continue
				}
				if packets == 0 {
					first, firstArrival = record.Timestamp, record.Arrival
				}
				now = time.Unix(0, int64((record.Arrival-firstArrival)*float64(time.Second)))
				payload := record.Payload
				if codec == "G711" {
					payload *= 4
				}
				pkt := &rtp.Packet{Header: rtp.Header{SequenceNumber: record.Sequence, Timestamp: record.Timestamp}, Payload: make([]byte, payload)}
				clock.process(pkt)
				if clock.active {
					require.Equal(t, uint32(320), pkt.Timestamp-previous)
				}
				if packets > 3 {
					lead := float64(pkt.Timestamp-first)/8000 - (record.Arrival - firstArrival)
					if lead > maxLead {
						maxLead = lead
					}
					// Arrival time includes TCP burst jitter. Bound the offset
					// introduced by repair against the actual sample timeline.
					sampleLead := float64(pkt.Timestamp-first)/8000 - float64(packets*320)/8000
					require.Less(t, sampleLead, 0.2)
					require.GreaterOrEqual(t, sampleLead, 0.0)
				}
				previous = pkt.Timestamp
				packets++
			}
			require.Equal(t, []uint32{320}, log.starts)
			require.Greater(t, packets, 150)
			t.Logf("retimed %d packets; maximum arrival-relative media lead %.3f seconds", clock.retimed, maxLead)
		})
	}
}

func TestSampleClockDoesNotRetimestampGapsOrTCPBursts(t *testing.T) {
	for _, arrivalStep := range []time.Duration{0, 128 * time.Millisecond} {
		t.Run(arrivalStep.String(), func(t *testing.T) {
			clock, log := newTestSampleClock(t, &format.G726{BitRate: 16})
			now := time.Unix(0, 0)
			sampleClockNow = func() time.Time { return now }
			defer func() { sampleClockNow = time.Now }()
			for i := range 10 {
				out := feed(clock, []uint16{uint16(i)}, []uint32{uint32(i * 1024)}, 80)
				require.Equal(t, uint32(i*1024), out[0])
				now = now.Add(arrivalStep)
			}
			require.Empty(t, log.starts)
		})
	}
}

func TestSampleClockCountsNegotiatedG726Codewords(t *testing.T) {
	for _, bitrate := range []int{16, 24, 32, 40} {
		clock, log := newTestSampleClock(t, &format.G726{BitRate: bitrate})
		out := feed(clock, []uint16{1, 2, 3, 4, 5}, []uint32{0, 0, 0, 0, 0}, bitrate*5)
		require.Equal(t, uint32(320), out[3])
		require.Equal(t, []uint32{320}, log.starts)
	}
	require.Nil(t, newSampleClock(&format.G726{BitRate: 0}))
}

func TestSampleClockDoesNotJoinABurstToALaterPause(t *testing.T) {
	clock, log := newTestSampleClock(t, &format.G726{BitRate: 16})
	now := time.Unix(0, 0)
	sampleClockNow = func() time.Time { return now }
	defer func() { sampleClockNow = time.Now }()
	for i := range 4 {
		feed(clock, []uint16{uint16(i)}, []uint32{uint32(i * 1024)}, 80)
	}
	now = now.Add(120 * time.Millisecond)
	feed(clock, []uint16{4}, []uint32{4096}, 80)
	require.Empty(t, log.starts)
}

func TestSampleClockIgnoresMalformedPackedG726Payload(t *testing.T) {
	clock, log := newTestSampleClock(t, &format.G726{BitRate: 24})
	// 80 bytes is not a whole number of 3-bit codewords; 120 bytes is.
	out := feed(clock, []uint16{1, 2, 3, 4, 5}, []uint32{0, 0, 0, 0, 0}, 80)
	require.Equal(t, []uint32{0, 0, 0, 0, 0}, out)
	require.False(t, clock.initialized)
	require.Empty(t, log.starts)
}
