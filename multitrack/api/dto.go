package api

// createProjectRequest is the JSON body for POST /projects. Multitrack
// creates *empty* projects (no source) — sources are imported separately
// via POST /projects/:id/sources (M6+).
type createProjectRequest struct {
	Name string `json:"name"`
}
