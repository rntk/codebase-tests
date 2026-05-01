package goplugin

import "github.com/review-server/internal/plugin"

// New returns a new Go plugin instance.
func New() plugin.Plugin {
	return &Plugin{}
}
