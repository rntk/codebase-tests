package pythonplugin

import "github.com/review-server/internal/plugin"

// New returns a new Python plugin instance.
func New() plugin.Plugin {
	return &Plugin{}
}
