package v2stats

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// fakeServer is a minimal StatsService that returns a canned QueryStats reply.
type fakeServer struct {
	UnimplementedStatsServiceServer
	resp *QueryStatsResponse
}

func (f *fakeServer) QueryStats(_ context.Context, _ *QueryStatsRequest) (*QueryStatsResponse, error) {
	return f.resp, nil
}

// realServiceName is the name the amnezia-box fork's experimental/v2rayapi
// stats.go init() registers at runtime. The client's *_FullMethodName constants
// MUST resolve to this path or the live server returns Unimplemented.
const realServiceName = "v2ray.core.app.stats.command.StatsService"

// TestServiceNameMatchesFork guards against the vendored generated name drifting
// back to "experimental.v2rayapi.StatsService" (which the fork overrides), which
// would make every live RPC fail with Unimplemented.
func TestServiceNameMatchesFork(t *testing.T) {
	if got := StatsService_ServiceDesc.ServiceName; got != realServiceName {
		t.Errorf("ServiceDesc.ServiceName = %q, want %q", got, realServiceName)
	}
	want := "/" + realServiceName + "/QueryStats"
	if StatsService_QueryStats_FullMethodName != want {
		t.Errorf("QueryStats_FullMethodName = %q, want %q", StatsService_QueryStats_FullMethodName, want)
	}
}

// TestQueryStatsRoundTrip proves the vendored generated proto marshals over a
// real grpc-go connection (catches any ProtoReflect/MessageInfo wiring breakage
// from the package rename before any other task depends on it). Client and
// server both use the fork's REALNAME, so a green run also matches reality.
func TestQueryStatsRoundTrip(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	RegisterStatsServiceServer(srv, &fakeServer{resp: &QueryStatsResponse{
		Stat: []*Stat{{Name: "user>>>alice>>>traffic>>>uplink", Value: 1234}},
	}})
	go srv.Serve(lis)
	defer srv.Stop()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	cli := NewStatsServiceClient(conn)
	out, err := cli.QueryStats(context.Background(), &QueryStatsRequest{Patterns: []string{"user>>>"}})
	if err != nil {
		t.Fatalf("QueryStats: %v", err)
	}
	if len(out.Stat) != 1 || out.Stat[0].GetName() != "user>>>alice>>>traffic>>>uplink" || out.Stat[0].GetValue() != 1234 {
		t.Fatalf("got %+v", out.Stat)
	}
}
