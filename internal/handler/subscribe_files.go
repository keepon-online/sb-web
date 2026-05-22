package handler

import (
	"net/http"
	"strings"

	"miaomiaowu/internal/storage"
)

type subscribeFilesHandler struct {
	repo *storage.TrafficRepository
}

// NewSubscribeFilesHandler returns an admin-only handler for managing subscribe files.
func NewSubscribeFilesHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("subscribe files handler requires repository")
	}

	return &subscribeFilesHandler{
		repo: repo,
	}
}

func (h *subscribeFilesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/subscribe-files")
	path = strings.Trim(path, "/")

	switch {
	case path == "" && r.Method == http.MethodGet:
		h.handleList(w, r)
	case path == "" && r.Method == http.MethodPost:
		h.handleCreate(w, r)
	case path == "reorder" && r.Method == http.MethodPut:
		h.handleReorder(w, r)
	case path == "import" && r.Method == http.MethodPost:
		h.handleImport(w, r)
	case path == "upload" && r.Method == http.MethodPost:
		h.handleUpload(w, r)
	case path == "create-from-config" && r.Method == http.MethodPost:
		h.handleCreateFromConfig(w, r)
	case strings.HasSuffix(path, "/content") && r.Method == http.MethodGet:
		// GET /api/admin/subscribe-files/{filename}/content
		filename := strings.TrimSuffix(path, "/content")
		h.handleGetContent(w, r, filename)
	case strings.HasSuffix(path, "/content") && r.Method == http.MethodPut:
		// PUT /api/admin/subscribe-files/{filename}/content
		filename := strings.TrimSuffix(path, "/content")
		h.handleUpdateContent(w, r, filename)
	case path != "" && path != "import" && path != "upload" && path != "create-from-config" && (r.Method == http.MethodPut || r.Method == http.MethodPatch):
		h.handleUpdate(w, r, path)
	case path != "" && path != "import" && path != "upload" && path != "create-from-config" && r.Method == http.MethodDelete:
		h.handleDelete(w, r, path)
	default:
		allowed := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
		methodNotAllowed(w, allowed...)
	}
}
