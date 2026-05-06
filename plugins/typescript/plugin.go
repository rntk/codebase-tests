package typescriptplugin

import (
	"github.com/rntk/codebase-tests/internal/plugin"
	javascriptplugin "github.com/rntk/codebase-tests/plugins/javascript"
)

// New returns a TypeScript plugin backed by the JavaScript/TypeScript implementation.
func New() plugin.Plugin {
	return javascriptplugin.NewWithName("typescript")
}
