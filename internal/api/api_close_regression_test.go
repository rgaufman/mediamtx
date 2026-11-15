package api //nolint:revive

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/test"
)

// Model a path manager occupied with synchronous source/recorder teardown.
// Closing the API must release its reader without awaiting that dependency.
type closeBlockedPathManager struct {
	testPathManager
	entered chan struct{}
	release chan struct{}
}

func (m *closeBlockedPathManager) APIPathsList(reqCtx context.Context) (*defs.APIPathList, error) {
	close(m.entered)
	select {
	case <-m.release:
	case <-reqCtx.Done():
		return nil, reqCtx.Err()
	}
	return &defs.APIPathList{}, nil
}

func TestAPICloseWithBlockedPathManager(t *testing.T) {
	pm := &closeBlockedPathManager{entered: make(chan struct{}), release: make(chan struct{})}
	a := &API{
		Address:      "127.0.0.1:19997",
		ReadTimeout:  conf.Duration(time.Second),
		WriteTimeout: conf.Duration(time.Second),
		AuthManager:  test.NilAuthManager,
		PathManager:  pm,
		Parent:       &testParent{},
	}
	if err := a.Initialize(); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		res, err := client.Get("http://127.0.0.1:19997/v3/paths/list")
		if err == nil {
			res.Body.Close()
		}
	}()
	select {
	case <-pm.entered:
	case <-time.After(3 * time.Second):
		close(pm.release)
		a.Close()
		t.Fatal("API request did not enter path manager")
	}
	closed := make(chan struct{})
	go func() { a.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Error("API.Close waits for a blocked path manager after its context is canceled")
	}
	close(pm.release)
	<-requestDone
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("API.Close did not finish after dependency was released")
	}
}
