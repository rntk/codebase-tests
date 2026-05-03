package javascriptplugin

import "github.com/rntk/codebase-tests/internal/plugin"

// New returns a new JavaScript/TypeScript plugin instance.
func New() plugin.Plugin {
	return &Plugin{}
}
