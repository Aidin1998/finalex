// Generic health checker implementation
package marketmaker

import (
	"context"
)

// CheckFunction represents a function that performs a health check
type CheckFunction func(ctx context.Context) HealthCheckResult

// GenericHealthChecker provides a simple way to create a health checker
type GenericHealthChecker struct {
	ComponentName string
	SubsystemName string
	CheckFunction CheckFunction
}

// Check performs the health check
func (g *GenericHealthChecker) Check(ctx context.Context) HealthCheckResult {
	return g.CheckFunction(ctx)
}

// Name returns the checker's name
func (g *GenericHealthChecker) Name() string {
	return g.ComponentName + "." + g.SubsystemName
}

// Component returns the component name
func (g *GenericHealthChecker) Component() string {
	return g.ComponentName
}

// Subsystem returns the subsystem name
func (g *GenericHealthChecker) Subsystem() string {
	return g.SubsystemName
}

// GenericSelfHealer provides a simple way to create a self-healer
type GenericSelfHealer struct {
	CanHealFunction func(result HealthCheckResult) bool
	HealFunction    func(ctx context.Context, result HealthCheckResult) error
	Description     string
}

// CanHeal determines if this healer can heal the given result
func (g *GenericSelfHealer) CanHeal(result HealthCheckResult) bool {
	return g.CanHealFunction(result)
}

// Heal performs self-healing actions
func (g *GenericSelfHealer) Heal(ctx context.Context, result HealthCheckResult) error {
	return g.HealFunction(ctx, result)
}

// HealingDescription returns a description of what this healer does
func (g *GenericSelfHealer) HealingDescription() string {
	return g.Description
}
