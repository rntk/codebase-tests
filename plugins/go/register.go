package goplugin

import "github.com/rntk/codebase-tests/internal/plugin"

func init() {
	plugin.RegisterFactory("go", func() plugin.Plugin { return New() })
}

// New returns a new Go plugin instance.
func New() *Plugin {
	return &Plugin{}
}
