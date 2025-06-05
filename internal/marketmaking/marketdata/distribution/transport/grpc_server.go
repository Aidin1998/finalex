// grpc_server.go: gRPC transport for market data distribution
package transport

import (
	"net"
	"time"

	pb "github.com/Aidin1998/finalex/api/marketdata"
	"github.com/Aidin1998/finalex/internal/marketmaking/marketdata/distribution"
	"google.golang.org/grpc"
)

// GRPCServer serves market data updates over gRPC
type GRPCServer struct {
	dist *distribution.Distributor
	sm   *distribution.SubscriptionManager
	pb.UnimplementedMarketDataServer
}

// NewGRPCServer creates a new GRPCServer
func NewGRPCServer(dist *distribution.Distributor, sm *distribution.SubscriptionManager) *GRPCServer {
	return &GRPCServer{dist: dist, sm: sm}
}

// Subscribe handles client subscription via gRPC stream
func (s *GRPCServer) Subscribe(req *pb.SubscribeRequest, stream pb.MarketData_SubscribeServer) error {
	clientID := req.ClientId

	// Convert []int32 to []int
	levels := make([]int, len(req.Levels))
	for i, level := range req.Levels {
		levels[i] = int(level)
	}

	// Convert protobuf Frequency to time.Duration
	var frequency time.Duration
	if req.Frequency != nil {
		frequency = time.Duration(req.Frequency.Seconds)*time.Second + time.Duration(req.Frequency.Nanos)*time.Nanosecond
	}

	sub := &distribution.Subscription{ClientID: clientID, Symbol: req.Symbol, PriceLevels: levels, Frequency: frequency, Compression: req.Compression}
	s.sm.Subscribe(sub)
	ch := make(chan interface{}, 100)
	s.dist.RegisterClient(clientID, ch)
	defer s.dist.UnregisterClient(clientID)

	for msg := range ch {
		switch m := msg.(type) {
		case distribution.Update:
			stream.Send(&pb.Update{Symbol: m.Symbol, PriceLevel: int32(m.PriceLevel), Bid: m.Bid, Ask: m.Ask, Timestamp: m.Timestamp.UnixNano()})
		}
	}
	return nil
}

// Start starts the gRPC server on the given address
func (s *GRPCServer) Start(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := grpc.NewServer()
	pb.RegisterMarketDataServer(srv, s)
	return srv.Serve(lis)
}
