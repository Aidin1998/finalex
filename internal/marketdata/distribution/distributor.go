// distributor.go: Core logic for batching and dispatching market data updates
package distribution

import (
	"sync"
	"time"
)

// Distributor collects updates and dispatches them to clients based on subscriptions
type Distributor struct {
	sm            *SubscriptionManager
	updateCh      chan Update
	quitCh        chan struct{}
	buffers       map[string][]Update // symbol -> buffered updates
	buffersMu     sync.Mutex
	flushInterval time.Duration               // global flush interval
	clientChans   map[string]chan interface{} // clientID -> message channel
	clientsMu     sync.RWMutex
	lastSent      map[string]map[int]Update // clientID -> priceLevel -> last sent update
}

// NewDistributor creates a new Distributor with the given flush interval
func NewDistributor(sm *SubscriptionManager, flushInterval time.Duration) *Distributor {
	return &Distributor{
		sm:            sm,
		updateCh:      make(chan Update, 10000),
		quitCh:        make(chan struct{}),
		buffers:       make(map[string][]Update),
		flushInterval: flushInterval,
		clientChans:   make(map[string]chan interface{}),
		lastSent:      make(map[string]map[int]Update),
	}
}

// Publish sends a new update into the distributor
func (d *Distributor) Publish(update Update) {
	select {
	case d.updateCh <- update:
	default:
		// drop if full
	}
}

// Run starts the main loop for buffering and dispatching updates
func (d *Distributor) Run() {
	ticker := time.NewTicker(d.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case update := <-d.updateCh:
			d.buffersMu.Lock()
			d.buffers[update.Symbol] = append(d.buffers[update.Symbol], update)
			d.buffersMu.Unlock()

		case <-ticker.C:
			d.flushAll()

		case <-d.quitCh:
			return
		}
	}
}

// flushAll processes all buffered updates and sends them to subscribers
func (d *Distributor) flushAll() {
	d.buffersMu.Lock()
	buffersCopy := d.buffers
	d.buffers = make(map[string][]Update)
	d.buffersMu.Unlock()

	for symbol, updates := range buffersCopy {
		// get subscribers for symbol
		subs := d.sm.GetSubscriptions(symbol)
		if len(subs) == 0 {
			continue
		}
		// for each subscriber, filter and send
		for _, sub := range subs {
			filtered := filterUpdates(updates, sub)
			if len(filtered) == 0 {
				continue
			}
			// apply delta compression if enabled
			msg := d.compressIfNeeded(filtered, sub)
			d.sendToClient(sub.ClientID, symbol, msg)
		}
	}
}

// Stop signals the distributor to stop processing
func (d *Distributor) Stop() {
	close(d.quitCh)
}

// RegisterClient registers a channel for delivering messages to a client
func (d *Distributor) RegisterClient(clientID string, ch chan interface{}) {
	d.clientsMu.Lock()
	defer d.clientsMu.Unlock()
	d.clientChans[clientID] = ch
}

// UnregisterClient removes the client's delivery channel
func (d *Distributor) UnregisterClient(clientID string) {
	d.clientsMu.Lock()
	defer d.clientsMu.Unlock()
	delete(d.clientChans, clientID)
}

// filterUpdates returns only updates matching the subscription's price levels and frequency
func filterUpdates(updates []Update, sub *Subscription) []Update {
	var out []Update
	// TODO: track last sent timestamp per client to respect Frequency
	for _, u := range updates {
		for _, lvl := range sub.PriceLevels {
			if u.PriceLevel == lvl {
				out = append(out, u)
				break
			}
		}
	}
	return out
}

// compressIfNeeded compresses updates into a delta payload based on lastSent state
func (d *Distributor) compressIfNeeded(updates []Update, sub *Subscription) interface{} {
	if !sub.Compression {
		return updates
	}
	var deltas []Update
	// ensure client map
	if _, ok := d.lastSent[sub.ClientID]; !ok {
		d.lastSent[sub.ClientID] = make(map[int]Update)
	}
	lastMap := d.lastSent[sub.ClientID]
	for _, u := range updates {
		prev, exists := lastMap[u.PriceLevel]
		if !exists || prev.Bid != u.Bid || prev.Ask != u.Ask {
			deltas = append(deltas, u)
			lastMap[u.PriceLevel] = u
		}
	}
	return deltas
}

// sendToClient delivers the message to the client transport layer
func (d *Distributor) sendToClient(clientID, symbol string, msg interface{}) {
	d.clientsMu.RLock()
	ch, ok := d.clientChans[clientID]
	d.clientsMu.RUnlock()
	if ok {
		select {
		case ch <- msg:
		default:
			// drop if client channel is full
		}
	}
}
