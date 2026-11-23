package webrtc

import (
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/rtpsender"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

func TestOutboundJitterClockUnits(t *testing.T) {
	newTrack := func(rate, jitter uint32, received bool) *OutboundTrack {
		tr := &OutboundTrack{Caps: webrtc.RTPCodecCapability{ClockRate: rate}, rtcpSender: &rtpsender.Sender{}}
		tr.reportReceived.Store(received)
		tr.reportedJitter.Store(jitter)
		return tr
	}

	video := newTrack(90000, 900, true)
	audio := newTrack(48000, 960, true)
	idle := newTrack(0, 100000, false)
	stats := outboundTracksStats([]*OutboundTrack{video, audio, idle})
	require.Equal(t, float64(930), stats.reportedJitter)
	require.NotNil(t, stats.reportedJitterUs)
	require.InDelta(t, 15000, *stats.reportedJitterUs, 0.001)

	require.Nil(t, outboundTracksStats(nil).reportedJitterUs)
	require.Nil(t, outboundTracksStats([]*OutboundTrack{idle}).reportedJitterUs)
	require.Nil(t, outboundTracksStats([]*OutboundTrack{video, newTrack(0, 0, true)}).reportedJitterUs)
	zero := outboundTracksStats([]*OutboundTrack{newTrack(8000, 0, true)})
	require.NotNil(t, zero.reportedJitterUs)
	require.Zero(t, *zero.reportedJitterUs)
}
