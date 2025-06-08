package server

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"sync"
	"time"

	"github.com/Aidin1998/finalex/internal/infrastructure/config"
	"go.uber.org/zap"
)

// TLSManager handles TLS certificate management including hot reload
type TLSManager struct {
	config       *config.TLSConfig
	logger       *zap.Logger
	certificate  *tls.Certificate
	certPool     *x509.CertPool
	mu           sync.RWMutex
	reloadTicker *time.Ticker
	stopCh       chan struct{}
}

// NewTLSManager creates a new TLS manager
func NewTLSManager(cfg *config.TLSConfig, logger *zap.Logger) (*TLSManager, error) {
	if !cfg.Enabled {
		return &TLSManager{
			config: cfg,
			logger: logger,
		}, nil
	}

	manager := &TLSManager{
		config: cfg,
		logger: logger,
		stopCh: make(chan struct{}),
	}

	// Load initial certificate
	if err := manager.loadCertificate(); err != nil {
		return nil, fmt.Errorf("failed to load initial certificate: %w", err)
	}

	// Start hot reload if enabled
	if cfg.HotReload && cfg.ReloadInterval > 0 {
		manager.startHotReload()
	}

	return manager, nil
}

// GetTLSConfig returns the TLS configuration for HTTP/gRPC servers
func (tm *TLSManager) GetTLSConfig() *tls.Config {
	if !tm.config.Enabled {
		return nil
	}

	tm.mu.RLock()
	defer tm.mu.RUnlock()

	tlsConfig := &tls.Config{
		GetCertificate:           tm.getCertificate,
		PreferServerCipherSuites: tm.config.PreferServerCiphers,
		CurvePreferences:         tm.getCurvePreferences(),
		CipherSuites:             tm.getCipherSuites(),
	}

	// Set TLS version constraints
	if tm.config.MinVersion != "" {
		tlsConfig.MinVersion = tm.getTLSVersion(tm.config.MinVersion)
	}
	if tm.config.MaxVersion != "" {
		tlsConfig.MaxVersion = tm.getTLSVersion(tm.config.MaxVersion)
	}

	// Set client authentication
	switch tm.config.ClientAuth {
	case "NoClientCert":
		tlsConfig.ClientAuth = tls.NoClientCert
	case "RequestClientCert":
		tlsConfig.ClientAuth = tls.RequestClientCert
	case "RequireAnyClientCert":
		tlsConfig.ClientAuth = tls.RequireAnyClientCert
	case "VerifyClientCertIfGiven":
		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
	case "RequireAndVerifyClientCert":
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		if tm.certPool != nil {
			tlsConfig.ClientCAs = tm.certPool
		}
	}

	return tlsConfig
}

// loadCertificate loads the certificate and CA from files
func (tm *TLSManager) loadCertificate() error {
	if tm.config.CertFile == "" || tm.config.KeyFile == "" {
		return fmt.Errorf("certificate and key files must be specified")
	}

	// Load certificate
	cert, err := tls.LoadX509KeyPair(tm.config.CertFile, tm.config.KeyFile)
	if err != nil {
		return fmt.Errorf("failed to load certificate: %w", err)
	}

	tm.mu.Lock()
	tm.certificate = &cert
	tm.mu.Unlock()

	// Load CA certificate if specified
	if tm.config.CAFile != "" {
		caCert, err := ioutil.ReadFile(tm.config.CAFile)
		if err != nil {
			return fmt.Errorf("failed to read CA certificate: %w", err)
		}

		tm.certPool = x509.NewCertPool()
		if !tm.certPool.AppendCertsFromPEM(caCert) {
			return fmt.Errorf("failed to parse CA certificate")
		}
	}

	tm.logger.Info("TLS certificate loaded successfully",
		zap.String("cert_file", tm.config.CertFile),
		zap.String("key_file", tm.config.KeyFile),
		zap.String("ca_file", tm.config.CAFile))

	return nil
}

// getCertificate returns the current certificate for TLS handshakes
func (tm *TLSManager) getCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.certificate, nil
}

// startHotReload starts the certificate hot reload process
func (tm *TLSManager) startHotReload() {
	tm.reloadTicker = time.NewTicker(tm.config.ReloadInterval)

	go func() {
		for {
			select {
			case <-tm.reloadTicker.C:
				if err := tm.loadCertificate(); err != nil {
					tm.logger.Error("Failed to reload certificate", zap.Error(err))
				} else {
					tm.logger.Debug("Certificate reloaded successfully")
				}
			case <-tm.stopCh:
				return
			}
		}
	}()

	tm.logger.Info("TLS hot reload started", zap.Duration("interval", tm.config.ReloadInterval))
}

// getTLSVersion converts string version to tls constant
func (tm *TLSManager) getTLSVersion(version string) uint16 {
	switch version {
	case "1.0":
		return tls.VersionTLS10
	case "1.1":
		return tls.VersionTLS11
	case "1.2":
		return tls.VersionTLS12
	case "1.3":
		return tls.VersionTLS13
	default:
		return tls.VersionTLS12 // Default to TLS 1.2
	}
}

// getCipherSuites converts string cipher suites to constants
func (tm *TLSManager) getCipherSuites() []uint16 {
	if len(tm.config.CipherSuites) == 0 {
		return nil // Use default
	}

	cipherMap := map[string]uint16{
		"TLS_AES_128_GCM_SHA256":                  tls.TLS_AES_128_GCM_SHA256,
		"TLS_AES_256_GCM_SHA384":                  tls.TLS_AES_256_GCM_SHA384,
		"TLS_CHACHA20_POLY1305_SHA256":            tls.TLS_CHACHA20_POLY1305_SHA256,
		"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256": tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384": tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305":  tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256":   tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384":   tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305":    tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
	}

	var ciphers []uint16
	for _, suite := range tm.config.CipherSuites {
		if cipher, ok := cipherMap[suite]; ok {
			ciphers = append(ciphers, cipher)
		}
	}

	return ciphers
}

// getCurvePreferences converts string curves to constants
func (tm *TLSManager) getCurvePreferences() []tls.CurveID {
	if len(tm.config.ECDHCurves) == 0 {
		return nil // Use default
	}

	curveMap := map[string]tls.CurveID{
		"X25519": tls.X25519,
		"P-256":  tls.CurveP256,
		"P-384":  tls.CurveP384,
		"P-521":  tls.CurveP521,
	}

	var curves []tls.CurveID
	for _, curve := range tm.config.ECDHCurves {
		if c, ok := curveMap[curve]; ok {
			curves = append(curves, c)
		}
	}

	return curves
}

// Stop stops the TLS manager and cleanup resources
func (tm *TLSManager) Stop() {
	if tm.reloadTicker != nil {
		tm.reloadTicker.Stop()
	}
	close(tm.stopCh)
	tm.logger.Info("TLS manager stopped")
}
