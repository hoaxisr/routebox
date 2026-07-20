package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEndpointsCRUD_ReservedTag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"endpoints":[{"type":"awg","tag":"awg-server","private_key":"K","address":["10.10.0.1/24"],"listen_port":51820}]}`), 0600)
	m, _ := NewManager(path)

	ep := map[string]interface{}{"type": "awg", "tag": "awg-server", "private_key": "K", "address": []interface{}{"10.10.0.1/24"}, "listen_port": float64(51820)}
	if err := m.CreateEndpoint(ep); !errors.Is(err, ErrReservedTag) {
		t.Fatalf("Create reserved: want ErrReservedTag, got %v", err)
	}
	if err := m.UpdateEndpoint("awg-server", ep); !errors.Is(err, ErrReservedTag) {
		t.Fatalf("Update reserved: want ErrReservedTag, got %v", err)
	}
	if err := m.DeleteEndpoint("awg-server"); !errors.Is(err, ErrReservedTag) {
		t.Fatalf("Delete reserved: want ErrReservedTag, got %v", err)
	}
}
