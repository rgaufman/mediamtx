package core

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Path-reader list/detail reads must escape a busy manager both before
// enqueueing and after actor acceptance, like the path and static-source reads.
func TestAPIReadersCancel(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		for _, method := range []string{"list", "get"} {
			t.Run(method+map[bool]string{false: "/queued", true: "/accepted"}[accepted], func(t *testing.T) {
				pmCtx, pmCancel := context.WithCancel(context.Background())
				defer pmCancel()
				pm := &pathManager{
					ctx:           pmCtx,
					chAPIPathsGet: make(chan pathAPIPathsGetReq),
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				done := make(chan error, 1)
				go func() {
					var err error
					switch method {
					case "list":
						_, err = pm.APIReadersList(ctx, "test")
					case "get":
						_, err = pm.APIReadersGet(ctx, "test", "id")
					}
					done <- err
				}()
				// Delay the actor's reply until the caller has already canceled.
				// Buffered replies must allow the actor to resume without a receiver.
				var reply func()
				if accepted {
					req := <-pm.chAPIPathsGet
					reply = func() { req.res <- pathAPIPathsGetRes{} }
				}
				cancel()
				select {
				case err := <-done:
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("expected cancellation, got %v", err)
					}
				case <-time.After(time.Second):
					t.Fatal("reader ignored cancellation")
				}
				if reply != nil {
					replied := make(chan struct{})
					go func() { reply(); close(replied) }()
					select {
					case <-replied:
					case <-time.After(time.Second):
						t.Fatal("late reply blocks actor after cancellation")
					}
				}
			})
		}
	}
}

func TestAPIPathReadersListCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pa := &path{ctx: context.Background(), chAPIReadersList: make(chan pathAPIReadersListReq)}
	done := make(chan error, 1)
	go func() { _, err := pa.APIReadersList(ctx); done <- err }()
	req := <-pa.chAPIReadersList
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("path reader list ignored cancellation")
	}
	replied := make(chan struct{})
	go func() { req.res <- pathAPIReadersListRes{}; close(replied) }()
	select {
	case <-replied:
	case <-time.After(time.Second):
		t.Fatal("late path reply blocks actor")
	}
}

func TestAPIReadersCancelsDuringPathRead(t *testing.T) {
	for _, method := range []string{"list", "get"} {
		t.Run(method, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			pm := &pathManager{ctx: context.Background(), chAPIPathsGet: make(chan pathAPIPathsGetReq)}
			pa := &path{ctx: context.Background(), chAPIReadersList: make(chan pathAPIReadersListReq)}
			done := make(chan error, 1)
			go func() {
				var err error
				if method == "list" {
					_, err = pm.APIReadersList(ctx, "test")
				} else {
					_, err = pm.APIReadersGet(ctx, "test", "id")
				}
				done <- err
			}()
			req := <-pm.chAPIPathsGet
			req.res <- pathAPIPathsGetRes{path: pa}
			pathReq := <-pa.chAPIReadersList
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("expected cancellation instead of a reader result, got %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("readers ignored cancellation while reading path")
			}
			replied := make(chan struct{})
			go func() { pathReq.res <- pathAPIReadersListRes{}; close(replied) }()
			select {
			case <-replied:
			case <-time.After(time.Second):
				t.Fatal("late path reply blocks actor")
			}
		})
	}
}
