package recorder

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/test"
)

func TestRecorderHeaderFailureAndExistingFile(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(map[bool]string{false: "existing-file", true: "invalid-header"}[invalid], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "segment.mp4")
			original := []byte("previous completed recording")
			require.NoError(t, os.WriteFile(path, original, 0o600))
			called := false
			sps := append([]byte(nil), test.FormatH264.SPS...)
			if invalid {
				sps[1] = 0x80
			}
			seg := &formatFMP4Segment{startNTP: time.Now(), f: &formatFMP4{
				ri: &recorderInstance{
					pathFormat2: path, location: time.UTC, streamID: uuid.New(),
					parent: test.NilLogger, onSegmentCreate: func(string) { called = true },
				},
				tracks: []*formatFMP4Track{{initTrack: &fmp4.InitTrack{
					ID: 1, TimeScale: 90000,
					Codec: &mcodecs.H264{SPS: sps, PPS: test.FormatH264.PPS},
				}}},
			}}
			err := seg.closeCurPart()
			require.Error(t, err)
			if !invalid {
				require.ErrorIs(t, err, os.ErrExist)
			}
			retained, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, original, retained)
			require.False(t, called)
		})
	}
}
