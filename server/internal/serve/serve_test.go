package serve

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"appcontrol/server/internal/config"
)

func TestRunDrainsActiveRequestDuringShutdown(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	handler := http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		close(requestStarted)
		<-releaseRequest
		response.WriteHeader(http.StatusNoContent)
	})

	settings, err := config.Load(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	settings.ShutdownTimeout = 2 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	runResult := make(chan error, 1)
	go func() {
		runResult <- Run(ctx, listener, handler, nil, settings)
	}()

	responseResult := make(chan error, 1)
	go func() {
		response, requestErr := http.Get("http://" + listener.Addr().String())
		if requestErr == nil {
			defer response.Body.Close()
			_, requestErr = io.Copy(io.Discard, response.Body)
			if requestErr == nil && response.StatusCode != http.StatusNoContent {
				requestErr = &unexpectedStatusError{status: response.StatusCode}
			}
		}
		responseResult <- requestErr
	}()

	select {
	case <-requestStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not reach handler")
	}

	cancel()
	select {
	case err := <-runResult:
		t.Fatalf("server stopped before active request completed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseRequest)
	if err := <-responseResult; err != nil {
		t.Fatalf("active request failed: %v", err)
	}
	if err := <-runResult; err != nil {
		t.Fatalf("run server: %v", err)
	}
}

type unexpectedStatusError struct {
	status int
}

func (err *unexpectedStatusError) Error() string {
	return http.StatusText(err.status)
}
