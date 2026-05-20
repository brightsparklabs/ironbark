/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

package zarf

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
)

// ---------------------------------------------------------------------------
// registryHost
// ---------------------------------------------------------------------------

// TestRegistryHost_NormalisesAllSupportedShapes confirms `registryHost`
// turns every endpoint shape Zarf returns into a bare `host:port` form.
func TestRegistryHost_NormalisesAllSupportedShapes(t *testing.T) {
	tests := map[string]struct {
		input string
		want  string
	}{
		"bare host:port":           {"127.0.0.1:5000", "127.0.0.1:5000"},
		"http scheme":              {"http://127.0.0.1:5000", "127.0.0.1:5000"},
		"https scheme":             {"https://registry.example.com:5000", "registry.example.com:5000"},
		"host only":                {"zarf-docker-registry", "zarf-docker-registry"},
		"http scheme, host only":   {"http://zarf-docker-registry", "zarf-docker-registry"},
		"surrounded by whitespace": {"  127.0.0.1:5000  ", "127.0.0.1:5000"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := registryHost(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("registryHost(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestRegistryHost_RejectsInvalidInput confirms `registryHost` returns
// an error for empty / malformed endpoints rather than silently
// producing a misleading host.
func TestRegistryHost_RejectsInvalidInput(t *testing.T) {
	tests := map[string]string{
		"empty":       "",
		"whitespace":  "   ",
		"scheme only": "http://",
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := registryHost(input); err == nil {
				t.Fatalf("registryHost(%q) = nil, want error", input)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// extractZstdTarball
// ---------------------------------------------------------------------------

// TestExtractZstdTarball_RoundTripsFilesAndDirs builds an in-memory
// zstd-compressed tarball with a representative payload and confirms
// every file/directory comes out the other side intact.
func TestExtractZstdTarball_RoundTripsFilesAndDirs(t *testing.T) {
	entries := map[string]string{
		"zarf.yaml":                  "kind: ZarfPackageConfig\n",
		"components/c1.tar":          "stub-tarball-bytes",
		"components/c2.tar":          "another-stub-tarball-bytes",
		"images/blobs/sha256/abc123": "binary-image-blob",
	}

	archivePath := writeZstdTestPackage(t, entries)

	destDir := t.TempDir()
	if err := extractZstdTarball(archivePath, destDir); err != nil {
		t.Fatalf("extractZstdTarball: %v", err)
	}

	for relPath, want := range entries {
		gotBytes, err := os.ReadFile(filepath.Join(destDir, relPath))
		if err != nil {
			t.Fatalf("expected file %q: %v", relPath, err)
		}
		if string(gotBytes) != want {
			t.Fatalf("file %q content mismatch:\n got: %q\nwant: %q", relPath, gotBytes, want)
		}
	}
}

// TestExtractZstdTarball_RejectsDirectoryTraversal confirms the
// extractor refuses to write outside the destination directory, even if
// the archive contains a malicious `../` entry.
func TestExtractZstdTarball_RejectsDirectoryTraversal(t *testing.T) {
	archivePath := writeZstdTestPackage(t, map[string]string{
		"../escaped.txt": "should never land outside destDir",
	})

	destDir := t.TempDir()
	err := extractZstdTarball(archivePath, destDir)
	if err == nil {
		t.Fatalf("extractZstdTarball accepted directory-traversal entry; want error")
	}
	if !strings.Contains(err.Error(), "refusing to extract") {
		t.Fatalf("error does not mention traversal protection: %v", err)
	}
}

// TestExtractZstdTarball_ReportsMissingArchive confirms we get a clear
// error when the archive itself is missing, rather than a confusing
// downstream failure.
func TestExtractZstdTarball_ReportsMissingArchive(t *testing.T) {
	err := extractZstdTarball(filepath.Join(t.TempDir(), "does-not-exist.tar.zst"), t.TempDir())
	if err == nil {
		t.Fatalf("extractZstdTarball returned nil for missing archive")
	}
}

// ---------------------------------------------------------------------------
// subPrefixFromURL
// ---------------------------------------------------------------------------

// TestSubPrefixFromURL confirms each upstream URL shape collapses to
// the expected `<host>/<parent-path>` form, with the trailing chart
// name stripped (Helm re-appends it during push).
func TestSubPrefixFromURL(t *testing.T) {
	tests := map[string]struct {
		input string
		want  string
	}{
		"oci with deep path": {"oci://ghcr.io/cowboysysop/charts/whoami", "ghcr.io/cowboysysop/charts"},
		"oci minimal":        {"oci://example.com/whoami", "example.com"},
		"https Helm repo":    {"https://charts.bitnami.com/bitnami", "charts.bitnami.com"},
		"https with subpath": {"https://example.com/path/to/repo", "example.com/path/to"},
		"trailing slash":     {"oci://ghcr.io/cowboysysop/charts/whoami/", "ghcr.io/cowboysysop/charts"},
		"empty":              {"", ""},
		"whitespace":         {"   ", ""},
		"no scheme":          {"ghcr.io/foo/bar", ""},
		"scheme only":        {"oci://", ""},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := subPrefixFromURL(tc.input)
			if got != tc.want {
				t.Fatalf("subPrefixFromURL(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// chartSubPrefix
// ---------------------------------------------------------------------------

// TestChartSubPrefix_UsesURLWhenAvailable confirms a URL-sourced chart
// derives its sub-prefix from the URL host+path, regardless of source
// scheme. This is the path the canary takes.
func TestChartSubPrefix_UsesURLWhenAvailable(t *testing.T) {
	pkg := &v1alpha1.ZarfPackage{
		Metadata: v1alpha1.ZarfMetadata{Name: "ironbark-canary-app"},
		Components: []v1alpha1.ZarfComponent{
			{
				Name: "oci-component",
				Charts: []v1alpha1.ZarfChart{
					{Name: "whoami", URL: "oci://ghcr.io/cowboysysop/charts/whoami"},
				},
			},
		},
	}
	got := chartSubPrefix(pkg, "whoami")
	want := "ghcr.io/cowboysysop/charts"
	if got != want {
		t.Fatalf("chartSubPrefix = %q, want %q", got, want)
	}
}

// TestChartSubPrefix_FallsBackToPackageNameForLocalPath confirms a
// chart declared via `localPath` (no URL) falls back to the Zarf
// package name as its namespace, so local charts across two packages
// shipping the same chart name do not collide.
func TestChartSubPrefix_FallsBackToPackageNameForLocalPath(t *testing.T) {
	pkg := &v1alpha1.ZarfPackage{
		Metadata: v1alpha1.ZarfMetadata{Name: "my-package"},
		Components: []v1alpha1.ZarfComponent{
			{
				Name: "local-component",
				Charts: []v1alpha1.ZarfChart{
					{Name: "my-chart", LocalPath: "./charts/my-chart"},
				},
			},
		},
	}
	got := chartSubPrefix(pkg, "my-chart")
	want := "my-package"
	if got != want {
		t.Fatalf("chartSubPrefix = %q, want %q", got, want)
	}
}

// TestChartSubPrefix_HandlesUnknownChart confirms a defensive fallback
// when a chart cannot be matched against the parsed package (should
// not happen in practice, but the function must still return something
// sensible rather than panic or push to `oci://host/ironbark/charts/`
// with no sub-path).
func TestChartSubPrefix_HandlesUnknownChart(t *testing.T) {
	pkg := &v1alpha1.ZarfPackage{
		Metadata: v1alpha1.ZarfMetadata{Name: "my-package"},
	}
	got := chartSubPrefix(pkg, "nonexistent-chart")
	if got != "my-package" {
		t.Fatalf("chartSubPrefix = %q, want %q (package-name fallback)", got, "my-package")
	}
}

// TestChartSubPrefix_HandlesNilPackage confirms a fully-defensive
// fallback when the package metadata could not be parsed at all.
func TestChartSubPrefix_HandlesNilPackage(t *testing.T) {
	got := chartSubPrefix(nil, "anything")
	if got != "unknown" {
		t.Fatalf("chartSubPrefix = %q, want %q", got, "unknown")
	}
}

// ---------------------------------------------------------------------------
// extractComponentTarballs
// ---------------------------------------------------------------------------

// TestExtractComponentTarballs_ExtractsAndRemovesEach builds a fake
// `components/` directory holding two per-component `.tar` archives
// (the layout `zarf package create` produces) and confirms each is
// expanded in place and the original archive removed. This is the
// regression test for the AFC-32 bug where the outer `.tar.zst` was
// being extracted but the inner per-component tars were not, so
// `findChartTarballs` saw nothing.
func TestExtractComponentTarballs_ExtractsAndRemovesEach(t *testing.T) {
	componentsDir := t.TempDir()

	writePlainTar(t, filepath.Join(componentsDir, "alpha.tar"), map[string]string{
		"alpha/charts/foo-1.0.0.tgz": "fake-foo-chart",
	})
	writePlainTar(t, filepath.Join(componentsDir, "beta.tar"), map[string]string{
		"beta/charts/bar-2.0.0.tgz": "fake-bar-chart",
	})

	if err := extractComponentTarballs(componentsDir); err != nil {
		t.Fatalf("extractComponentTarballs: %v", err)
	}

	// Originals must be removed; extracted content must be in place.
	for _, name := range []string{"alpha.tar", "beta.tar"} {
		if _, err := os.Stat(filepath.Join(componentsDir, name)); !os.IsNotExist(err) {
			t.Fatalf("expected %q to be removed after extraction (err=%v)", name, err)
		}
	}
	for _, relPath := range []string{
		"alpha/charts/foo-1.0.0.tgz",
		"beta/charts/bar-2.0.0.tgz",
	} {
		if _, err := os.Stat(filepath.Join(componentsDir, relPath)); err != nil {
			t.Fatalf("expected extracted file %q: %v", relPath, err)
		}
	}
}

// TestExtractComponentTarballs_NoOpsOnMissingDir confirms a package
// with no `components/` directory (uncommon but valid) does not error.
func TestExtractComponentTarballs_NoOpsOnMissingDir(t *testing.T) {
	if err := extractComponentTarballs(filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatalf("expected nil error for missing dir, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// findChartTarballs
// ---------------------------------------------------------------------------

// TestFindChartTarballs_DiscoversTgzAndPackagesDirs confirms the
// discovery loop returns every `.tgz` found and packages every exploded
// chart directory into a `.tgz` on the fly. Both cases must work
// transparently to the caller because Zarf may stage a chart in either
// form depending on the source type. Both must also carry their chart
// name (read from `Chart.yaml`) so callers can compute the per-chart
// OCI sub-prefix.
func TestFindChartTarballs_DiscoversTgzAndPackagesDirs(t *testing.T) {
	componentsDir := t.TempDir()

	// Component A: chart staged as a pre-built .tgz (typical for `url: oci://`).
	// Use `packageChartDir` to produce a real tgz so `loader.LoadFile`
	// can read its `Chart.yaml`.
	prebuiltSrcDir := filepath.Join(componentsDir, "component-a", "charts", "prebuilt-src")
	writeMinimalChart(t, prebuiltSrcDir, "prebuilt", "1.0.0")
	prebuiltTgz, err := packageChartDir(prebuiltSrcDir, filepath.Dir(prebuiltSrcDir))
	if err != nil {
		t.Fatalf("setup: package prebuilt: %v", err)
	}
	if err := os.RemoveAll(prebuiltSrcDir); err != nil {
		t.Fatalf("setup: remove prebuilt src dir: %v", err)
	}

	// Component B: chart staged as an exploded directory (typical for `localPath`).
	explodedDir := filepath.Join(componentsDir, "component-b", "charts", "exploded")
	writeMinimalChart(t, explodedDir, "exploded", "2.0.0")

	got, err := findChartTarballs(componentsDir)
	if err != nil {
		t.Fatalf("findChartTarballs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 charts, got %d: %v", len(got), got)
	}

	foundPrebuilt, foundPackaged := false, false
	for _, chart := range got {
		if !strings.HasSuffix(chart.TgzPath, ".tgz") {
			t.Fatalf("expected .tgz path, got %q", chart.TgzPath)
		}
		if chart.TgzPath == prebuiltTgz && chart.ChartName == "prebuilt" {
			foundPrebuilt = true
		}
		if chart.ChartName == "exploded" && strings.HasPrefix(filepath.Base(chart.TgzPath), "exploded-2.0.0") {
			foundPackaged = true
		}
	}
	if !foundPrebuilt {
		t.Fatalf("pre-built chart missing or wrong name: %v", got)
	}
	if !foundPackaged {
		t.Fatalf("packaged exploded chart missing or wrong name: %v", got)
	}
}

// TestFindChartTarballs_HandlesPackageWithNoCharts confirms a package
// that has components but none of them declare charts returns an empty
// slice rather than an error. This is the common case for any package
// whose components only contribute images or git repos.
func TestFindChartTarballs_HandlesPackageWithNoCharts(t *testing.T) {
	componentsDir := t.TempDir()
	imagesOnly := filepath.Join(componentsDir, "component-x", "images")
	if err := os.MkdirAll(imagesOnly, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := findChartTarballs(componentsDir)
	if err != nil {
		t.Fatalf("findChartTarballs: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no charts, got: %v", got)
	}
}

// TestFindChartTarballs_HandlesMissingComponentsDir confirms the
// discovery loop tolerates a package that has no `components/` directory
// at all (e.g. a malformed or stripped-down archive) without erroring.
func TestFindChartTarballs_HandlesMissingComponentsDir(t *testing.T) {
	got, err := findChartTarballs(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("expected nil error for missing components dir, got %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil slice, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// packageChartDir
// ---------------------------------------------------------------------------

// TestPackageChartDir_ProducesLoadableTgz confirms an exploded chart
// dir round-trips through `packageChartDir` into a tgz that Helm can
// load back and inspect.
func TestPackageChartDir_ProducesLoadableTgz(t *testing.T) {
	chartDir := filepath.Join(t.TempDir(), "src", "demo")
	writeMinimalChart(t, chartDir, "demo", "3.4.5")

	destDir := t.TempDir()
	tgzPath, err := packageChartDir(chartDir, destDir)
	if err != nil {
		t.Fatalf("packageChartDir: %v", err)
	}
	if !strings.HasSuffix(tgzPath, ".tgz") {
		t.Fatalf("expected .tgz path, got %q", tgzPath)
	}
	if _, err := os.Stat(tgzPath); err != nil {
		t.Fatalf("expected tgz to exist at %q: %v", tgzPath, err)
	}
	// Helm encodes chart name and version into the filename, so we can
	// cheaply assert correctness by inspecting the file name.
	if filepath.Base(tgzPath) != "demo-3.4.5.tgz" {
		t.Fatalf("unexpected tgz filename: got %q, want %q", filepath.Base(tgzPath), "demo-3.4.5.tgz")
	}
}

// ---------------------------------------------------------------------------
// MirrorPackageCharts (boundary cases that do NOT require a cluster)
// ---------------------------------------------------------------------------
//
// The full happy path of `MirrorPackageCharts` opens a live tunnel to
// the in-cluster Zarf registry and pushes via Helm, so it is exercised
// by the manual smoke-test plan documented in AFC-32 rather than here.
// Only the early-return code paths that never touch the cluster are
// covered as unit tests; everything else would otherwise force these
// tests to depend on a real Kubernetes context.

// TestMirrorPackageCharts_NoOpsOnMissingDir confirms a missing input
// dir is treated as "nothing to do" (matching `runPackagesCommand`),
// not as an error. This means an `ironbark init` against a fresh repo
// that simply has no mirror packages does not blow up.
func TestMirrorPackageCharts_NoOpsOnMissingDir(t *testing.T) {
	err := MirrorPackageCharts(nil, filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("expected nil for missing dir, got %v", err)
	}
}

// TestMirrorPackageCharts_NoOpsOnEmptyDir confirms an empty packages
// dir short-circuits before any cluster connection is attempted.
// This is asserted by ensuring no error is returned even when the
// process has no kubeconfig available; if `GetCluster` were reached,
// it would error.
func TestMirrorPackageCharts_NoOpsOnEmptyDir(t *testing.T) {
	err := MirrorPackageCharts(nil, t.TempDir())
	if err != nil {
		t.Fatalf("expected nil for empty dir, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// writeZstdTestPackage creates a `.tar.zst` archive at a temp path
// containing the supplied entries (`relative-path => file contents`)
// and returns the archive path. Parent directories for each entry are
// added implicitly. Intended for tests that exercise the outer-layer
// extraction logic of a Zarf package.
func writeZstdTestPackage(t *testing.T, entries map[string]string) string {
	t.Helper()

	var buf bytes.Buffer
	zw, err := zstd.NewWriter(&buf)
	if err != nil {
		t.Fatalf("zstd writer: %v", err)
	}
	if err := writeTarEntries(t, zw, entries); err != nil {
		t.Fatalf("tar: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zstd close: %v", err)
	}

	archivePath := filepath.Join(t.TempDir(), "package.tar.zst")
	if err := os.WriteFile(archivePath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	return archivePath
}

// writePlainTar creates an uncompressed `.tar` archive at `tarPath`
// containing the supplied entries. Intended for tests that exercise
// the per-component (inner) extraction logic.
func writePlainTar(t *testing.T, tarPath string, entries map[string]string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(tarPath), 0o755); err != nil {
		t.Fatalf("mkdir for tar: %v", err)
	}
	f, err := os.Create(tarPath)
	if err != nil {
		t.Fatalf("create tar: %v", err)
	}
	defer f.Close()
	if err := writeTarEntries(t, f, entries); err != nil {
		t.Fatalf("tar: %v", err)
	}
}

// writeTarEntries writes the supplied entries (and their implicit
// parent directories) as tar headers + bodies into `w`. Shared by
// `writeZstdTestPackage` and `writePlainTar` so both archive types use
// identical tar semantics.
func writeTarEntries(t *testing.T, w io.Writer, entries map[string]string) error {
	t.Helper()

	tw := tar.NewWriter(w)
	seenDirs := map[string]bool{}
	for relPath, body := range entries {
		// Emit parent dir entries first so they have explicit headers.
		dir := filepath.Dir(relPath)
		for dir != "." && dir != "/" && !seenDirs[dir] {
			seenDirs[dir] = true
			_ = tw.WriteHeader(&tar.Header{
				Name:     dir + "/",
				Mode:     0o755,
				Typeflag: tar.TypeDir,
			})
		}

		if err := tw.WriteHeader(&tar.Header{
			Name:     relPath,
			Mode:     0o644,
			Size:     int64(len(body)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			return fmt.Errorf("header %q: %w", relPath, err)
		}
		if _, err := io.Copy(tw, strings.NewReader(body)); err != nil {
			return fmt.Errorf("body %q: %w", relPath, err)
		}
	}
	return tw.Close()
}

// writeMinimalChart writes the smallest valid Helm chart layout
// (`Chart.yaml` + a no-op templates dir) into `chartDir` with the
// supplied name + version. Helm's loader is strict about both files
// being present, so anything less fails to load.
func writeMinimalChart(t *testing.T, chartDir string, name string, version string) {
	t.Helper()

	templatesDir := filepath.Join(chartDir, "templates")
	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	chartYaml := "apiVersion: v2\nname: " + name + "\nversion: " + version + "\n"
	if err := os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(chartYaml), 0o644); err != nil {
		t.Fatalf("write Chart.yaml: %v", err)
	}
	// Empty (but present) templates dir is enough for `loader.LoadDir`.
	if err := os.WriteFile(filepath.Join(templatesDir, ".gitkeep"), nil, 0o644); err != nil {
		t.Fatalf("write .gitkeep: %v", err)
	}
}
