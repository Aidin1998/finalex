//go:build userauth

package test

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

// TestBasicSetup ensures that the test infrastructure is working correctly
func TestBasicSetup(t *testing.T) {
	// This is a simple test to verify that the test infrastructure works
	t.Log("Basic setup test running")
	assert.True(t, true, "True should be true")
}
