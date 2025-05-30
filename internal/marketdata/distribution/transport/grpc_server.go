// grpc_server.go: gRPC transport for market data distribution
package transport

import (
	"net"

	pb "github.com/Aidin1998/pincex_unified/api/marketdata"
	"github.com/Aidin1998/pincex_unified/internal/marketdata/distribution"
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
	sub := &distribution.Subscription{ClientID: clientID, Symbol: req.Symbol, PriceLevels: req.Levels, Frequency: req.Frequency.AsDuration(), Compression: req.Compression}
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
