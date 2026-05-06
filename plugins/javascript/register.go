package javascriptplugin

import "github.com/rntk/codebase-tests/internal/plugin"

func init() {
	plugin.RegisterFactory("javascript", func() plugin.Plugin { return New() })
	plugin.RegisterFactory("typescript", func() plugin.Plugin { return NewWithName("typescript") })
}

// New returns a new JavaScript/TypeScript plugin instance.
func New() *Plugin {
	return NewWithName("javascript")
}

// NewWithName returns a JavaScript/TypeScript plugin instance registered under name.
func NewWithName(name string) *Plugin {
	return &Plugin{name: name}
}
