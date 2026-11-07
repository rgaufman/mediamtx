// Package bootstrapfixture loads sanitized observed initial-session timing.
package bootstrapfixture

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type NALMetadata struct {
	Codec                string `json:"codec"`
	PacketizationType    int    `json:"packetization_type"`
	NALTypes             []int  `json:"nal_types"`
	FUStart              bool   `json:"fu_start"`
	FUEnd                bool   `json:"fu_end"`
	ContainsVPS          bool   `json:"contains_vps"`
	ContainsSPS          bool   `json:"contains_sps"`
	ContainsPPS          bool   `json:"contains_pps"`
	ContainsRandomAccess bool   `json:"contains_random_access"`
	ContainsVCL          bool   `json:"contains_vcl"`
}

// SSRC, PT, sequence, RTP time and SR RTP time are RAW, including valid zero.
// UnitNTPNS is nil: neither packet callback NTP nor producer arrival was captured.
type Event struct {
	EventIndex              int          `json:"event_index"`
	CaseID                  string       `json:"case_id"`
	PathID                  string       `json:"path_id"`
	Role                    string       `json:"role"`
	StreamID                string       `json:"stream_id"`
	Media                   string       `json:"media"`
	Codec                   string       `json:"codec"`
	ClockRate               int          `json:"clock_rate"`
	Kind                    string       `json:"kind"`
	ArrivalNS               int64        `json:"arrival_ns"`
	OrderedArrivalNS        int64        `json:"ordered_arrival_ns"`
	CaptureRecordIndex      int          `json:"capture_record_index"`
	PT                      uint8        `json:"pt"`
	SSRC                    uint32       `json:"ssrc"`
	UnitNTPNS               *int64       `json:"unit_ntp_ns"`
	ProducerArrivalObserved bool         `json:"producer_arrival_observed"`
	Sequence                uint16       `json:"sequence"`
	RTPTimestamp            uint32       `json:"rtp_timestamp"`
	Marker                  bool         `json:"marker"`
	PayloadBytes            int          `json:"payload_bytes"`
	RawRTPBytes             int          `json:"raw_rtp_bytes"`
	RTPHeaderBytes          int          `json:"rtp_header_bytes"`
	RTPPaddingBytes         int          `json:"rtp_padding_bytes"`
	PayloadID               string       `json:"payload_id"`
	PayloadSamples          int          `json:"payload_samples"`
	NAL                     *NALMetadata `json:"nal"`
	SRRTPTimestamp          uint32       `json:"sr_rtp_timestamp"`
	SRNTPSeconds            uint32       `json:"sr_ntp_seconds"`
	SRNTPFraction           uint32       `json:"sr_ntp_fraction"`
	SRSenderPackets         uint32       `json:"sr_sender_packets"`
	SRSenderOctets          uint32       `json:"sr_sender_octets"`
}

func ReadEvents(path string, visit func(Event) error) error {
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
	index, previous := 0, int64(-1)
	for scanner.Scan() {
		var e Event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			return err
		}
		if e.EventIndex != index || e.OrderedArrivalNS < previous {
			return fmt.Errorf("capture event order mismatch")
		}
		if e.UnitNTPNS != nil || e.ProducerArrivalObserved {
			return fmt.Errorf("unobserved callback state was populated")
		}
		if e.Kind != "rtp" && e.Kind != "sr" {
			return fmt.Errorf("unsupported event kind")
		}
		if e.Kind == "rtp" && (e.PayloadBytes <= 0 || e.PayloadID == "") {
			return fmt.Errorf("invalid synthetic payload metadata")
		}
		if e.Kind == "sr" && e.SRNTPSeconds < 3155673600 {
			return fmt.Errorf("positive2000 common whole-second epoch required")
		}
		index, previous = index+1, e.OrderedArrivalNS
		if err := visit(e); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// SyntheticPayload is opaque bytes of observed size, NOT original speech/NALs
// and NOT a codec-valid H264/H265 bitstream. Use NAL metadata in an explicit
// test adapter, or independent synthetic codec assets for a real decoder test.
func SyntheticPayload(e Event) []byte {
	seed := sha256.Sum256([]byte("synthetic-bootstrap:" + e.PayloadID))
	out := make([]byte, e.PayloadBytes)
	for i := range out {
		out[i] = seed[i%len(seed)] ^ byte(i/len(seed))
	}
	return out
}

// Route independently by CaseID+Role/PathID. Do not feed independent camera
// paths or main/sub alternatives into one decoder. AdvanceTo is wire capture
// time, not observed producer arrival or per-unit NTP. Session/control metadata
// and invocation precision are separate inputs; do not silently fabricate them.
type Sink interface {
	AdvanceTo(relativeWireTime time.Duration) error
	RTP(Event, []byte) error
	SenderReport(Event) error
}

func Replay(path string, sink Sink) error {
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
