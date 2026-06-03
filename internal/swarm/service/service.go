// Package service is the process-singleton swarm host: the 127.0.0.1:8888
// HTTP/WS server that fronts one or more isolated SwarmSpaces.
//
// At M0 (SPRD-1-1) this is a walking skeleton — it binds the address, answers
// GET /healthz, and serves the embedded vue SPA placeholder. The multi-space
// SwarmSpace registry, the session token, and the webapi wiring land in
// SPRD-1-8.
package service

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/johnny1110/evva/web"
)

// DefaultAddr is the loopback bind the service uses unless overridden. Binding
// to 127.0.0.1 (not 0.0.0.0) is the security baseline (invariant #6): agents
// run shell and edit files, so the workstation is RCE-equivalent and must not
// be reachable off-box by default.
const DefaultAddr = "127.0.0.1:8888"

// Service is the swarm host. One per process.
//
// TODO(SPRD-1-8): add the SwarmSpace registry (map[id]*swarm.SwarmSpace), a
// session token, and the webapi handlers. TODO(SPRD-1-9): daemonization +
// pidfile/log under ~/.evva/service/.
type Service struct {
	mu   sync.Mutex
	addr string        // resolved after Listen (supports :0 in tests)
	ln   net.Listener  // bound listener, nil until Listen
	srv  *http.Server
}

// New builds the host bound (logically) to addr. An empty addr uses
// DefaultAddr. Call Listen then Serve (Serve calls Listen if you skip it).
func New(addr string) *Service {
	if addr == "" {
		addr = DefaultAddr
	}
	s := &Service{addr: addr}

	mux := http.NewServeMux()
	// Method-specific patterns (Go 1.22+ mux) take precedence over the "/"
	// catch-all below, so health checks never fall through to the SPA.
	mux.HandleFunc("GET /healthz", handleHealthz)
	if sub, err := fs.Sub(web.Dist, "dist"); err == nil {
		mux.Handle("/", http.FileServerFS(sub))
	}

	s.srv = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

// Listen binds the configured address without serving. Exposed so callers
// (and tests using a :0 ephemeral port) can read Addr() before Serve blocks.
// Idempotent: a second call is a no-op once bound.
func (s *Service) Listen() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln != nil {
		return nil
	}
	ln, err := net.Listen("tcp", s.srv.Addr)
	if err != nil {
		return err
	}
	s.ln = ln
	s.addr = ln.Addr().String()
	return nil
}

// Serve serves until ctx is cancelled, then gracefully drains. It binds first
// if Listen was not already called. A context-triggered shutdown returns nil;
// any other server error is returned.
func (s *Service) Serve(ctx context.Context) error {
	if err := s.Listen(); err != nil {
		return err
	}

	errc := make(chan error, 1)
	go func() { errc <- s.srv.Serve(s.ln) }()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(shutCtx)
		return nil
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// Addr returns the address the service is bound to. Before Listen it is the
// configured address; after Listen it is the resolved one (the concrete port
// when :0 was requested).
func (s *Service) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addr
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
