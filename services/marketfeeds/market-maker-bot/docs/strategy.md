# Market Making Strategy Developer Guide

## Overview
This guide explains how to implement, configure, and extend market making strategies in the bot.

## Strategy Types
- **Spread-Based**: Maintains a fixed or dynamic spread around the mid-price.
- **Inventory-Aware**: Adjusts quotes based on current inventory.
- **Dynamic Pricing**: Incorporates volatility and inventory for adaptive spreads.

## Implementing a New Strategy
1. Implement the `Strategy` interface in `strategy/strategy.go`:
   ```go
   type Strategy interface {
       Quote(state MarketState) (bid, ask float64)
   }
   ```
2. Add your strategy to the `NewStrategy` factory.

## Configuration
- Strategies are configured via `StrategyConfig` (spread, inventory targets, volatility, etc).
- Example:
   ```go
   cfg := StrategyConfig{BaseSpread: 0.01, InventoryTarget: 0, MaxPosition: 100, Volatility: 0.05}
   strat := NewStrategy(DynamicPricing, cfg)
   ```

## Analytics & LLM Integration
- Use `MarketState` for analytics.
- LLM predictions can be fetched via the LLM API client (`llm/client.go`).

## Testing & Backtesting
- Use the backtesting framework in `test/backtest/backtest.go` for strategy evaluation.

## Deployment
- Strategies are hot-swappable and can be reloaded without downtime.

---
