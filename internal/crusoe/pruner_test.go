package crusoe

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/crusoecloud/client-go/swagger/v1"
)

const (
	testProjectID = "6f1d4a8e-2c3b-4d5e-9f60-718293a4b5c6"
	testAccessKey = "test-access-key"
)

var testSecretKey = base64.RawURLEncoding.EncodeToString([]byte("test-secret-key"))

func daysAgo(days int) time.Time {
	return time.Now().Add(-time.Duration(days) * 24 * time.Hour)
}

// registry is a small project spanning two locations.
// Sizes are distinct powers of two, so the freed bytes identify exactly what got deleted.
func registry() []*fakeRepository {
	return []*fakeRepository{
		{
			Repository: swagger.Repository{Id: "repo-cache", Name: "cache", Location: "us-east1-a"},
			images: []*fakeImage{
				{
					CcrImage: swagger.CcrImage{Name: "library/alpine"},
					manifests: []swagger.Manifest{
						{Digest: "sha256:alpine-pulled-recently", SizeBytes: "1", PushedAt: daysAgo(60), PulledAt: daysAgo(1)},
						{Digest: "sha256:alpine-stale", SizeBytes: "2", Tags: []string{"3.19"}, PushedAt: daysAgo(60), PulledAt: daysAgo(60)},
						{Digest: "sha256:alpine-release", SizeBytes: "4", Tags: []string{"v3.18"}, PushedAt: daysAgo(90), PulledAt: daysAgo(90)},
						{Digest: "sha256:alpine-fresh", SizeBytes: "8", Tags: []string{"latest"}, PushedAt: daysAgo(2), PulledAt: daysAgo(2)},
					},
				},
				{
					CcrImage: swagger.CcrImage{Name: "library/busybox"},
					manifests: []swagger.Manifest{
						{Digest: "sha256:busybox-never-pulled", SizeBytes: "16", PushedAt: daysAgo(45)},
						{Digest: "sha256:busybox-never-pulled-tagged", SizeBytes: "32", Tags: []string{"1.36"}, PushedAt: daysAgo(50)},
					},
				},
			},
		},
		{
			Repository: swagger.Repository{Id: "repo-mirror", Name: "mirror", Location: "eu-iceland1-a"},
			images: []*fakeImage{
				{
					CcrImage: swagger.CcrImage{Name: "team/app"},
					manifests: []swagger.Manifest{
						{Digest: "sha256:app-pulled-recently", SizeBytes: "64", Tags: []string{"main"}, PushedAt: daysAgo(100), PulledAt: daysAgo(3)},
						{Digest: "sha256:app-stale", SizeBytes: "128", PushedAt: daysAgo(100), PulledAt: daysAgo(100)},
					},
				},
				{
					CcrImage: swagger.CcrImage{Name: "team/empty"},
				},
			},
		},
	}
}

// sizeOf sums the sizes of the given digests in repositories.
func sizeOf(t *testing.T, repositories []*fakeRepository, digests []string) int64 {
	t.Helper()
	var total int64
	for _, repository := range repositories {
		for _, image := range repository.images {
			for _, manifest := range image.manifests {
				if slices.Contains(digests, manifest.Digest) {
					size, err := strconv.ParseInt(manifest.SizeBytes, 10, 64)
					if err != nil {
						t.Fatal(err)
					}
					total += size
				}
			}
		}
	}
	return total
}

