package timingfixture

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestHeaderAndOutcomeInvariants(t *testing.T) {
	counts := map[string]int{}
	audioIDs := map[string]Event{}
	if err := ReadEvents("events.jsonl.gz", func(e Event) error {
		counts[e.Kind+"_"+e.Media]++
		if e.Media == "video" && (e.PT != 99 || e.SSRC != 0) {
			t.Fatal("video PT/zero SSRC changed")
		}
		if e.Media == "audio" && (e.PT != 0 || e.SSRC != 10) {
			t.Fatal("audio raw PT0/SSRC10 changed")
		}
		if e.Kind == "rtp" && e.Media == "audio" {
			if e.PayloadBytes != 1000 {
				t.Fatal("audio sample count changed")
			}
			audioIDs[e.PayloadID] = e
		}
		if e.Kind == "rtp" && e.Media == "video" && e.H264 == nil {
			t.Fatal("NAL eligibility metadata absent")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for key, expected := range map[string]int{"rtp_video": 46430, "rtp_audio": 1437, "sr_video": 180, "sr_audio": 28} {
		if counts[key] != expected {
			t.Fatalf("%s: %d != %d", key, counts[key], expected)
		}
	}
	retained, missing := 0, 0
	if err := ReadOutcomes("observed-audio-outcomes.jsonl.gz", func(o AudioOutcome) error {
		if _, ok := audioIDs[o.PayloadID]; !ok {
			t.Fatal("unknown observed audio ID")
		}
		if o.ObservedRecorded {
			retained++
		} else {
			missing++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if retained != 1023 || missing != 414 {
		t.Fatalf("outcomes %d/%d", retained, missing)
	}
	aliases := 0
	if err := ReadEvents("equivalent-audio-events.jsonl.gz", func(e Event) error {
		original, ok := audioIDs[e.PayloadID]
		if !ok || !bytes.Equal(SyntheticPayload(original), SyntheticPayload(e)) {
			t.Fatal("equivalent audio identity changed")
		}
		aliases++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if aliases != 2874 {
		t.Fatalf("alias event count %d", aliases)
	}
}

type countingSink struct {
	rtp, audio, sr int
	previous       time.Duration
}

func (s *countingSink) AdvanceTo(now time.Duration) error {
	if now < s.previous {
		panic("virtual time moved backwards")
	}
	s.previous = now
	return nil
}
func (s *countingSink) RTP(e Event, payload []byte) error {
	if len(payload) != e.PayloadBytes {
		panic("synthetic size changed")
	}
	s.rtp++
	if e.Media == "audio" {
		s.audio++
	}
	return nil
}
func (s *countingSink) SenderReport(e Event) error { s.sr++; return nil }
func TestReplayDoesNotInjectOutcomeLabels(t *testing.T) {
	sink := &countingSink{}
	if err := Replay("events.jsonl.gz", sink); err != nil {
		t.Fatal(err)
	}
	if sink.rtp != 47867 || sink.audio != 1437 || sink.sr != 208 {
		t.Fatalf("replay counts %d/%d/%d", sink.rtp, sink.audio, sink.sr)
	}
}

func TestRawInitialCountersAndPositive2000Epoch(t *testing.T) {
	data, err := os.ReadFile("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		SchemaVersion     int `json:"schema_version"`
		RawInitialHeaders map[string]struct {
			Seq          uint16 `json:"seq"`
			RTPTimestamp uint32 `json:"rtp_timestamp"`
			SSRC         uint32 `json:"ssrc"`
		} `json:"raw_initial_headers"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 2 {
		t.Fatal("superseded fixture version")
	}
	seen := map[string]bool{}
	if err := ReadEvents("events.jsonl.gz", func(e Event) error {
		if e.Kind == "rtp" && !seen[e.Media] {
			want := manifest.RawInitialHeaders[e.Media]
			if e.Sequence != want.Seq || e.RTPTimestamp != want.RTPTimestamp || e.SSRC != want.SSRC {
				t.Fatal("raw initial header changed")
			}
			seen[e.Media] = true
		}
		if e.Kind == "sr" && (e.SRNTPSeconds < 3155673600 || e.SRNTPSeconds >= 3155760000) {
			t.Fatal("SR not in positive2000 epoch")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
