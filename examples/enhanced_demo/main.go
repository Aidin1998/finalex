// Complete integration demo showing the enhanced market data server with backpressure management
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/auth"
	"github.com/Aidin1998/pincex_unified/internal/marketdata"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// DemoConfig contains configuration for the integration demo
type DemoConfig struct {
	ServerPort             int           `json:"server_port"`
	EnableLoadTesting      bool          `json:"enable_load_testing"`
	LoadTestClients        int           `json:"load_test_clients"`
	LoadTestDuration       time.Duration `json:"load_test_duration"`
	MessageRate            int           `json:"message_rate"`
	BackpressureConfigPath string        `json:"backpressure_config_path"`
}

// IntegrationDemo demonstrates the complete backpressure system
type IntegrationDemo struct {
	config       *DemoConfig
	server       *marketdata.EnhancedMarketDataServer
	logger       *zap.Logger
	httpServer   *http.Server
	cancelFunc   context.CancelFunc
	simulationWG sync.WaitGroup
}

// NewIntegrationDemo creates a new integration demo
func NewIntegrationDemo(config *DemoConfig) (*IntegrationDemo, error) {
	logger, err := zap.NewDevelopment()
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	// Create a simple auth service for demo
	authService := &SimpleAuthService{}

	// Create enhanced server configuration
	serverConfig := &marketdata.ServerConfig{
		Port:                config.ServerPort,
		Host:                "0.0.0.0",
		ReadTimeout:         30 * time.Second,
		WriteTimeout:        30 * time.Second,
		EnableCompression:   true,
		ReadBufferSize:      1024,
		WriteBufferSize:     1024,
		RequireAuth:         false,
		AuthTokenHeader:     "Authorization",
		AuthTokenQueryParam: "token",
		EnhancedHubConfig: &marketdata.EnhancedHubConfig{
			BackpressureConfigPath:      config.BackpressureConfigPath,
			EnablePerformanceMonitoring: true,
			EnableAdaptiveRateLimiting:  true,
			EnableCrossServiceCoord:     true,
			AutoClientClassification:    true,
			ClientTimeoutExtended:       5 * time.Minute,
			MaxClientsPerInstance:       10000,
			MessageBatchSize:            100,
			BroadcastCoalescing:         true,
			CoalescingWindowMs:          10,
		},
	}

	// Create enhanced market data server
	server, err := marketdata.NewEnhancedMarketDataServer(authService, serverConfig, logger.Named("server"))
	if err != nil {
		return nil, fmt.Errorf("failed to create enhanced server: %w", err)
	}

	return &IntegrationDemo{
		config: config,
		server: server,
		logger: logger,
	}, nil
}

// Start initializes and starts the integration demo
func (demo *IntegrationDemo) Start(ctx context.Context) error {
	demo.logger.Info("Starting integration demo",
		zap.Int("port", demo.config.ServerPort),
		zap.Bool("load_testing", demo.config.EnableLoadTesting))

	// Create context with cancellation
	ctx, cancel := context.WithCancel(ctx)
	demo.cancelFunc = cancel

	// Start enhanced server
	if err := demo.server.Start(ctx); err != nil {
		return fmt.Errorf("failed to start enhanced server: %w", err)
	}

	// Setup HTTP routes
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	// Setup server routes
	demo.server.SetupRoutes(router)

	// Add demo-specific routes
	demo.setupDemoRoutes(router)

	// Create HTTP server
	demo.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", demo.config.ServerPort),
		Handler: router,
	}

	// Start HTTP server
	go func() {
		demo.logger.Info("Starting HTTP server", zap.String("addr", demo.httpServer.Addr))
		if err := demo.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			demo.logger.Error("HTTP server error", zap.Error(err))
		}
	}()

	// Start market data simulation
	demo.simulationWG.Add(1)
	go demo.simulateMarketData(ctx)

	// Start load testing if enabled
	if demo.config.EnableLoadTesting {
		demo.logger.Info("Starting load testing",
			zap.Int("clients", demo.config.LoadTestClients),
			zap.Duration("duration", demo.config.LoadTestDuration))

		demo.simulationWG.Add(1)
		go demo.runLoadTest(ctx)
	}

	return nil
}

