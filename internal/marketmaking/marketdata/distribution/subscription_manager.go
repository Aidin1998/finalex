// subscription_manager.go: Manages client subscriptions for market data distribution
package distribution

import (
	"sync"
	"time"
)

// SubscriptionManager handles subscription CRUD and retrieval
type SubscriptionManager struct {
	mu              sync.RWMutex
	subsByClient    map[string]*Subscription
	clientsBySymbol map[string]map[string]struct{} // symbol -> clientID set
}

// NewSubscriptionManager initializes a SubscriptionManager
func NewSubscriptionManager() *SubscriptionManager {
	return &SubscriptionManager{
		subsByClient:    make(map[string]*Subscription),
		clientsBySymbol: make(map[string]map[string]struct{}),
	}
}

// Subscribe registers or updates a client's subscription
func (sm *SubscriptionManager) Subscribe(sub *Subscription) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	// Remove old subscription if exists
	if old, ok := sm.subsByClient[sub.ClientID]; ok {
		sm.removeLocked(old)
	}
	// Add new subscription
	sm.subsByClient[sub.ClientID] = sub
	clients := sm.clientsBySymbol[sub.Symbol]
	if clients == nil {
		clients = make(map[string]struct{})
		sm.clientsBySymbol[sub.Symbol] = clients
	}
	clients[sub.ClientID] = struct{}{}
}

// Unsubscribe removes a client's subscription
func (sm *SubscriptionManager) Unsubscribe(clientID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sub, ok := sm.subsByClient[clientID]; ok {
		sm.removeLocked(sub)
	}
}

// removeLocked removes subscription, caller must hold write lock
func (sm *SubscriptionManager) removeLocked(sub *Subscription) {
	delete(sm.subsByClient, sub.ClientID)
	clients := sm.clientsBySymbol[sub.Symbol]
	if clients != nil {
		delete(clients, sub.ClientID)
		if len(clients) == 0 {
			delete(sm.clientsBySymbol, sub.Symbol)
		}
	}
}

// GetSubscriptions returns all subscriptions for a symbol
func (sm *SubscriptionManager) GetSubscriptions(symbol string) []*Subscription {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	subs := make([]*Subscription, 0)
	if clients, ok := sm.clientsBySymbol[symbol]; ok {
		for clientID := range clients {
			subs = append(subs, sm.subsByClient[clientID])
		}
	}
	return subs
}

// Get all current subscriptions
func (sm *SubscriptionManager) AllSubscriptions() []*Subscription {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	subs := make([]*Subscription, 0, len(sm.subsByClient))
	for _, sub := range sm.subsByClient {
		subs = append(subs, sub)
	}
	return subs
}

// UpdateFrequency allows changing a subscription's frequency
func (sm *SubscriptionManager) UpdateFrequency(clientID string, freq time.Duration) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sub, ok := sm.subsByClient[clientID]; ok {
		sub.Frequency = freq
	}
}
