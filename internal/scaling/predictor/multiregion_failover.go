package predictor

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

// DefaultRegionCoordinator implements RegionCoordinator with automated health checks and DNS-based failover
type DefaultRegionCoordinator struct {
	config         RegionConfig
	logger         *zap.SugaredLogger
	client         *http.Client
	healthStatuses map[string]bool
	mu             sync.RWMutex
}

// NewDefaultRegionCoordinator creates a DefaultRegionCoordinator and starts health checks
func NewDefaultRegionCoordinator(cfg RegionConfig, logger *zap.SugaredLogger) *DefaultRegionCoordinator {
	coord := &DefaultRegionCoordinator{
		config:         cfg,
		logger:         logger,
		client:         &http.Client{Timeout: 5 * time.Second},
		healthStatuses: make(map[string]bool, len(cfg.PeerRegions)+1),
	}
	go coord.startHealthChecks()
	return coord
}

// startHealthChecks periodically checks the health endpoint for each region
func (d *DefaultRegionCoordinator) startHealthChecks() {
	ticker := time.NewTicker(d.config.SyncInterval)
	for range ticker.C {
		regions := append([]string{d.config.CurrentRegion}, d.config.PeerRegions...)
		for _, region := range regions {
			url := fmt.Sprintf("https://%s.health.example.com/status", region)
			resp, err := d.client.Get(url)
			healthy := err == nil && resp.StatusCode == http.StatusOK
			d.mu.Lock()
			d.healthStatuses[region] = healthy
			d.mu.Unlock()
			if !healthy {
				d.logger.Warnw("Region unhealthy, triggering failover", "region", region)
				d.triggerFailover(region)
			}
		}
		// After health checks, rebalance traffic across healthy regions
		d.rebalanceTraffic()
	}
}

// rebalanceTraffic adjusts traffic distribution across currently healthy regions
func (d *DefaultRegionCoordinator) rebalanceTraffic() {
	d.mu.RLock()
	defer d.mu.RUnlock()

	healthyRegions := []string{}
	for region, healthy := range d.healthStatuses {
		if healthy {
			healthyRegions = append(healthyRegions, region)
		}
	}
	d.logger.Infow("Rebalancing traffic", "healthy_regions", healthyRegions)
	// TODO: integrate with DNS or load balancer API to set weights across healthyRegions
}

// triggerFailover routes traffic away from the unhealthy region (DNS or LB update)
func (d *DefaultRegionCoordinator) triggerFailover(region string) {
	d.logger.Infow("Rerouting traffic away from region", "region", region)
	// TODO: integrate with DNS or load balancer API to update records
}

// SyncScalingDecision shares scaling decisions with peer regions
func (d *DefaultRegionCoordinator) SyncScalingDecision(ctx context.Context, decision *ScalingDecision) error {
	// No-op for multi-region failover management
	return nil
}

// GetPeerRegionStatus returns the last known health and load status for peer regions
func (d *DefaultRegionCoordinator) GetPeerRegionStatus(ctx context.Context) (map[string]*RegionStatus, error) {
	status := make(map[string]*RegionStatus, len(d.config.PeerRegions))
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, region := range d.config.PeerRegions {
		healthy := d.healthStatuses[region]
		status[region] = &RegionStatus{
			Region:    region,
			IsHealthy: healthy,
		}
	}
	return status, nil
}

// ResolveConflict chooses the scaling decision based on region priority or other rules
func (d *DefaultRegionCoordinator) ResolveConflict(ctx context.Context, decisions []*ScalingDecision) (*ScalingDecision, error) {
	// Simple priority-based resolution: first decision wins
	if len(decisions) == 0 {
		return nil, fmt.Errorf("no decisions to resolve")
	}
	return decisions[0], nil
}

// NotifyPeerRegions sends region events to peer regions (no-op placeholder)
func (d *DefaultRegionCoordinator) NotifyPeerRegions(ctx context.Context, event *RegionEvent) error {
	// Implement peer notification if needed
	return nil
}
