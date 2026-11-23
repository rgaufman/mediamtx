package rtsp

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/sdpunmarshaler"
)

// applyG711RTPMap sets the sample rate and channel count of G711 formats that use a
// static payload type (0 = PCMU, 8 = PCMA) from the rtpmap attribute the server declared.
//
// gortsplib (format.G711.unmarshal) hardcodes 8000 Hz mono for payload types 0 and 8
// and ignores the rtpmap. Some cameras (Dahua IPC-HDW4431EM-ASE) declare
// "a=rtpmap:0 PCMU/16000" and send 16000 samples per second with RTP timestamps at
// 16000 Hz. Read at 8000 Hz, their audio timeline runs at twice wall-clock speed,
// the recorder resets on drift every few seconds and segments grow without bound.
//
// Without an rtpmap the RFC 3551 defaults (8000 Hz mono) are kept.
func applyG711RTPMap(desc *description.Session, sdpBody []byte) error {
	sd, err := sdpunmarshaler.Unmarshal(sdpBody)
	if err != nil {
		return fmt.Errorf("unable to read G711 rtpmap: %w", err)
	}

	if len(sd.MediaDescriptions) != len(desc.Medias) {
		return fmt.Errorf("unable to read G711 rtpmap: SDP has %d medias, description has %d",
			len(sd.MediaDescriptions), len(desc.Medias))
	}

	for i, media := range desc.Medias {
		for _, forma := range media.Formats {
			g711, ok := forma.(*format.G711)
			if !ok || (g711.PayloadTyp != 0 && g711.PayloadTyp != 8) {
				continue
			}

			var rtpMap string
			for _, attr := range sd.MediaDescriptions[i].Attributes {
				if attr.Key != "rtpmap" {
					continue
				}
				parts := strings.SplitN(strings.TrimSpace(attr.Value), " ", 2)
				if len(parts) == 2 && parts[0] == strconv.FormatUint(uint64(g711.PayloadTyp), 10) {
					rtpMap = parts[1]
				}
			}

			if rtpMap == "" {
				continue
			}

			sampleRate, channelCount, err := parseG711RTPMap(rtpMap)
			if err != nil {
				return fmt.Errorf("invalid rtpmap for payload type %d '%s': %w", g711.PayloadTyp, rtpMap, err)
			}

			g711.SampleRate = sampleRate
			g711.ChannelCount = channelCount
		}
	}

	return nil
}

// parseG711RTPMap parses "PCMU/16000" or "PCMU/16000/2".
func parseG711RTPMap(rtpMap string) (int, int, error) {
	parts := strings.Split(rtpMap, "/")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, 0, fmt.Errorf("expected encoding/clock[/channels]")
	}

	sampleRate, err := strconv.ParseUint(parts[1], 10, 31)
	if err != nil || sampleRate == 0 {
		return 0, 0, fmt.Errorf("invalid sample rate: '%s'", parts[1])
	}

	channelCount := uint64(1)
	if len(parts) == 3 {
		channelCount, err = strconv.ParseUint(parts[2], 10, 31)
		if err != nil || channelCount == 0 {
			return 0, 0, fmt.Errorf("invalid channel count: '%s'", parts[2])
		}
	}

	return int(sampleRate), int(channelCount), nil
}
