package rtsp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/defs"
)

func TestJitterUsAPISerialization(t *testing.T) {
	zero, measured := float64(0), float64(15000)
	for _, ca := range []struct {
		name  string
		value *float64
		want  string
	}{
		{"unavailable", nil, ""},
		{"measured-zero", &zero, "0"},
		{"measured", &measured, "15000"},
	} {
		t.Run(ca.name, func(t *testing.T) {
			payload, err := json.Marshal(defs.APIStaticSourceTypeSpecificRTSP{InboundRTPPacketsJitterUs: ca.value})
			require.NoError(t, err)
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(payload, &fields))
			value, present := fields["inboundRTPPacketsJitterUs"]
			require.Equal(t, ca.value != nil, present)
			require.Equal(t, ca.want, string(value))
		})
	}
}