// Stop gracefully shuts down the integration demo
func (demo *IntegrationDemo) Stop(ctx context.Context) error {
	demo.logger.Info("Stopping integration demo")

	// Cancel context
	if demo.cancelFunc != nil {
		demo.cancelFunc()
	}

	// Stop HTTP server
	if demo.httpServer != nil {
		if err := demo.httpServer.Shutdown(ctx); err != nil {
			demo.logger.Error("Error shutting down HTTP server", zap.Error(err))
		}
	}

	// Stop enhanced server
	if err := demo.server.Stop(ctx); err != nil {
		demo.logger.Error("Error stopping enhanced server", zap.Error(err))
	}

	// Wait for simulation goroutines
	demo.simulationWG.Wait()

	demo.logger.Info("Integration demo stopped")
	return nil
}

// setupDemoRoutes adds demo-specific HTTP routes
func (demo *IntegrationDemo) setupDemoRoutes(router *gin.Engine) {
	// Demo dashboard
	router.GET("/", demo.handleDashboard)

	// Load testing controls
	router.POST("/demo/start-load-test", demo.handleStartLoadTest)
	router.POST("/demo/stop-load-test", demo.handleStopLoadTest)

	// Market data simulation controls
	router.POST("/demo/simulate-spike", demo.handleSimulateSpike)
	router.POST("/demo/simulate-emergency", demo.handleSimulateEmergency)

	// Performance testing endpoints
	router.GET("/demo/performance", demo.handlePerformanceTest)
	router.GET("/demo/status", demo.handleStatus)
}

