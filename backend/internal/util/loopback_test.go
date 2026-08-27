package util

import "testing"

func TestIsLoopbackListen(t *testing.T) {
	loopback := []string{"127.0.0.1", "127.0.0.2", "::1", "[::1]", "localhost", "LOCALHOST", " 127.0.0.1 "}
	public := []string{"", "::", "[::]", "0.0.0.0", "*", "10.0.0.1", "example.com", "127.0.0.1:8443"}

	for _, s := range loopback {
		if !IsLoopbackListen(s) {
			t.Errorf("%q: want loopback", s)
		}
	}
	for _, s := range public {
		if IsLoopbackListen(s) {
			t.Errorf("%q: want NOT loopback", s)
		}
	}
}
