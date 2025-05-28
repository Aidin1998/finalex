package connector

// Registry for Connector factories
var registry = make(map[string]ConnectorFactory)

// Register adds a ConnectorFactory under the given name
func Register(name string, f ConnectorFactory) {
	registry[name] = f
}

// Factory returns the ConnectorFactory registered under name, or nil if not found
func Factory(name string) ConnectorFactory {
	return registry[name]
}