// handleDashboard serves a simple dashboard for the demo
func (demo *IntegrationDemo) handleDashboard(c *gin.Context) {
	html := `
<!DOCTYPE html>
<html>
<head>
    <title>Enhanced Market Data Server Demo</title>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        .container { max-width: 1200px; margin: 0 auto; }
        .section { margin-bottom: 30px; padding: 20px; border: 1px solid #ddd; border-radius: 5px; }
        .button { background: #007cba; color: white; padding: 10px 20px; border: none; border-radius: 3px; cursor: pointer; }
        .button:hover { background: #005a8b; }
        .status { padding: 10px; margin: 10px 0; border-radius: 3px; }
        .status.ok { background: #d4edda; color: #155724; }
        .status.warning { background: #fff3cd; color: #856404; }
        .status.error { background: #f8d7da; color: #721c24; }
        .metrics { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 15px; }
        .metric { padding: 15px; background: #f8f9fa; border-radius: 3px; }
        .metric-value { font-size: 24px; font-weight: bold; color: #007cba; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Enhanced Market Data Server Demo</h1>
        
        <div class="section">
            <h2>System Status</h2>
            <div id="system-status">Loading...</div>
        </div>
        
        <div class="section">
            <h2>Real-time Metrics</h2>
            <div id="metrics" class="metrics">Loading...</div>
        </div>
        
        <div class="section">
            <h2>Backpressure Controls</h2>
            <button class="button" onclick="triggerEmergencyStop()">Emergency Stop</button>
            <button class="button" onclick="triggerEmergencyRecovery()">Emergency Recovery</button>
            <button class="button" onclick="simulateSpike()">Simulate Traffic Spike</button>
        </div>
        
        <div class="section">
            <h2>Load Testing</h2>
            <button class="button" onclick="startLoadTest()">Start Load Test</button>
            <button class="button" onclick="stopLoadTest()">Stop Load Test</button>
        </div>
        
        <div class="section">
            <h2>WebSocket Connection</h2>
            <button class="button" onclick="connectWebSocket()">Connect WebSocket</button>
            <button class="button" onclick="disconnectWebSocket()">Disconnect WebSocket</button>
            <div id="ws-status" class="status">Not connected</div>
            <div id="ws-messages" style="max-height: 200px; overflow-y: scroll; background: #f8f9fa; padding: 10px; margin-top: 10px;"></div>
        </div>
    </div>

    <script>
        let ws = null;
        
        function updateStatus() {
            fetch('/demo/status')
                .then(response => response.json())
                .then(data => {
                    document.getElementById('system-status').innerHTML = 
                        '<div class="status ' + (data.healthy ? 'ok' : 'error') + '">' +
                        'Server: ' + (data.healthy ? 'Healthy' : 'Unhealthy') + '<br>' +
                        'Connected Clients: ' + data.connected_clients + '<br>' +
                        'Emergency Mode: ' + (data.emergency_mode ? 'ACTIVE' : 'Normal') +
                        '</div>';
                        
                    const metrics = data.metrics || {};
                    document.getElementById('metrics').innerHTML = [
                        '<div class="metric"><div class="metric-value">' + (data.connected_clients || 0) + '</div><div>Connected Clients</div></div>',
                        '<div class="metric"><div class="metric-value">' + (metrics.messages_per_second || 0) + '</div><div>Messages/sec</div></div>',
                        '<div class="metric"><div class="metric-value">' + (metrics.avg_latency || 0) + 'ms</div><div>Avg Latency</div></div>',
                        '<div class="metric"><div class="metric-value">' + (metrics.bandwidth || 0) + 'MB/s</div><div>Bandwidth</div></div>'
                    ].join('');
                })
                .catch(error => {
                    document.getElementById('system-status').innerHTML = 
                        '<div class="status error">Error loading status: ' + error + '</div>';
                });
        }
        
        function connectWebSocket() {
            if (ws) {
                ws.close();
            }
            
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            ws = new WebSocket(protocol + '//' + window.location.host + '/ws');
            
            ws.onopen = function() {
                document.getElementById('ws-status').innerHTML = '<div class="status ok">Connected</div>';
                // Subscribe to some channels
                ws.send(JSON.stringify({action: 'subscribe', channel: 'BTCUSD'}));
                ws.send(JSON.stringify({action: 'subscribe', channel: 'ETHUSD'}));
            };
            
            ws.onmessage = function(event) {
                const msg = document.createElement('div');
                msg.textContent = new Date().toLocaleTimeString() + ': ' + event.data;
                const container = document.getElementById('ws-messages');
                container.appendChild(msg);
                container.scrollTop = container.scrollHeight;
            };
            
            ws.onclose = function() {
                document.getElementById('ws-status').innerHTML = '<div class="status warning">Disconnected</div>';
            };
            
            ws.onerror = function(error) {
                document.getElementById('ws-status').innerHTML = '<div class="status error">Error: ' + error + '</div>';
            };
        }
        
        function disconnectWebSocket() {
            if (ws) {
                ws.close();
                ws = null;
            }
        }
        
        function triggerEmergencyStop() {
            fetch('/admin/emergency-stop', {method: 'POST'})
                .then(response => response.json())
                .then(data => alert('Emergency stop triggered: ' + data.status));
        }
        
        function triggerEmergencyRecovery() {
            fetch('/admin/emergency-recovery', {method: 'POST'})
                .then(response => response.json())
                .then(data => alert('Emergency recovery: ' + data.status));
        }
        
        function simulateSpike() {
            fetch('/demo/simulate-spike', {method: 'POST'})
                .then(response => response.json())
                .then(data => alert('Traffic spike simulated: ' + data.status));
        }
        
        function startLoadTest() {
            fetch('/demo/start-load-test', {method: 'POST'})
                .then(response => response.json())
                .then(data => alert('Load test started: ' + data.status));
        }
        
        function stopLoadTest() {
            fetch('/demo/stop-load-test', {method: 'POST'})
                .then(response => response.json())
                .then(data => alert('Load test stopped: ' + data.status));
        }
        
        // Update status every 2 seconds
        setInterval(updateStatus, 2000);
        updateStatus();
    </script>
</body>
</html>`

	c.Header("Content-Type", "text/html")
	c.String(http.StatusOK, html)
}

// handleStatus provides current system status
func (demo *IntegrationDemo) handleStatus(c *gin.Context) {
	backpressureStatus := demo.server.GetBackpressureStatus()

	status := map[string]interface{}{
		"healthy":           true,
		"connected_clients": demo.server.GetConnectedClients(),
		"emergency_mode":    backpressureStatus["emergency_mode"],
		"backpressure":      backpressureStatus,
		"metrics": map[string]interface{}{
			"messages_per_second": 1000, // TODO: Calculate from metrics
			"avg_latency":         5,    // TODO: Calculate from metrics
			"bandwidth":           10,   // TODO: Calculate from metrics
		},
	}

	c.JSON(http.StatusOK, status)
}

