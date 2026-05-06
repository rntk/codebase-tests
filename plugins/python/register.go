package pythonplugin

import "github.com/rntk/codebase-tests/internal/plugin"

func init() {
	plugin.RegisterFactory("python", func() plugin.Plugin { return New() })
}

// New returns a new Python plugin instance.
func New() *Plugin {
	return &Plugin{}
}
