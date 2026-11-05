package rtsp_test

import (
	"context"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/staticsources/rtsp"
	"github.com/bluenviron/mediamtx/internal/test"
)

// A server announcing "a=rtpmap:0 PCMU/16000" (as Dahua IPC-HDW4431EM-ASE does)
// must reach the path, and therefore the recorder, as a 16000 Hz G711 track.
func TestSourceG711StaticPayloadTypeHonoursRTPMap(t *testing.T) {
	var serverStream *gortsplib.ServerStream
	server := gortsplib.Server{
		RTSPAddress: "127.0.0.1:8558",
		Handler: &testServer{
			onDescribe: func(*gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
				return &base.Response{StatusCode: base.StatusOK}, serverStream, nil
			},
			onSetup: func(*gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
				return &base.Response{StatusCode: base.StatusOK}, serverStream, nil
			},
			onPlay: func(*gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
				return &base.Response{StatusCode: base.StatusOK}, nil
			},
		},
	}
	require.NoError(t, server.Start())
	defer server.Close()

	serverStream = &gortsplib.ServerStream{
		Server: &server,
		Desc: &description.Session{Medias: []*description.Media{
			test.UniqueMediaH264(),
			{Type: description.MediaTypeAudio, Formats: []format.Format{&format.G711{
				PayloadTyp: 0, MULaw: true, SampleRate: 16000, ChannelCount: 1,
			}}},
		}},
	}
	require.NoError(t, serverStream.Initialize())
	defer serverStream.Close()

	parent := &filteredSourceParent{ready: make(chan *description.Session, 1)}
	parent.Initialize()
	defer parent.Close()

	source := &rtsp.Source{
		ReadTimeout: conf.Duration(5 * time.Second), WriteTimeout: conf.Duration(5 * time.Second),
		WriteQueueSize: 512, Parent: parent,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	defer func() { cancel(); <-done }()
	go func() {
		done <- source.Run(defs.StaticSourceRunParams{
			Context: ctx, ResolvedSource: "rtsp://127.0.0.1:8558/camera",
			Conf: &conf.Path{
				RTSPTransport:          conf.RTSPTransport{Protocol: new(gortsplib.ProtocolTCP)},
				RTSPUDPSourcePortRange: []uint{32768, 60999},
			},
		})
	}()

	select {
	case desc := <-parent.ready:
		var g711 *format.G711
		require.NotNil(t, desc.FindFormat(&g711))
		require.Equal(t, 16000, g711.ClockRate())
		require.Equal(t, 1, g711.ChannelCount)
	case <-time.After(5 * time.Second):
		t.Fatal("source did not become ready")
	}
}
