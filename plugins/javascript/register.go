package javascriptplugin

// New returns a new JavaScript/TypeScript plugin instance.
func New() *Plugin {
	return NewWithName("javascript")
}

// NewWithName returns a JavaScript/TypeScript plugin instance registered under name.
func NewWithName(name string) *Plugin {
	return &Plugin{name: name}
}
