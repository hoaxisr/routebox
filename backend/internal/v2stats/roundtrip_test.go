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

// TestQueryStatsRoundTrip proves the vendored generated proto marshals over a
// real grpc-go connection (catches any ProtoReflect/MessageInfo wiring breakage
// from the package rename before any other task depends on it).
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
	out, err := cli.QueryStats(context.Background(), &QueryStatsRequest{Pattern: "user>>>"})
	if err != nil {
		t.Fatalf("QueryStats: %v", err)
	}
	if len(out.Stat) != 1 || out.Stat[0].GetName() != "user>>>alice>>>traffic>>>uplink" || out.Stat[0].GetValue() != 1234 {
		t.Fatalf("got %+v", out.Stat)
	}
}
