package api //nolint:revive

import (
	"bufio"
	"context"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/test"
)

// blockingPathManager stands in for a path manager whose path is busy: every
// context-taking read blocks until its context is cancelled, and reports the
// cancellation on cancelled.
type blockingPathManager struct {
	started   chan struct{}
	cancelled chan error
}

func newBlockingPathManager() *blockingPathManager {
	return &blockingPathManager{
		started:   make(chan struct{}, 1),
		cancelled: make(chan error, 1),
	}
}

func (m *blockingPathManager) block(ctx context.Context) error {
	m.started <- struct{}{}
	<-ctx.Done()
	m.cancelled <- ctx.Err()
	return ctx.Err()
}

func (m *blockingPathManager) APIPathsList(ctx context.Context) (*defs.APIPathList, error) {
	return nil, m.block(ctx)
}

func (m *blockingPathManager) APIPathsGet(ctx context.Context, _ string) (*defs.APIPath, error) {
	return nil, m.block(ctx)
}

func (m *blockingPathManager) APIReadersList(ctx context.Context, _ string) (*defs.APIPathReaderList, error) {
	return nil, m.block(ctx)
}

func (m *blockingPathManager) APIReadersGet(ctx context.Context, _ string, _ string) (*defs.APIPathReader, error) {
	return nil, m.block(ctx)
}

func (m *blockingPathManager) APIStaticSourcesGet(ctx context.Context, _ string) (*defs.APIStaticSource, error) {
	return nil, m.block(ctx)
}

func (*blockingPathManager) APIForwardDestsList(string) (*defs.APIForwardDestList, error) {
	return &defs.APIForwardDestList{}, nil
}

func (*blockingPathManager) APIForwardDestsGet(string, uuid.UUID) (*defs.APIForwardDest, error) {
	return nil, conf.ErrPathNotFound
}

var requestCancelEndpoints = []string{
	"/v3/paths/list",
	"/v3/paths/get/mypath",
	"/v3/paths/readers/list?path=mypath",
	"/v3/paths/readers/get?path=mypath&id=id1",
	"/v3/paths/static-sources/get/mypath",
}

func startBlockingAPI(t *testing.T, pm *blockingPathManager) *API {
	api := &API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		PathManager:  pm,
		Parent:       &testParent{},
	}
	require.NoError(t, api.Initialize())
	return api
}

// sendBlockedRequest opens a raw connection, sends a GET and waits until the
// path manager read has started, so the request is known to be in flight.
func sendBlockedRequest(t *testing.T, pm *blockingPathManager, path string) net.Conn {
	conn, err := net.Dial("tcp", "localhost:9997")
	require.NoError(t, err)

	w := bufio.NewWriter(conn)
	_, err = w.WriteString("GET " + path + " HTTP/1.1\r\nHost: localhost\r\n\r\n")
	require.NoError(t, err)
	require.NoError(t, w.Flush())

	select {
	case <-pm.started:
	case <-time.After(5 * time.Second):
		conn.Close()
		t.Fatal("path manager read did not start")
	}
	return conn
}

func TestAPIPathReadCancelledOnClientDisconnect(t *testing.T) {
	for _, path := range requestCancelEndpoints {
		t.Run(path, func(t *testing.T) {
			pm := newBlockingPathManager()
			api := startBlockingAPI(t, pm)
			defer api.Close()

			conn := sendBlockedRequest(t, pm, path)
			conn.Close()

			select {
			case err := <-pm.cancelled:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(2 * time.Second):
				t.Fatal("path read was not cancelled within 2s of the client disconnecting")
			}
		})
	}
}

func TestAPIPathReadCancelledOnAPIClose(t *testing.T) {
	pm := newBlockingPathManager()
	api := startBlockingAPI(t, pm)

	conn := sendBlockedRequest(t, pm, "/v3/paths/list")
	defer conn.Close()

	closed := make(chan struct{})
	go func() {
		api.Close()
		close(closed)
	}()

	select {
	case err := <-pm.cancelled:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("path read was not cancelled within 2s of the API closing")
	}

	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("API.Close did not return")
	}
}
