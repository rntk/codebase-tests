package typescriptplugin

import (
	javascriptplugin "github.com/rntk/codebase-tests/plugins/javascript"
)

// New returns a TypeScript plugin backed by the JavaScript/TypeScript implementation.
func New() *javascriptplugin.Plugin {
	return javascriptplugin.NewWithName("typescript")
}
