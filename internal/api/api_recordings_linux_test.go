//go:build linux

package api //nolint:revive

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestRecordingsListPaginationIO(t *testing.T) {
	testRecordingsListPagination(t, func(t *testing.T, path string) func(bool) {
		t.Helper()
		fd, err := unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC)
		require.NoError(t, err)
		t.Cleanup(func() { _ = unix.Close(fd) })
		_, err = unix.InotifyAddWatch(fd, path, unix.IN_OPEN)
		require.NoError(t, err)
		return func(opened bool) {
			t.Helper()
			buf := make([]byte, 4096)
			n, readErr := unix.Read(fd, buf)
			if opened {
				require.NoError(t, readErr)
				require.Positive(t, n)
			} else {
				require.ErrorIs(t, readErr, unix.EAGAIN)
			}
		}
	})
}
