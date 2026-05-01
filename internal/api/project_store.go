package api

import "github.com/review-server/internal/project"

const defaultProjectID = "default"

func registerProject(store map[string]*project.Project, p *project.Project) {
	store[p.ID] = p
	store[defaultProjectID] = p
}