func TestPruneCcrPolicies(t *testing.T) {
	analyzed := Resources{Repositories: 2, Images: 4, Manifests: 8}

	cases := []struct {
		name      string
		env       map[string]string
		manifests []string
		images    []string
	}{
		{
			name: "defaults prune everything pushed over 30 days ago",
			manifests: []string{
				"sha256:alpine-pulled-recently",
				"sha256:alpine-release",
				"sha256:alpine-stale",
				"sha256:app-pulled-recently",
				"sha256:app-stale",
				"sha256:busybox-never-pulled",
				"sha256:busybox-never-pulled-tagged",
			},
		},
		{
			name: "kept tag prefixes protect manifests and the image holding them",
			env: map[string]string{
				"CRUSOE_PRUNER_KEEP_TAG_PREFIXES": "release-, v",
				"CRUSOE_PRUNER_DELETE_IMAGES":     "true",
			},
			manifests: []string{
				"sha256:alpine-pulled-recently",
				"sha256:alpine-stale",
				"sha256:app-pulled-recently",
				"sha256:app-stale",
				"sha256:busybox-never-pulled",
				"sha256:busybox-never-pulled-tagged",
			},
			images: []string{"library/busybox", "team/app", "team/empty"},
		},
		{
			name: "untagged only",
			env:  map[string]string{"CRUSOE_PRUNER_TAG_STATE": "untagged"},
			manifests: []string{
				"sha256:alpine-pulled-recently",
				"sha256:app-stale",
				"sha256:busybox-never-pulled",
			},
		},
		{
			name: "longer max age",
			env:  map[string]string{"CRUSOE_PRUNER_MAX_AGE": "1800h"},
			manifests: []string{
				"sha256:alpine-release",
				"sha256:app-pulled-recently",
				"sha256:app-stale",
			},
		},
		{
			name: "age from activity spares recently pulled manifests",
			env:  map[string]string{"CRUSOE_PRUNER_AGE_FROM": "activity"},
			manifests: []string{
				"sha256:alpine-release",
				"sha256:alpine-stale",
				"sha256:app-stale",
				"sha256:busybox-never-pulled",
				"sha256:busybox-never-pulled-tagged",
			},
		},
		{
			name: "age from pulled keeps never pulled manifests",
			env:  map[string]string{"CRUSOE_PRUNER_AGE_FROM": "pulled"},
			manifests: []string{
				"sha256:alpine-release",
				"sha256:alpine-stale",
				"sha256:app-stale",
			},
		},
		{
			name: "age from pulled with never pulled pruning",
			env: map[string]string{
				"CRUSOE_PRUNER_AGE_FROM":           "pulled",
				"CRUSOE_PRUNER_PRUNE_NEVER_PULLED": "true",
				"CRUSOE_PRUNER_DELETE_IMAGES":      "true",
			},
			manifests: []string{
				"sha256:alpine-release",
				"sha256:alpine-stale",
				"sha256:app-stale",
				"sha256:busybox-never-pulled",
				"sha256:busybox-never-pulled-tagged",
			},
			images: []string{"library/busybox", "team/empty"},
		},
		{
			name: "zero max age is a no-op",
			env:  map[string]string{"CRUSOE_PRUNER_MAX_AGE": "0"},
		},
	}

	for _, tc := range cases {
		want := Summary{
			Analyzed: analyzed,
			Deleted:  Resources{Images: len(tc.images), Manifests: len(tc.manifests)},
			Bytes:    sizeOf(t, registry(), tc.manifests),
		}

		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeCcr(registry())
			fake.serve(t)
			result := runPruner(t, tc.env)

			if result.err != nil {
				t.Fatalf("PruneCcr() error = %v", result.err)
			}
			assertDeletions(t, fake, tc.manifests, tc.images)
			assertSummary(t, result.summary, want)
		})

		t.Run(tc.name+" in dry run", func(t *testing.T) {
			fake := newFakeCcr(registry())
			fake.serve(t)
			env := map[string]string{"CRUSOE_PRUNER_DRY_RUN": "true"}
			for key, value := range tc.env {
				env[key] = value
			}
			result := runPruner(t, env)

			if result.err != nil {
				t.Fatalf("PruneCcr() error = %v", result.err)
			}
			if fake.deletes() != 0 {
				t.Errorf("dry run sent %d delete requests, want 0", fake.deletes())
			}
			// A dry run reports what a real run would reclaim.
			assertSummary(t, result.summary, want)
		})
	}
}

