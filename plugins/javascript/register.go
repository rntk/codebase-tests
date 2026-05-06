package javascriptplugin

import "github.com/rntk/codebase-tests/internal/plugin"

// New returns a new JavaScript/TypeScript plugin instance.
func New() plugin.Plugin {
	return NewWithName("javascript")
}

// NewWithName returns a JavaScript/TypeScript plugin instance registered under name.
func NewWithName(name string) plugin.Plugin {
	return &Plugin{name: name}
}
