package goplugin

import "github.com/rntk/codebase-tests/internal/plugin"

// New returns a new Go plugin instance.
func New() plugin.Plugin {
	return &Plugin{}
}
