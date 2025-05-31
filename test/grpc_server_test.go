package transport

import (
	"net"
	"testing"
	"time"

	pb "github.com/Aidin1998/pincex_unified/api/marketdata"
	"github.com/Aidin1998/pincex_unified/internal/marketdata/distribution"
	clientSDK "github.com/Aidin1998/pincex_unified/pkg/marketdata/client"
	"google.golang.org/grpc"
)

func TestGRPCServer_SubscribeAndReceive(t *testing.T) {
	// Setup distribution
	sm := distribution.NewSubscriptionManager()
	d := distribution.NewDistributor(sm, 20*time.Millisecond)
	go d.Run()

	// Start gRPC server on random port
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	server := NewGRPCServer(d, sm)
	pb.RegisterMarketDataServer(grpcServer, server)
	go grpcServer.Serve(lis)
	defer grpcServer.Stop()

	// Setup client
	client := clientSDK.NewGRPCClient(lis.Addr().String(), time.Second)
	client.Subscribe(clientSDK.SubscribeOptions{
		ClientID:    "test-client",
		Symbol:      "SYM",
		Levels:      []int32{0},
		Frequency:   0,
		Compression: false,
	})
	// Allow subscription to register
	time.Sleep(50 * time.Millisecond)

	// Publish update
	upd := distribution.Update{Symbol: "SYM", PriceLevel: 0, Bid: 1.23, Ask: 4.56, Timestamp: time.Now()}
	d.Publish(upd)

	// Receive
	rec, ok := client.Next()
	if !ok {
		t.Fatalf("did not receive update")
	}
	if rec.Bid != upd.Bid || rec.Ask != upd.Ask {
		t.Fatalf("mismatch: got %v, want %v", rec, upd)
	}
	client.Close()
}
