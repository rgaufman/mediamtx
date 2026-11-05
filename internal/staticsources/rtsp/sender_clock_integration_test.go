package rtsp

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/logger"
)

type senderClockServer struct{ stream *gortsplib.ServerStream }

func (s *senderClockServer) OnDescribe(
	*gortsplib.ServerHandlerOnDescribeCtx,
) (*base.Response, *gortsplib.ServerStream, error) {
	return &base.Response{StatusCode: base.StatusOK}, s.stream, nil
}

func (s *senderClockServer) OnSetup(
	*gortsplib.ServerHandlerOnSetupCtx,
) (*base.Response, *gortsplib.ServerStream, error) {
	return &base.Response{StatusCode: base.StatusOK}, s.stream, nil
}

func (s *senderClockServer) OnPlay(*gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	return &base.Response{StatusCode: base.StatusOK}, nil
}

type senderClockLog struct{}

func (senderClockLog) Log(logger.Level, string, ...any) {}

// Exercise the real Client callbacks over both transports: sender reports must
// reach the repair, including UDP where RTP and RTCP run on different readers.
type senderClockObservation struct {
	timestamp                    uint32
	ntp, originalNTP             time.Time
	available, originalAvailable bool
}

func TestSenderClockRTSPCallbacks(t *testing.T) {
	for _, transport := range []gortsplib.Protocol{gortsplib.ProtocolTCP, gortsplib.ProtocolUDP} {
		for _, mode := range []struct {
			name     string
			absolute bool
		}{
			{"relative", false}, {"absolute", true},
		} {
			t.Run(transport.String()+"/"+mode.name, func(t *testing.T) {
				var virtual atomic.Int64
				wallClockNow = func() time.Time { return time.Unix(1700000000, virtual.Load()) }
				defer func() { wallClockNow = time.Now }()
				handler := &senderClockServer{}
				server := &gortsplib.Server{
					DisableRTCPSenderReports: true, RTSPAddress: "127.0.0.1:8569",
					UDPRTPAddress: "127.0.0.1:8570", UDPRTCPAddress: "127.0.0.1:8571", Handler: handler,
				}
				require.NoError(t, server.Start())
				defer server.Close()
				media := &description.Media{
					Type: description.MediaTypeAudio, Formats: []format.Format{&format.G711{
						PayloadTyp: 0, MULaw: true, SampleRate: 8000, ChannelCount: 1,
					}},
				}
				handler.stream = &gortsplib.ServerStream{
					Server: server, Desc: &description.Session{Medias: []*description.Media{media}},
				}
				require.NoError(t, handler.stream.Initialize())
				defer handler.stream.Close()
				client := &gortsplib.Client{Protocol: &transport, Scheme: "rtsp", Host: "127.0.0.1:8569"}
				require.NoError(t, client.Start())
				defer client.Close()
				url, err := base.ParseURL("rtsp://127.0.0.1:8569/test")
				require.NoError(t, err)
				desc, _, err := client.Describe(url)
				require.NoError(t, err)
				require.NoError(t, client.SetupAll(desc.BaseURL, desc.Medias))
				source := &timestampRepairSource{Client: client, log: senderClockLog{}, preserveNTP: mode.absolute}
				received := make(chan senderClockObservation, 1)
				var originalTimestamp atomic.Uint32
				source.OnPacketRTP(desc.Medias[0], desc.Medias[0].Formats[0], func(packet *rtp.Packet) {
					raw := *packet
					raw.Timestamp = originalTimestamp.Load()
					originalNTP, originalAvailable := client.PacketNTP(desc.Medias[0], &raw)
					ntp, available := source.PacketNTP(desc.Medias[0], packet)
					received <- senderClockObservation{packet.Timestamp, ntp, originalNTP, available, originalAvailable}
				})
				_, err = client.Play(nil)
				require.NoError(t, err)
				var final, lastInput uint32
				for i := range 121 {
					at := time.Duration(i) * time.Second
					virtual.Store(int64(at + at/1000))
					// Sparse packets deliberately avoid the sample-count trigger. The RTCP
					// mapping demonstrates a +1600 ppm media clock, independent of arrival.
					ts := uint32(uint64(0xffff0000) + uint64(i*8000+i*128/10))
					originalTimestamp.Store(ts)
					lastInput = ts
					packet := &rtp.Packet{
						Header:  rtp.Header{Version: 2, PayloadType: 0, SequenceNumber: uint16(i), Timestamp: ts, SSRC: 1234},
						Payload: make([]byte, 160),
					}
					require.NoError(t, handler.stream.WritePacketRTP(media, packet))
					select {
					case observed := <-received:
						final = observed.timestamp
						if mode.absolute {
							require.Equal(t, observed.originalAvailable, observed.available)
							if observed.originalAvailable {
								require.Equal(t, observed.originalNTP, observed.ntp,
									"retimed RTP must preserve the original sender NTP mapping")
							}
						}
					case <-time.After(3 * time.Second):
						t.Fatal("missing RTP")
					}
					if i%5 == 0 {
						require.NoError(t, handler.stream.WritePacketRTCP(media, &rtcp.SenderReport{
							SSRC: packet.SSRC, NTPTime: auditNTP(at), RTPTime: ts,
						}))
						// Wait for the asynchronous RTCP callback before advancing virtual time.
						track := source.walls[desc.Medias[0]][0]
						require.Eventually(t, func() bool {
							track.mu.Lock()
							defer track.mu.Unlock()
							return track.prevRTP == ts && track.reportSet
						}, time.Second, time.Millisecond)
					}
				}
				require.Negative(t, int32(final-lastInput))
				require.Eventually(t, func() bool {
					count := 0
					source.absoluteTimes.Range(func(_, _ any) bool { count++; return true })
					return count == 0
				}, time.Second, time.Millisecond, "per-packet NTP state must be released after callbacks")

				track := source.walls[desc.Medias[0]][0]
				track.mu.Lock()
				require.True(t, track.active)
				track.mu.Unlock()
			})
		}
	}
}
