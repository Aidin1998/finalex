package aggregator

import (
	"context"
	"io/ioutil"
	"log"
	"strings"
	"sync"
	"time"

	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/hashicorp/consul/api"
	"github.com/segmentio/kafka-go"
)

// ServiceConsumer manages a Kafka consumer for a connector
type ServiceConsumer struct {
	exchange string
	reader   *kafka.Reader
	cancel   context.CancelFunc
}

// ConsumerManager watches Consul for connector services and manages consumers
type ConsumerManager struct {
	consul       *api.Client
	topicPre     string
	kafkaBrokers []string
	consumers    map[string]*ServiceConsumer
	mu           sync.Mutex
	ctx          context.Context
}

func NewConsumerManager(consulAddr string, kafkaBrokers []string, topicPrefix string) (*ConsumerManager, error) {
	cfg := api.DefaultConfig()
	cfg.Address = consulAddr
	client, err := api.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	return &ConsumerManager{
		consul:       client,
		topicPre:     topicPrefix,
		kafkaBrokers: kafkaBrokers,
		consumers:    make(map[string]*ServiceConsumer),
		ctx:          ctx,
	}, nil
}

// Start begins polling Consul and managing consumers
func (m *ConsumerManager) Start() {
	// start config watcher first
	m.initConfigWatcher("/etc/marketfeeds/config")

	// existing consul polling
	go func() {
		for {
			services, _, err := m.consul.Catalog().Services(nil)
			if err != nil {
				log.Println("Consul services error:", err)
				time.Sleep(10 * time.Second)
				continue
			}
			m.updateConsumers(services)
			time.Sleep(30 * time.Second)
		}
	}()
}

func (m *ConsumerManager) updateConsumers(services map[string][]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// register new
	for svc := range services {
		if !strings.HasPrefix(svc, "connector-") {
			continue
		}
		ex := strings.TrimPrefix(svc, "connector-")
		if _, exists := m.consumers[ex]; !exists {
			m.spawnConsumer(ex)
		}
	}
	// deregister removed
	for ex, sc := range m.consumers {
		svcName := "connector-" + ex
		if _, ok := services[svcName]; !ok {
			// Service no longer exists, stop and remove consumer
			sc.cancel()
			sc.reader.Close()
			delete(m.consumers, ex)
			log.Println("Stopped consumer for", ex)
		}
	}
}

func (m *ConsumerManager) spawnConsumer(exchange string) {
	topic := m.topicPre + exchange
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: m.kafkaBrokers,
		Topic:   topic,
		GroupID: "aggregator",
	})
	ctx, cancel := context.WithCancel(m.ctx)
	sc := &ServiceConsumer{exchange: exchange, reader: reader, cancel: cancel}
	m.consumers[exchange] = sc
	go sc.run(ctx)
	log.Println("Started consumer for", exchange, "topic", topic)
}

func (sc *ServiceConsumer) run(ctx context.Context) {
	for {
		msg, err := sc.reader.FetchMessage(ctx)
		if err != nil {
			return
		}
		// TODO: process tick message, integrate as needed
		sc.reader.CommitMessages(ctx, msg)
	}
}

// Add field for enabled config
func (m *ConsumerManager) initConfigWatcher(configDir string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Println("Config watcher error:", err)
		return
	}
	// watch the directory containing the ConfigMap files
	if err := watcher.Add(configDir); err != nil {
		log.Println("Watcher add error:", err)
		return
	}
	// initial load
	m.reloadEnabledExchanges(filepath.Join(configDir, "enabled_exchanges"))
	// handle events
	go func() {
		for evt := range watcher.Events {
			if filepath.Base(evt.Name) == "enabled_exchanges" && evt.Op&fsnotify.Write > 0 {
				m.reloadEnabledExchanges(evt.Name)
			}
		}
	}()
}

// reloadEnabledExchanges reads the enabled_exchanges file and updates consumers
func (m *ConsumerManager) reloadEnabledExchanges(path string) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		log.Println("Read enabled_exchanges error:", err)
		return
	}
	parts := strings.Split(strings.TrimSpace(string(data)), ",")
	newSet := make(map[string]bool)
	for _, p := range parts {
		ex := strings.TrimSpace(p)
		if ex != "" {
			newSet[ex] = true
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// spawn new
	for ex := range newSet {
		if _, exists := m.consumers[ex]; !exists {
			m.spawnConsumer(ex)
		}
	}
	// remove old
	for ex, sc := range m.consumers {
		if !newSet[ex] {
			sc.cancel()
			sc.reader.Close()
			delete(m.consumers, ex)
			log.Println("Removed consumer for disabled exchange", ex)
		}
	}
}
