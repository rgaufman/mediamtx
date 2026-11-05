package rtsp

import (
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/sdpunmarshaler"
	"github.com/stretchr/testify/require"
)

// SDP captured verbatim from a Dahua IPC-HDW4431EM-ASE (firmware
// V2.622.0000000.23.R, build 2018-03-30) on TetherBox be3edeee3f2564f96448,
// camera 5ba810803d1f9235ef940d2b, 2026-10-13. A direct capture of that camera
// carried 12.2 s of PCMU payload at 16000 samples per second and 12.169 s of
// audio RTP time at 16000 Hz against 12.129 s of video in a 12.33 s window.
const dahuaPCMU16000SDP = "v=0\r\n" +
	"o=- 2255446415 2255446415 IN IP4 0.0.0.0\r\n" +
	"s=Media Server\r\n" +
	"c=IN IP4 0.0.0.0\r\n" +
	"t=0 0\r\n" +
	"a=control:*\r\n" +
	"a=packetization-supported:DH\r\n" +
	"a=rtppayload-supported:DH\r\n" +
	"a=range:npt=now-\r\n" +
	"m=video 0 RTP/AVP 96\r\n" +
	"a=control:trackID=0\r\n" +
	"a=framerate:25.000000\r\n" +
	"a=rtpmap:96 H264/90000\r\n" +
	"a=fmtp:96 packetization-mode=1;profile-level-id=640028;" +
	"sprop-parameter-sets=Z2QAKKwbGoB4AiflwFuAgICgAAB9AAAYah0MAF3AAAL68F3lxoYALuAAAX14LvLhQAA=,aO44MAA=\r\n" +
	"a=recvonly\r\n" +
	"m=audio 0 RTP/AVP 0\r\n" +
	"a=control:trackID=1\r\n" +
	"a=rtpmap:0 PCMU/16000\r\n" +
	"a=recvonly\r\n"

// decodeLikeClient decodes an SDP body exactly as gortsplib.Client.Describe does.
func decodeLikeClient(t *testing.T, body string) *description.Session {
	sd, err := sdpunmarshaler.Unmarshal([]byte(body))
	require.NoError(t, err)
	var desc description.Session
	require.NoError(t, desc.Unmarshal2(sd))
	return &desc
}

func g711Of(t *testing.T, desc *description.Session) *format.G711 {
	var g711 *format.G711
	require.NotNil(t, desc.FindFormat(&g711))
	return g711
}

func TestGortsplibIgnoresStaticG711RTPMap(t *testing.T) {
	// Documents the library behaviour this package corrects: format.G711.unmarshal
	// returns 8000 Hz for payload type 0 whatever the rtpmap says.
	require.Equal(t, 8000, g711Of(t, decodeLikeClient(t, dahuaPCMU16000SDP)).ClockRate())
}

func TestApplyG711RTPMap(t *testing.T) {
	for _, ca := range []struct {
		name     string
		sdp      string
		rate     int
		channels int
		mulaw    bool
	}{
		{"dahua pcmu 16000", dahuaPCMU16000SDP, 16000, 1, true},
		{
			"pcmu without rtpmap keeps rfc3551 default",
			"v=0\r\no=- 0 0 IN IP4 0.0.0.0\r\ns=-\r\nt=0 0\r\nm=audio 0 RTP/AVP 0\r\na=control:trackID=1\r\n",
			8000, 1, true,
		},
		{
			"pcma 8000 stereo",
			"v=0\r\no=- 0 0 IN IP4 0.0.0.0\r\ns=-\r\nt=0 0\r\nm=audio 0 RTP/AVP 8\r\na=rtpmap:8 PCMA/8000/2\r\n",
			8000, 2, false,
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			desc := decodeLikeClient(t, ca.sdp)
			require.NoError(t, applyG711RTPMap(desc, []byte(ca.sdp)))
			g711 := g711Of(t, desc)
			require.Equal(t, ca.rate, g711.ClockRate())
			require.Equal(t, ca.rate, g711.SampleRate)
			require.Equal(t, ca.channels, g711.ChannelCount)
			require.Equal(t, ca.mulaw, g711.MULaw)
		})
	}
}

func TestApplyG711RTPMapRejectsInvalidRate(t *testing.T) {
	sdp := "v=0\r\no=- 0 0 IN IP4 0.0.0.0\r\ns=-\r\nt=0 0\r\nm=audio 0 RTP/AVP 0\r\na=rtpmap:0 PCMU/abc\r\n"
	err := applyG711RTPMap(decodeLikeClient(t, sdp), []byte(sdp))
	require.EqualError(t, err, "invalid rtpmap for payload type 0 'PCMU/abc': invalid sample rate: 'abc'")
}