func TestPruneCcrContinuesPastFailures(t *testing.T) {
	fake := newFakeCcr(registry())
	fake.intercept = func(w http.ResponseWriter, r *http.Request) bool {
		query := r.URL.Query()
		switch {
		case r.Method == http.MethodGet && query.Get("image_id") == "team/app":
			writeError(w, http.StatusForbidden, "listing denied")
			return true
		case r.Method == http.MethodDelete && query.Get("digest") == "sha256:busybox-never-pulled":
			writeError(w, http.StatusInternalServerError, "deletion failed")
			return true
		}
		return false
	}
	fake.serve(t)

	result := runPruner(t, map[string]string{"CRUSOE_PRUNER_DELETE_IMAGES": "true"})

	if result.err == nil || !strings.Contains(result.err.Error(), "cleanup had failures") {
		t.Errorf("PruneCcr() error = %v, want cleanup failure", result.err)
	}
	// busybox keeps a manifest it failed to delete, so the image must survive.
	// team/app could not be listed, so nothing in it may be touched.
	assertDeletions(t, fake,
		[]string{"sha256:alpine-pulled-recently", "sha256:alpine-release", "sha256:alpine-stale", "sha256:busybox-never-pulled-tagged"},
		[]string{"team/empty"},
	)
	assertSummary(t, result.summary, Summary{
		Analyzed: Resources{Repositories: 2, Images: 4, Manifests: 6},
		Deleted:  Resources{Images: 1, Manifests: 4},
		Failed:   Failures{Listings: 1, Deletions: 1},
		Bytes:    1 + 2 + 4 + 32,
	})
}

func TestPruneCcrRetriesRateLimitedRequests(t *testing.T) {
	fake := newFakeCcr(registry())
	var requests, throttled atomic.Int64
	fake.intercept = func(w http.ResponseWriter, r *http.Request) bool {
		// Reject every other request, so each call is throttled at most once in a row.
		if requests.Add(1)%2 == 1 {
			throttled.Add(1)
			w.Header().Set("Retry-After", "0")
			writeError(w, http.StatusTooManyRequests, "slow down")
			return true
		}
		return false
	}
	fake.serve(t)

	result := runPruner(t, map[string]string{"CRUSOE_PRUNER_TAG_STATE": "untagged"})

	if result.err != nil {
		t.Fatalf("PruneCcr() error = %v", result.err)
	}
	if throttled.Load() == 0 {
		t.Fatal("no request was throttled")
	}
	assertDeletions(t, fake,
		[]string{"sha256:alpine-pulled-recently", "sha256:app-stale", "sha256:busybox-never-pulled"},
		nil,
	)
}

func TestPruneCcrWalksEveryPage(t *testing.T) {
	// Every listing crosses a page boundary: 101 repositories,
	// the first holding 101 images, the first of which holds 201 manifests.
	var repositories []*fakeRepository
	for r := range 101 {
		repository := &fakeRepository{
			Repository: swagger.Repository{Id: fmt.Sprintf("repo-%03d", r), Name: fmt.Sprintf("repo-%03d", r), Location: "us-east1-a"},
		}
		images := 1
		if r == 0 {
			images = 101
		}
		for i := range images {
			image := &fakeImage{CcrImage: swagger.CcrImage{Name: fmt.Sprintf("%s/image-%03d", repository.Name, i)}}
			manifests := 1
			if r == 0 && i == 0 {
				manifests = 201
			}
			for m := range manifests {
				image.manifests = append(image.manifests, swagger.Manifest{
					Digest:    fmt.Sprintf("sha256:%s-%03d", image.Name, m),
					SizeBytes: "1",
					PushedAt:  daysAgo(60),
				})
			}
			repository.images = append(repository.images, image)
		}
		repositories = append(repositories, repository)
	}
	fake := newFakeCcr(repositories)
	fake.serve(t)

	result := runPruner(t, map[string]string{"CRUSOE_PRUNER_DELETE_IMAGES": "true"})

	if result.err != nil {
		t.Fatalf("PruneCcr() error = %v", result.err)
	}
	if images, manifests := fake.remaining(); images != 0 || manifests != 0 {
		t.Errorf("%d images and %d manifests remain, want none", images, manifests)
	}
	assertSummary(t, result.summary, Summary{
		Analyzed: Resources{Repositories: 101, Images: 201, Manifests: 401},
		Deleted:  Resources{Images: 201, Manifests: 401},
		Bytes:    401,
	})
}

