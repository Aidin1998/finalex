// grpc_client.go: gRPC client SDK for market data distribution with auto-reconnect
package client

import (
	"context"
	"io"
	"log"
	"sync"
	"time"

	pb "github.com/Aidin1998/pincex_unified/api/marketdata"
	"github.com/Aidin1998/pincex_unified/internal/marketmaking/marketdata/distribution"
	"google.golang.org/grpc"
)

// GRPCClient is a gRPC client with auto-reconnect and streaming
type GRPCClient struct {
	addr              string
	connMu            sync.RWMutex
	conn              *grpc.ClientConn
	client            pb.MarketDataClient
	subStream         pb.MarketData_SubscribeClient
	subMu             sync.Mutex
	recvCh            chan distribution.Update
	quitCh            chan struct{}
	subReq            *pb.SubscribeRequest
	reconnectInterval time.Duration
}

// NewGRPCClient creates a new GRPCClient connecting to the given address
func NewGRPCClient(addr string, reconnectInterval time.Duration) *GRPCClient {
	c := &GRPCClient{
		addr:              addr,
		recvCh:            make(chan distribution.Update, 1000),
		quitCh:            make(chan struct{}),
		reconnectInterval: reconnectInterval,
	}
	go c.run()
	return c
}

// run manages connection, subscription, and reconnection
func (c *GRPCClient) run() {
	for {
		select {
		case <-c.quitCh:
			return
		default:
			err := c.connectAndSubscribe()
			if err != nil {
				log.Printf("GRPCClient error: %v, reconnecting in %v", err, c.reconnectInterval)
				time.Sleep(c.reconnectInterval)
				continue
			}
			// receive loop
			for {
				rsp, err := c.subStream.Recv()
				if err == io.EOF {
					break
				}
				if err != nil {
					c.conn.Close()
					break
				}
				// push to channel
				upd := distribution.Update{
					Symbol:     rsp.Symbol,
					PriceLevel: int(rsp.PriceLevel),
					Bid:        rsp.Bid,
					Ask:        rsp.Ask,
					Timestamp:  time.Unix(0, rsp.Timestamp),
				}
				select {
				case c.recvCh <- upd:
				default:
				}
			}
		}
	}
}

// connectAndSubscribe dials and sends the subscribe request
func (c *GRPCClient) connectAndSubscribe() error {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	if c.conn != nil {
		c.conn.Close()
	}
	conn, err := grpc.Dial(c.addr, grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(5*time.Second))
	if err != nil {
		return err
	}
	c.conn = conn
	c.client = pb.NewMarketDataClient(conn)

	c.subMu.Lock()
	defer c.subMu.Unlock()
	if c.subReq == nil {
		return nil // no subscription yet
	}
	s, err := c.client.Subscribe(context.Background(), c.subReq)
	if err != nil {
		return err
	}
	c.subStream = s
	return nil
}

// Subscribe sends a subscription request to the server
type SubscribeOptions struct {
	ClientID    string
	Symbol      string
	Levels      []int32
	Frequency   time.Duration
	Compression bool
}

// Subscribe sets subscription parameters and initiates the stream
func (c *GRPCClient) Subscribe(opts SubscribeOptions) {
	c.subMu.Lock()
	c.subReq = &pb.SubscribeRequest{
		ClientId:    opts.ClientID,
		Symbol:      opts.Symbol,
		Levels:      opts.Levels,
		Frequency:   toProtoDuration(opts.Frequency),
		Compression: opts.Compression,
	}
	c.subMu.Unlock()
}

// Next returns the next update (blocking)
func (c *GRPCClient) Next() (distribution.Update, bool) {
	upd, ok := <-c.recvCh
	return upd, ok
}

// Close shuts down the client
type closeOnce struct{}

func (c *GRPCClient) Close() {
	close(c.quitCh)
	c.connMu.Lock()
	if c.conn != nil {
		c.conn.Close()
	}
	c.connMu.Unlock()
}

// toProtoDuration converts time.Duration to protobuf Duration
func toProtoDuration(d time.Duration) *pb.Frequency {
	return &pb.Frequency{Seconds: int64(d.Seconds()), Nanos: int32(d.Nanoseconds() % 1e9)}
}
