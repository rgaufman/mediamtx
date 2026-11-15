package httpp

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/logger"
)

type closeRegressionLogger struct{}

func (closeRegressionLogger) Log(logger.Level, string, ...any) {}

// A handler may wait for its request context while no response is being written.
// Once graceful shutdown expires, Close must cancel that context and finish.
func TestServerCloseCancelsActiveRequest(t *testing.T) {
	entered := make(chan struct{})
	s := &Server{
		Address:      "127.0.0.1:0",
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
		Parent:       closeRegressionLogger{},
		Handler: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			close(entered)
			<-r.Context().Done()
		}),
	}
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+s.ln.Addr().String()+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		res, requestErr := http.DefaultClient.Do(req)
		if requestErr == nil {
			res.Body.Close()
		}
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not start")
	}
	closed := make(chan struct{})
	go func() { s.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(shutdownTimeout + time.Second):
		t.Error("Close remains blocked after shutdown timeout; active request context was not canceled")
	}
	// Ensure the regression also cleans up when run against the broken server.
	cancel()
	<-requestDone
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close did not finish after explicit client cancellation")
	}
}
