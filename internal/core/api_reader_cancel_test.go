package core

import (
	"context"
	"errors"
	"testing"
	"time"
)

// The manager can be occupied waiting for a path's source/recorder to stop.
// API readers must escape both before enqueueing and after actor acceptance.
func TestAPIPathReadersCancel(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		for _, method := range []string{"list", "get", "staticGet"} {
			t.Run(method+map[bool]string{false: "/queued", true: "/accepted"}[accepted], func(t *testing.T) {
				pmCtx, pmCancel := context.WithCancel(context.Background())
				defer pmCancel()
				pm := &pathManager{
					ctx:                   pmCtx,
					chAPIPathsList:        make(chan pathAPIPathsListReq),
					chAPIPathsGet:         make(chan pathAPIPathsGetReq),
					chAPIStaticSourcesGet: make(chan pathAPIStaticSourcesGetReq),
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				done := make(chan error, 1)
				go func() {
					var err error
					switch method {
					case "list":
						_, err = pm.APIPathsList(ctx)
					case "get":
						_, err = pm.APIPathsGet(ctx, "test")
					case "staticGet":
						_, err = pm.APIStaticSourcesGet(ctx, "test")
					}
					done <- err
				}()
				// Delay the actor's reply until the caller has already canceled.
				// Buffered replies must allow the actor to resume without a receiver.
				var reply func()
				if accepted {
					switch method {
					case "list":
						req := <-pm.chAPIPathsList
						reply = func() { req.res <- pathAPIPathsListRes{} }
					case "get":
						req := <-pm.chAPIPathsGet
						reply = func() { req.res <- pathAPIPathsGetRes{} }
					case "staticGet":
						req := <-pm.chAPIStaticSourcesGet
						reply = func() { req.res <- pathAPIStaticSourcesGetRes{} }
					}
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

func TestAPIPathDetailReadersCancel(t *testing.T) {
	for _, method := range []string{"get", "staticGet"} {
		t.Run(method, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			pa := &path{ctx: context.Background(), chAPIPathsGet: make(chan pathAPIPathsGetReq), chAPIStaticSourcesGet: make(chan pathAPIStaticSourcesGetReq)}
			done := make(chan error, 1)
			go func() {
				var err error
				if method == "get" {
					_, err = pa.APIPathsGet(ctx, pathAPIPathsGetReq{})
				} else {
					_, err = pa.APIStaticSourcesGet(ctx, pathAPIStaticSourcesGetReq{})
				}
				done <- err
			}()
			var reply func()
			if method == "get" {
				req := <-pa.chAPIPathsGet
				reply = func() { req.res <- pathAPIPathsGetRes{} }
			} else {
				req := <-pa.chAPIStaticSourcesGet
				reply = func() { req.res <- pathAPIStaticSourcesGetRes{} }
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("expected cancellation, got %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("path reader ignored cancellation")
			}
			replied := make(chan struct{})
			go func() { reply(); close(replied) }()
			select {
			case <-replied:
			case <-time.After(time.Second):
				t.Fatal("late path reply blocks actor")
			}
		})
	}
}

func TestAPIPathsListCancelsDuringPathRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pm := &pathManager{ctx: context.Background(), chAPIPathsList: make(chan pathAPIPathsListReq)}
	pa := &path{ctx: context.Background(), chAPIPathsGet: make(chan pathAPIPathsGetReq)}
	done := make(chan error, 1)
	go func() { _, err := pm.APIPathsList(ctx); done <- err }()
	req := <-pm.chAPIPathsList
	req.res <- pathAPIPathsListRes{paths: map[string]*path{"test": pa}}
	pathReq := <-pa.chAPIPathsGet
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation instead of partial list, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("list ignored cancellation while reading path")
	}
	replied := make(chan struct{})
	go func() { pathReq.res <- pathAPIPathsGetRes{}; close(replied) }()
	select {
	case <-replied:
	case <-time.After(time.Second):
		t.Fatal("late path reply blocks actor")
	}
}
