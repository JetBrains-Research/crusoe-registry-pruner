package crusoe

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/crusoecloud/client-go/swagger/v1"
)

// fakeCcr is an in-memory stand-in for the CCR endpoints of the Crusoe API.
// It rejects what the real API would: foreign credentials, an unknown project,
// repository, image or digest, a location that does not match the repository,
// and malformed pagination. Deletions mutate its state and are recorded.
type fakeCcr struct {
	project   string
	accessKey string

	// intercept, when set, sees every authenticated request before the fake handles it.
	// It reports whether it already wrote a response.
	intercept func(w http.ResponseWriter, r *http.Request) bool

	mu               sync.Mutex
	repositories     []*fakeRepository
	deletedManifests []string
	deletedImages    []string
	deleteRequests   int
}

type fakeRepository struct {
	swagger.Repository
	images []*fakeImage
}

type fakeImage struct {
	swagger.CcrImage
	manifests []swagger.Manifest
}

func newFakeCcr(repositories []*fakeRepository) *fakeCcr {
	return &fakeCcr{
		project:      testProjectID,
		accessKey:    testAccessKey,
		repositories: repositories,
	}
}

// serve starts the fake and points the client at it for the duration of the test.
// Configure the fake before calling it.
func (fake *fakeCcr) serve(t *testing.T) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /projects/{project}/ccr/repositories", fake.listRepositories)
	mux.HandleFunc("GET /projects/{project}/ccr/repositories/{repository}/images", fake.listImages)
	mux.HandleFunc("DELETE /projects/{project}/ccr/repositories/{repository}/images", fake.deleteImage)
	mux.HandleFunc("GET /projects/{project}/ccr/repositories/{repository}/manifests", fake.listManifests)
	mux.HandleFunc("DELETE /projects/{project}/ccr/repositories/{repository}/manifests", fake.deleteManifest)

	server := httptest.NewServer(fake.authenticate(mux))
	t.Cleanup(server.Close)

	previous := endpoint
	endpoint = server.URL
	t.Cleanup(func() { endpoint = previous })
}

// deletions returns the digests of deleted manifests and the names of deleted images, sorted.
func (fake *fakeCcr) deletions() (manifests []string, images []string) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	manifests = slices.Sorted(slices.Values(fake.deletedManifests))
	images = slices.Sorted(slices.Values(fake.deletedImages))
	return manifests, images
}

// deletes returns the number of delete requests received, including rejected ones.
func (fake *fakeCcr) deletes() int {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return fake.deleteRequests
}

// remaining returns the number of images and manifests still stored.
func (fake *fakeCcr) remaining() (images int, manifests int) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, repository := range fake.repositories {
		images += len(repository.images)
		for _, image := range repository.images {
			manifests += len(image.manifests)
		}
	}
	return images, manifests
}

func (fake *fakeCcr) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization := r.Header.Get("Authorization")
		if !strings.HasPrefix(authorization, "Bearer 1.0:"+fake.accessKey+":") ||
			r.Header.Get("X-Crusoe-Timestamp") == "" {
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		if r.Method == http.MethodDelete {
			fake.mu.Lock()
			fake.deleteRequests++
			fake.mu.Unlock()
		}
		if fake.intercept != nil && fake.intercept(w, r) {
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (fake *fakeCcr) listRepositories(w http.ResponseWriter, r *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if r.PathValue("project") != fake.project {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	items := make([]swagger.Repository, 0, len(fake.repositories))
	for _, repository := range fake.repositories {
		items = append(items, repository.Repository)
	}
	writePage(w, r, items)
}

func (fake *fakeCcr) listImages(w http.ResponseWriter, r *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	repository, ok := fake.repository(w, r)
	if !ok {
		return
	}
	items := make([]swagger.CcrImage, 0, len(repository.images))
	for _, image := range repository.images {
		items = append(items, image.CcrImage)
	}
	writePage(w, r, items)
}

func (fake *fakeCcr) deleteImage(w http.ResponseWriter, r *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	repository, ok := fake.repository(w, r)
	if !ok {
		return
	}
	name := r.URL.Query().Get("image_id")
	index := slices.IndexFunc(repository.images, func(image *fakeImage) bool { return image.Name == name })
	if index < 0 {
		writeError(w, http.StatusNotFound, "image not found")
		return
	}
	repository.images = slices.Delete(repository.images, index, index+1)
	fake.deletedImages = append(fake.deletedImages, name)
	w.WriteHeader(http.StatusNoContent)
}

func (fake *fakeCcr) listManifests(w http.ResponseWriter, r *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	image, ok := fake.image(w, r)
	if !ok {
		return
	}
	writePage(w, r, image.manifests)
}

func (fake *fakeCcr) deleteManifest(w http.ResponseWriter, r *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	image, ok := fake.image(w, r)
	if !ok {
		return
	}
	digest := r.URL.Query().Get("digest")
	index := slices.IndexFunc(image.manifests, func(manifest swagger.Manifest) bool { return manifest.Digest == digest })
	if index < 0 {
		writeError(w, http.StatusNotFound, "manifest not found")
		return
	}
	image.manifests = slices.Delete(image.manifests, index, index+1)
	fake.deletedManifests = append(fake.deletedManifests, digest)
	w.WriteHeader(http.StatusNoContent)
}

// repository resolves the project and repository from the path. A location, when sent, must match the repository's.
func (fake *fakeCcr) repository(w http.ResponseWriter, r *http.Request) (*fakeRepository, bool) {
	if r.PathValue("project") != fake.project {
		writeError(w, http.StatusNotFound, "project not found")
		return nil, false
	}
	query := r.URL.Query()
	for _, repository := range fake.repositories {
		if repository.Id != r.PathValue("repository") {
			continue
		}
		if query.Has("location") && query.Get("location") != repository.Location {
			writeError(w, http.StatusNotFound, "repository not found in location")
			return nil, false
		}
		return repository, true
	}
	writeError(w, http.StatusNotFound, "repository not found")
	return nil, false
}

func (fake *fakeCcr) image(w http.ResponseWriter, r *http.Request) (*fakeImage, bool) {
	repository, ok := fake.repository(w, r)
	if !ok {
		return nil, false
	}
	name := r.URL.Query().Get("image_id")
	for _, image := range repository.images {
		if image.Name == name {
			return image, true
		}
	}
	writeError(w, http.StatusNotFound, "image not found")
	return nil, false
}

// writePage serves a 1-indexed page of items. Pages past the end are empty.
func writePage[T any](w http.ResponseWriter, r *http.Request, items []T) {
	query := r.URL.Query()
	page, pageErr := strconv.Atoi(query.Get("page"))
	size, sizeErr := strconv.Atoi(query.Get("page_size"))
	if pageErr != nil || sizeErr != nil || page < 1 || size < 1 {
		writeError(w, http.StatusBadRequest, "invalid pagination")
		return
	}
	start := min((page-1)*size, len(items))
	end := min(start+size, len(items))
	writeJSON(w, http.StatusOK, map[string]any{"items": items[start:end]})
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"code": http.StatusText(status), "message": message})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
