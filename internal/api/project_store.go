package api

import (
	"net/url"

	"github.com/rntk/codebase-tests/internal/project"
)

const defaultProjectID = "default"

func registerProject(store map[string]*project.Project, p *project.Project) {
	store[p.ID] = p
	store[defaultProjectID] = p
}

func pathParam(value string) string {
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return value
	}
	return decoded
}
