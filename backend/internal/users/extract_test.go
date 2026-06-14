package users

import (
	"reflect"
	"testing"
)

func TestServerInboundUsers(t *testing.T) {
	tests := []struct {
		name    string
		inbound map[string]interface{}
		want    []ConfigUser
	}{
		{
			name: "vless yields uuid credential",
			inbound: map[string]interface{}{
				"type": "vless", "tag": "vless-in",
				"users": []interface{}{
					map[string]interface{}{"name": "alice", "uuid": "u-1", "flow": "xtls-rprx-vision"},
				},
			},
			want: []ConfigUser{{InboundTag: "vless-in", Protocol: "vless", Credential: "u-1", Name: "alice", Flow: "xtls-rprx-vision"}},
		},
		{
			name: "naive yields username credential",
			inbound: map[string]interface{}{
				"type": "naive", "tag": "naive-in",
				"users": []interface{}{
					map[string]interface{}{"username": "bob", "password": "pw"},
				},
			},
			want: []ConfigUser{{InboundTag: "naive-in", Protocol: "naive", Credential: "bob", Name: "bob"}},
		},
		{
			name: "hysteria2 yields password credential",
			inbound: map[string]interface{}{
				"type": "hysteria2", "tag": "hy2-in",
				"users": []interface{}{
					map[string]interface{}{"name": "carol", "password": "secret"},
				},
			},
			want: []ConfigUser{{InboundTag: "hy2-in", Protocol: "hysteria2", Credential: "secret", Name: "carol"}},
		},
		{
			name:    "tun yields nothing",
			inbound: map[string]interface{}{"type": "tun", "tag": "tun-in"},
			want:    nil,
		},
		{
			name:    "mixed yields nothing even with users",
			inbound: map[string]interface{}{"type": "mixed", "tag": "m", "users": []interface{}{map[string]interface{}{"username": "x", "password": "y"}}},
			want:    nil,
		},
		{
			name: "blank credential skipped",
			inbound: map[string]interface{}{
				"type": "vless", "tag": "vless-in",
				"users": []interface{}{map[string]interface{}{"name": "x", "uuid": ""}},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ServerInboundUsers(tt.inbound)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}
