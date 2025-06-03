//go:build test
// +build test

package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Test AutoScaler basic functionality
func TestAutoScalerBasic(t *testing.T) {
	config := &AutoScalerConfig{
		Namespace: "default",
		ScalingConstraints: &ScalingConstraints{
			MaxTotalReplicas:     10,
			MinTotalReplicas:     1,
			MaxScaleUpPercent:    0.5,
			MaxScaleDownPercent:  0.3,
			CooldownPeriod:       time.Minute * 2,
			StabilizationWindow:  time.Minute * 1,
			MaxConcurrentScaling: 3,
		},
	}

	// Test configuration validation
	assert.NotNil(t, config)
	assert.Equal(t, "default", config.Namespace)
	assert.NotNil(t, config.ScalingConstraints)
	
	// Test constraint validation
	valid := validateScalingConstraints(config.ScalingConstraints)
	assert.True(t, valid, "Scaling constraints should be valid")
}

func TestScalingConstraintsValidation(t *testing.T) {
	tests := []struct {
		name        string
		constraints *ScalingConstraints
		expectValid bool
	}{
		{
			name: "Valid constraints",
			constraints: &ScalingConstraints{
				MaxTotalReplicas:     10,
				MinTotalReplicas:     1,
				MaxScaleUpPercent:    0.5,
				MaxScaleDownPercent:  0.3,
				CooldownPeriod:       time.Minute * 2,
				StabilizationWindow:  time.Minute * 1,
				MaxConcurrentScaling: 3,
			},
			expectValid: true,
		},
		{
			name: "Invalid min/max replicas",
			constraints: &ScalingConstraints{
				MaxTotalReplicas:     1,
				MinTotalReplicas:     10, // Min > Max
				MaxScaleUpPercent:    0.5,
				MaxScaleDownPercent:  0.3,
				CooldownPeriod:       time.Minute * 2,
				StabilizationWindow:  time.Minute * 1,
				MaxConcurrentScaling: 3,
			},
			expectValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := validateScalingConstraints(tt.constraints)
			assert.Equal(t, tt.expectValid, valid)
		})
	}
}

// Helper function to validate scaling constraints
func validateScalingConstraints(constraints *ScalingConstraints) bool {
	if constraints == nil {
		return false
	}
	if constraints.MinTotalReplicas > constraints.MaxTotalReplicas {
		return false
	}
	if constraints.MaxScaleUpPercent <= 0 || constraints.MaxScaleUpPercent > 1 {
		return false
	}
	if constraints.MaxScaleDownPercent <= 0 || constraints.MaxScaleDownPercent > 1 {
		return false
	}
	if constraints.CooldownPeriod <= 0 {
		return false
	}
	return true
}
