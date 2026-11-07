// Package timingfixture loads sanitized observed timing, never customer media.
package timingfixture

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type H264Metadata struct {
	PacketizationType int   `json:"packetization_type"`
	NALTypes          []int `json:"nal_types"`
	FUStart           bool  `json:"fu_start"`
	FUEnd             bool  `json:"fu_end"`
	ContainsSPS       bool  `json:"contains_sps"`
	ContainsPPS       bool  `json:"contains_pps"`
	ContainsIDR       bool  `json:"contains_idr"`
	ContainsVCL       bool  `json:"contains_vcl"`
}

// Raw video SSRC0, audio SSRC10, PT0 and uint32 RTP counters are meaningful.
// UnitNTPNS is nil because it was not observed.
type Event struct {
	EventIndex               int           `json:"event_index"`
	Kind                     string        `json:"kind"`
	StreamID                 string        `json:"stream_id"`
	Media                    string        `json:"media"`
	TrackID                  int           `json:"track_id"`
	PT                       uint8         `json:"pt"`
	SSRC                     uint32        `json:"ssrc"`
	CaptureFirstArrivalNS    int64         `json:"capture_first_arrival_ns"`
	CaptureCompleteArrivalNS int64         `json:"capture_complete_arrival_ns"`
	OrderedArrivalNS         int64         `json:"ordered_arrival_ns"`
	UnitNTPNS                *int64        `json:"unit_ntp_ns"`
	Sequence                 uint16        `json:"sequence"`
	RTPTimestamp             uint32        `json:"rtp_timestamp"`
	Marker                   bool          `json:"marker"`
	PayloadBytes             int           `json:"payload_bytes"`
	PayloadID                string        `json:"payload_id"`
	H264                     *H264Metadata `json:"h264"`
	InterleavedChannel       *int          `json:"interleaved_channel"`
	TCPRun                   *int          `json:"tcp_run"`
	TCPRelativeByteStart     *int64        `json:"tcp_relative_byte_start"`
	TCPRelativeByteEnd       *int64        `json:"tcp_relative_byte_end"`
	SourceIndex              *int          `json:"source_index"`
	SRRTPTimestamp           uint32        `json:"sr_rtp_timestamp"`
	SRNTPSeconds             uint32        `json:"sr_ntp_seconds"`
	SRNTPFraction            uint32        `json:"sr_ntp_fraction"`
	SRSenderPackets          uint32        `json:"sr_sender_packets"`
	SRSenderOctets           uint32        `json:"sr_sender_octets"`
}

type RecordedPacket struct {
	FileID      string `json:"file_id"`
	PacketIndex int    `json:"packet_index"`
	AudioIndex  int    `json:"audio_index"`
	LocalDTSNS  int64  `json:"local_dts_ns"`
	LocalPTSNS  int64  `json:"local_pts_ns"`
	DurationNS  int64  `json:"duration_ns"`
	PCMBytes    int    `json:"pcm_bytes"`
}

// AudioOutcome is an observed derived label. It is never consumed by Replay.
type AudioOutcome struct {
	PayloadID         string          `json:"payload_id"`
	SourceIndex       int             `json:"source_index"`
	PayloadSamples    int             `json:"payload_samples"`
	ObservedRecorded  bool            `json:"observed_recorded"`
	LabelProvenance   string          `json:"label_provenance"`
	ObservedUnitNTPNS *int64          `json:"observed_unit_ntp_ns"`
	Recorded          *RecordedPacket `json:"recorded"`
	MissingLabel      string          `json:"missing_label"`
}

func readJSONL(path string, visit func([]byte) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	scanner := bufio.NewScanner(gz)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for line := 1; scanner.Scan(); line++ {
		if err := visit(scanner.Bytes()); err != nil {
			return fmt.Errorf("%s line %d: %w", path, line, err)
		}
	}
	return scanner.Err()
}

func ReadEvents(path string, visit func(Event) error) error {
	index, previous := 0, int64(-1)
	return readJSONL(path, func(raw []byte) error {
		var e Event
		if err := json.Unmarshal(raw, &e); err != nil {
			return err
		}
		if e.EventIndex != index || e.OrderedArrivalNS < previous {
			return fmt.Errorf("event order mismatch")
		}
		if e.UnitNTPNS != nil {
			return fmt.Errorf("unobserved unit NTP was populated")
		}
		if e.Kind != "rtp" && e.Kind != "sr" {
			return fmt.Errorf("unknown event kind %q", e.Kind)
		}
		if e.Media != "audio" && e.Media != "video" {
			return fmt.Errorf("unknown media %q", e.Media)
		}
		if e.Media == "audio" && (e.PT != 0 || e.SSRC != 10) {
			return fmt.Errorf("v2 raw audio PT0/SSRC10 mismatch")
		}
		if e.Media == "video" && (e.PT != 99 || e.SSRC != 0) {
			return fmt.Errorf("v2 raw video PT99/SSRC0 mismatch")
		}
		if e.Kind == "sr" && e.SRNTPSeconds < 3155673600 {
			return fmt.Errorf("v2 positive2000 NTP epoch required; v1 normalization is superseded")
		}
		if e.Kind == "rtp" && (e.PayloadID == "" || e.PayloadBytes <= 0) {
			return fmt.Errorf("invalid synthetic payload description")
		}
		index, previous = index+1, e.OrderedArrivalNS
		return visit(e)
	})
}

func ReadOutcomes(path string, visit func(AudioOutcome) error) error {
	return readJSONL(path, func(raw []byte) error {
		var e AudioOutcome
		if err := json.Unmarshal(raw, &e); err != nil {
			return err
		}
		if e.ObservedUnitNTPNS != nil {
			return fmt.Errorf("unobserved unit NTP was populated")
		}
		if e.ObservedRecorded != (e.Recorded != nil) {
			return fmt.Errorf("outcome join mismatch")
		}
		return visit(e)
	})
}

// SyntheticPayload preserves size and equivalent-audio identity only. The output
// is opaque synthetic bytes, NOT a valid H264 bitstream or the observed PCMU.
// A real codec-path test must supply independent synthetic codec fixtures.
func SyntheticPayload(e Event) []byte {
	seed := sha256.Sum256([]byte("synthetic-timing-fixture:" + e.PayloadID))
	out := make([]byte, e.PayloadBytes)
	for i := range out {
		out[i] = seed[i%len(seed)] ^ byte(i/len(seed))
	}
	return out
}

// Sink is the local adapter seam. AdvanceTo is virtual capture time, not an
// observed per-unit NTP value. Initial clock/decoder state must be supplied by
// the caller and identified as an assumption; this capture is not bootstrap.
type Sink interface {
	AdvanceTo(relativeCaptureTime time.Duration) error
	RTP(e Event, syntheticPayload []byte) error
	SenderReport(e Event) error
}

func Replay(path string, sink Sink) error {
	manifest, err := os.ReadFile(filepath.Join(filepath.Dir(path), "manifest.json"))
	if err != nil {
		return err
	}
	var version struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(manifest, &version); err != nil {
		return err
	}
	if version.SchemaVersion != 2 {
		return fmt.Errorf("v2 fixture required: v1 counter normalization is superseded")
	}
	return ReadEvents(path, func(e Event) error {
		if err := sink.AdvanceTo(time.Duration(e.OrderedArrivalNS)); err != nil {
			return err
		}
		if e.Kind == "sr" {
			return sink.SenderReport(e)
		}
		return sink.RTP(e, SyntheticPayload(e))
	})
}
