package leader

import (
	"context"
	"log/slog"

	"github.com/litebittech/cex/services/bookkeeper/balancer"
)

// TODO: Coordination between followers for rebalancing partitions

type Leader struct {
	log      *slog.Logger
	balancer balancer.Balancer
}

func New(logger *slog.Logger, balancer balancer.Balancer) *Leader {
	return &Leader{
		log:      logger,
		balancer: balancer,
	}
}

// AddNode adds a single node to the balancer
func (l *Leader) AddNode(ctx context.Context, node string) error {
	l.log.Info("adding node to balancer", "node", node)
	return l.balancer.AddNodes(ctx, []balancer.Node{node})
}

// RemoveNode removes a single node from the balancer
func (l *Leader) RemoveNode(ctx context.Context, node string) error {
	l.log.Info("removing node from balancer", "node", node)
	return l.balancer.RemoveNodes(ctx, []balancer.Node{node})
}

// SetNodes sets the complete list of nodes in the balancer
func (l *Leader) SetNodes(ctx context.Context, nodes []string) error {
	l.log.Info("setting nodes in balancer", "node_count", len(nodes))
	return l.balancer.SetNodes(ctx, nodes)
}

// GetPartitionMappings returns the current partition to node mappings
func (l *Leader) GetPartitionMappings(ctx context.Context) ([]balancer.PartitionMapping, error) {
	l.log.Debug("retrieving partition mappings")
	return l.balancer.PartitionMappings(ctx)
}