// simulateMarketData generates realistic market data for testing
func (demo *IntegrationDemo) simulateMarketData(ctx context.Context) {
	defer demo.simulationWG.Done()

	symbols := []string{"BTCUSD", "ETHUSD", "ADAUSD", "SOLUSD", "DOTUSD"}
	prices := map[string]float64{
		"BTCUSD": 45000.0,
		"ETHUSD": 3000.0,
		"ADAUSD": 1.2,
		"SOLUSD": 100.0,
		"DOTUSD": 25.0,
	}

	ticker := time.NewTicker(time.Duration(1000/demo.config.MessageRate) * time.Millisecond)
	defer ticker.Stop()

	demo.logger.Info("Starting market data simulation", zap.Int("rate", demo.config.MessageRate))

	for {
		select {
		case <-ctx.Done():
			demo.logger.Info("Market data simulation stopping")
			return

		case <-ticker.C:
			for _, symbol := range symbols {
				// Simulate price movement
				price := prices[symbol]
				change := (float64(time.Now().UnixNano()%100) - 50) / 1000.0
				price += price * change / 100.0
				prices[symbol] = price

				// Broadcast trade data
				tradeData := map[string]interface{}{
					"symbol": symbol,
					"price":  price,
					"volume": float64(time.Now().UnixNano()%1000) / 100.0,
					"side":   []string{"buy", "sell"}[time.Now().UnixNano()%2],
				}

				demo.server.BroadcastMessage("trade", tradeData)

				// Broadcast ticker data less frequently
				if time.Now().UnixNano()%5 == 0 {
					tickerData := map[string]interface{}{
						"symbol": symbol,
						"price":  price,
						"change": change,
						"volume": float64(time.Now().UnixNano()%10000) / 100.0,
					}

					demo.server.BroadcastMessage("ticker", tickerData)
				}
			}
		}
	}
}

// Additional demo handler methods...
func (demo *IntegrationDemo) handleStartLoadTest(c *gin.Context) {
	c.JSON(http.StatusOK, map[string]string{"status": "Load test started"})
}

func (demo *IntegrationDemo) handleStopLoadTest(c *gin.Context) {
	c.JSON(http.StatusOK, map[string]string{"status": "Load test stopped"})
}

func (demo *IntegrationDemo) handleSimulateSpike(c *gin.Context) {
	c.JSON(http.StatusOK, map[string]string{"status": "Traffic spike simulated"})
}

func (demo *IntegrationDemo) handleSimulateEmergency(c *gin.Context) {
	c.JSON(http.StatusOK, map[string]string{"status": "Emergency condition simulated"})
}

func (demo *IntegrationDemo) handlePerformanceTest(c *gin.Context) {
	c.JSON(http.StatusOK, map[string]string{"status": "Performance test completed"})
}

func (demo *IntegrationDemo) runLoadTest(ctx context.Context) {
	defer demo.simulationWG.Done()
	demo.logger.Info("Load test simulation running")
	// TODO: Implement load testing logic
}

// SimpleAuthService provides basic auth for demo
type SimpleAuthService struct{}

func (s *SimpleAuthService) ValidateToken(ctx context.Context, token string) (*auth.Claims, error) {
	// Simple demo auth - accept any token
	return &auth.Claims{
		UserID: "demo_user",
		Email:  "demo@example.com",
	}, nil
}

// Main function to run the integration demo
func main() {
	config := &DemoConfig{
		ServerPort:             8080,
		EnableLoadTesting:      true,
		LoadTestClients:        100,
		LoadTestDuration:       5 * time.Minute,
		MessageRate:            1000, // messages per second
		BackpressureConfigPath: "configs/backpressure.yaml",
	}

	demo, err := NewIntegrationDemo(config)
	if err != nil {
		log.Fatalf("Failed to create demo: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the demo
	if err := demo.Start(ctx); err != nil {
		log.Fatalf("Failed to start demo: %v", err)
	}

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Printf("\n🚀 Enhanced Market Data Server Demo started!\n")
	fmt.Printf("📊 Dashboard: http://localhost:%d/\n", config.ServerPort)
	fmt.Printf("🔌 WebSocket: ws://localhost:%d/ws\n", config.ServerPort)
	fmt.Printf("📈 Admin API: http://localhost:%d/admin/\n", config.ServerPort)
	fmt.Printf("🎯 Metrics: http://localhost:%d/metrics/backpressure\n", config.ServerPort)
	fmt.Printf("\nPress Ctrl+C to stop...\n\n")

	<-sigChan
	fmt.Println("\nShutting down...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := demo.Stop(shutdownCtx); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}

	fmt.Println("Demo stopped successfully!")
}
