package api

import (
	"fmt"
	"net/http"
)

// RegisterStubRoutes wires endpoints that are not yet implemented.
func RegisterStubRoutes(r *http.ServeMux) {
	r.HandleFunc("GET /projects/{id}/symbols", stub("symbols", "Plan B/C"))
	r.HandleFunc("GET /projects/{id}/tests", stub("tests", "Plan C"))
	r.HandleFunc("GET /projects/{id}/tests/{testId}", stub("test detail", "Plan C"))
	r.HandleFunc("GET /projects/{id}/tests/{testId}/golden", stub("golden list", "Plan D"))
	r.HandleFunc("GET /projects/{id}/tests/{testId}/golden/{caseId}", stub("golden case", "Plan D"))
	r.HandleFunc("GET /projects/{id}/tests/{testId}/golden/{caseId}/diff", stub("golden diff", "Plan D"))
	r.HandleFunc("GET /projects/{id}/coverage", stub("coverage", "Plan C"))
}

func stub(name, plan string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = w.Write([]byte(fmt.Sprintf("%s not yet implemented (owned by %s)", name, plan)))
	}
}
