package bootstrapfixture

import "testing"

func TestObservedHeadersAndSeparatePaths(t *testing.T) {
	counts := map[string]int{}
	paths := map[string]bool{}
	audioMain := map[string]bool{}
	if err := ReadEvents("events.jsonl.gz", func(e Event) error {
		counts[e.Kind+"_"+e.Codec]++
		paths[e.PathID] = true
		if e.Kind == "rtp" && e.Codec == "H265" {
			if e.PT != 98 || e.ClockRate != 90000 || e.NAL == nil {
				t.Fatal("H265 metadata changed")
			}
		}
		if e.Kind == "rtp" && e.Codec == "PCMA" {
			if e.PT != 8 || e.ClockRate != 8000 || e.PayloadSamples != 316 || e.RTPPaddingBytes != 0 {
				t.Fatal("PCMA observed payload/counter semantics changed")
			}
			if e.Role == "main" {
				audioMain[e.CaseID] = true
			}
		}
		if e.Kind == "rtp" && len(SyntheticPayload(e)) != e.PayloadBytes {
			t.Fatal("synthetic size mismatch")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for key, n := range map[string]int{"rtp_H265": 10177, "sr_H265": 64, "rtp_PCMA": 4731, "sr_PCMA": 32} {
		if counts[key] != n {
			t.Fatalf("%s %d !=%d", key, counts[key], n)
		}
	}
	if len(paths) != 16 || len(audioMain) != 4 {
		t.Fatalf("path/audio availability %d/%d", len(paths), len(audioMain))
	}
}
