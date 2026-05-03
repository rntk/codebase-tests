package pythonplugin

import "github.com/rntk/codebase-tests/internal/plugin"

// New returns a new Python plugin instance.
func New() plugin.Plugin {
	return &Plugin{}
}
