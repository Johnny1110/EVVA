package service

import (
	"strings"
	"testing"
)

// RP-15: every start mints a fresh random token — the fixed "root" is gone.
func TestMintedTokenPerStart(t *testing.T) {
	a, b := New("127.0.0.1:0"), New("127.0.0.1:0")
	if a.Token() == "" || a.Token() == "root" {
		t.Fatalf("token = %q, want a minted secret", a.Token())
	}
	if a.Token() == b.Token() {
		t.Fatal("two hosts minted the same token")
	}
}

// RP-15: Listen refuses wildcard / non-loopback binds unless the explicit
// --allow-remote opt-in was set, and the refusal names the flag.
func TestListenRefusesNonLoopback(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:0", ":0", "[::]:0"} {
		s := New(addr)
		if err := s.Listen(); err == nil || !strings.Contains(err.Error(), "allow-remote") {
			t.Fatalf("Listen(%q) = %v, want an allow-remote refusal", addr, err)
		}
	}

	s := New("127.0.0.1:0") // loopback binds with no opt-in, as before
	if err := s.Listen(); err != nil {
		t.Fatalf("loopback Listen: %v", err)
	}
	_ = s.ln.Close()

	s = New("0.0.0.0:0")
	s.SetAllowRemote(true)
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen with allow-remote: %v", err)
	}
	_ = s.ln.Close()
}

func TestIsLoopbackAddr(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:8888", "127.0.0.2:80", "localhost:0", "localhost", "[::1]:9"} {
		if !IsLoopbackAddr(addr) {
			t.Errorf("IsLoopbackAddr(%q) = false, want true", addr)
		}
	}
	for _, addr := range []string{"0.0.0.0:8888", ":8888", "[::]:8888", "192.168.1.7:8888", "example.com:80", ""} {
		if IsLoopbackAddr(addr) {
			t.Errorf("IsLoopbackAddr(%q) = true, want false", addr)
		}
	}
}