func TestPruneCcrRejectedCredentials(t *testing.T) {
	fake := newFakeCcr(registry())
	fake.accessKey = "another-access-key"
	fake.serve(t)

	result := runPruner(t, nil)

	if result.err == nil || !strings.Contains(result.err.Error(), "401") {
		t.Errorf("PruneCcr() error = %v, want 401", result.err)
	}
	assertDeletions(t, fake, nil, nil)
	assertSummary(t, result.summary, Summary{})
}

func TestPruneCcrStopsAtTimeout(t *testing.T) {
	fake := newFakeCcr(registry())
	fake.intercept = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && r.URL.Query().Has("image_id") {
			<-r.Context().Done() // hang until the client gives up
			return true
		}
		return false
	}
	fake.serve(t)

	start := time.Now()
	result := runPruner(t, map[string]string{"CRUSOE_PRUNER_TIMEOUT": "200ms"})

	if result.err == nil || !strings.Contains(result.err.Error(), "timeout exceeded") {
		t.Errorf("PruneCcr() error = %v, want timeout", result.err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("PruneCcr() took %v to give up", elapsed)
	}
	assertDeletions(t, fake, nil, nil)
}

type pruneResult struct {
	err     error
	summary Summary
}

// runPruner runs PruneCcr against the fake with the given environment
// on top of valid credentials, and parses the summary from its JSON logs.
func runPruner(t *testing.T, env map[string]string) pruneResult {
	t.Helper()

	// Developers may have a .env exported in their shell, as the README suggests.
	for _, variable := range os.Environ() {
		if key, _, _ := strings.Cut(variable, "="); strings.HasPrefix(key, "CRUSOE_") {
			t.Setenv(key, "") // restores the original value after the test
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Setenv("CRUSOE_PROJECT_ID", testProjectID)
	t.Setenv("CRUSOE_ACCESS_KEY", testAccessKey)
	t.Setenv("CRUSOE_SECRET_KEY", testSecretKey)
	t.Setenv("CRUSOE_PRUNER_LOG_FORMAT", "json")
	t.Setenv("CRUSOE_PRUNER_LOG_LEVEL", "debug")
	for key, value := range env {
		t.Setenv(key, value)
	}

	logs, err := os.CreateTemp(t.TempDir(), "pruner-*.log")
	if err != nil {
		t.Fatal(err)
	}

	stdout, logger := os.Stdout, slog.Default()
	os.Stdout = logs
	err = func() error {
		defer func() { os.Stdout = stdout; slog.SetDefault(logger) }()
		return PruneCcr()
	}()

	if closeErr := logs.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	output, readErr := os.ReadFile(logs.Name())
	if readErr != nil {
		t.Fatal(readErr)
	}
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("pruner logs:\n%s", output)
		}
	})

	return pruneResult{err: err, summary: parseSummary(t, output)}
}

func parseSummary(t *testing.T, logs []byte) Summary {
	t.Helper()
	scanner := bufio.NewScanner(bytes.NewReader(logs))
	scanner.Buffer(nil, 1<<20)
	for scanner.Scan() {
		var record struct {
			Msg     string  `json:"msg"`
			Summary Summary `json:"summary"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("malformed log line %q: %v", scanner.Text(), err)
		}
		if record.Msg == "finished cleanup" {
			return record.Summary
		}
	}
	t.Fatal("no summary in logs")
	return Summary{}
}

func assertDeletions(t *testing.T, fake *fakeCcr, wantManifests []string, wantImages []string) {
	t.Helper()
	manifests, images := fake.deletions()
	if !slices.Equal(manifests, slices.Sorted(slices.Values(wantManifests))) {
		t.Errorf("deleted manifests = %q, want %q", manifests, wantManifests)
	}
	if !slices.Equal(images, slices.Sorted(slices.Values(wantImages))) {
		t.Errorf("deleted images = %q, want %q", images, wantImages)
	}
}

func assertSummary(t *testing.T, got Summary, want Summary) {
	t.Helper()
	if got.Duration <= 0 {
		t.Errorf("summary duration = %v, want positive", got.Duration)
	}
	got.Duration = 0
	if got != want {
		t.Errorf("summary = %+v, want %+v", got, want)
	}
}
