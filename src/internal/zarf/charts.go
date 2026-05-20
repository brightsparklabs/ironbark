/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

package zarf

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/registry"
	"sigs.k8s.io/yaml"
)

// ChartMirrorPrefix is the predictable OCI path prefix under which Ironbark
// publishes mirrored Helm charts in the in-cluster Zarf registry. Every chart
// found in a mirror package is pushed under this prefix, so consumers
// (ArgoCD `Application` specs, `helm install`, etc.) can reference it via a
// predictable URL.
//
// The `ironbark/` namespace is intentionally separated from the `charts`
// scope with a `/` rather than a `-`, so future Ironbark-mirrored
// artefacts (e.g. `ironbark/wasm`, `ironbark/workflows`) can sit
// alongside without breaking existing references.
const ChartMirrorPrefix = "ironbark/charts"

// MirrorPackageCharts walks every Zarf package tarball under `dir`, extracts
// each Helm chart staged inside, and pushes it to the in-cluster Zarf
// registry under `oci://<registry>/ironbark/charts/<provenance>/<chart-name>:<chart-version>`
// (see `chartSubPrefix` for how the `<provenance>` segment is derived).
//
// This works around the fact that `zarf package mirror-resources` does not
// mirror Helm charts (see AFC-32); only images and Git repos are mirrored
// by Zarf itself. Without this step, any chart declared in a mirror package
// (via `charts[].localPath`, `gitPath`, `url: https://`, or `url: oci://`)
// is missing from the in-cluster registry in air-gapped environments.
//
// Any single chart-push failure aborts the entire operation; no chart is
// ever silently skipped, so callers get a clear error if the cluster ends
// up in a partially-mirrored state.
//
// Returns `nil` (with a warning log) if `dir` does not exist, mirroring the
// behaviour of `runPackagesCommand`.
func MirrorPackageCharts(ctx context.Context, dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		slog.Warn("Not searching for package charts as directory does not exist", "dir", dir)
		return nil
	}

	// Collect every package tarball under `dir` before we open any tunnel,
	// so we do not pay the tunnel setup cost when there is nothing to do.
	var packagePaths []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// Zarf packages are zstd-compressed tarballs.
		if strings.HasSuffix(path, ".tar.zst") {
			packagePaths = append(packagePaths, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("could not walk packages dir `%s`: %w", dir, err)
	}

	if len(packagePaths) == 0 {
		slog.Info("No Zarf packages found to mirror charts from", "dir", dir)
		return nil
	}

	cluster, err := GetCluster(ctx)
	if err != nil {
		return fmt.Errorf("could not get zarf cluster: %w", err)
	}
	registryInfo, err := GetRegistryInfo(ctx)
	if err != nil {
		return fmt.Errorf("could not get zarf registry info: %w", err)
	}

	// Open a single tunnel for the entire mirror operation so we do not
	// repeatedly pay the port-forward setup cost per package.
	endpoint, tunnel, err := cluster.ConnectToZarfRegistryEndpoint(ctx, *registryInfo)
	if err != nil {
		return fmt.Errorf("could not connect to zarf registry endpoint: %w", err)
	}
	if tunnel != nil {
		defer tunnel.Close()
	}

	// The endpoint string returned by Zarf is a `host:port` (no scheme).
	// Helm's registry client expects the bare host for `Login`, and the
	// OCI ref expects `oci://host/...` for `Push`.
	host, err := registryHost(endpoint)
	if err != nil {
		return fmt.Errorf("could not parse registry endpoint `%s`: %w", endpoint, err)
	}
	slog.Debug("Connected to zarf registry tunnel", "endpoint", endpoint, "host", host)

	helmClient, err := registry.NewClient()
	if err != nil {
		return fmt.Errorf("could not create helm registry client: %w", err)
	}
	// The in-cluster Zarf registry is plain HTTP by default, so log in
	// over plain text. Push will also be configured to use plain HTTP.
	err = helmClient.Login(host,
		registry.LoginOptBasicAuth(registryInfo.PushUsername, registryInfo.PushPassword),
		registry.LoginOptPlainText(true),
	)
	if err != nil {
		return fmt.Errorf("could not log in to zarf registry: %w", err)
	}
	defer func() {
		if logoutErr := helmClient.Logout(host); logoutErr != nil {
			slog.Warn("Failed to log out of zarf registry", "host", host, "err", logoutErr)
		}
	}()

	ociBase := fmt.Sprintf("oci://%s/%s", host, ChartMirrorPrefix)
	pusher := action.NewPushWithOpts(
		action.WithPushConfig(&action.Configuration{RegistryClient: helmClient}),
		action.WithPlainHTTP(true),
	)

	for _, packagePath := range packagePaths {
		if err := mirrorChartsFromPackage(ctx, packagePath, pusher, ociBase); err != nil {
			return fmt.Errorf("could not mirror charts from package `%s`: %w", packagePath, err)
		}
	}

	return nil
}

// discoveredChart pairs a chart `.tgz` file (ready to push) with the
// declared chart name from `zarf.yaml`. The chart name is needed so
// `mirrorChartsFromPackage` can compute the correct per-chart OCI
// sub-prefix derived from the source URL (or fall back to the Zarf
// package name).
type discoveredChart struct {
	TgzPath   string
	ChartName string
}

// mirrorChartsFromPackage extracts `packagePath` to a temporary directory,
// discovers every Helm chart staged inside, and pushes each chart to a
// sub-path of `ociBase` (`oci://<host>/ironbark/charts`).
//
// Each chart's sub-path preserves upstream provenance where possible
// (see `chartSubPrefix`), so the canary chart sourced from
// `oci://ghcr.io/cowboysysop/charts/whoami` lands at
// `oci://<host>/ironbark/charts/ghcr.io/cowboysysop/charts/whoami:6.0.0`.
//
// The temporary extraction directory is removed before the function returns.
func mirrorChartsFromPackage(ctx context.Context, packagePath string, pusher *action.Push, ociBase string) error {
	tmpDir, err := os.MkdirTemp("", "ironbark-mirror-charts-*")
	if err != nil {
		return fmt.Errorf("could not create temp dir: %w", err)
	}
	defer func() {
		if rmErr := os.RemoveAll(tmpDir); rmErr != nil {
			slog.Warn("Failed to clean up temp dir", "dir", tmpDir, "err", rmErr)
		}
	}()

	if err := extractZstdTarball(packagePath, tmpDir); err != nil {
		return fmt.Errorf("could not extract package: %w", err)
	}

	// A Zarf package nests its component payloads one level deeper than
	// the rest of the archive: the outer `.tar.zst` only contains a
	// per-component `components/<name>.tar` file, which we must extract
	// in turn to reach `<name>/charts/<chart-name>-<version>.tgz`. Walk
	// every component tar, extract each in place, and then look for
	// chart `.tgz` archives under the resulting tree.
	//
	// The chart-mirror loop intentionally walks the on-disk layout
	// directly rather than re-parsing each chart's source declaration
	// in `zarf.yaml`. This works because `zarf package create`
	// normalises all four chart source types (`localPath`, `gitPath`,
	// `url: https://`, `url: oci://`) into a packaged `.tgz` at the
	// same location when it bakes the package.
	componentsDir := filepath.Join(tmpDir, "components")
	if err := extractComponentTarballs(componentsDir); err != nil {
		return fmt.Errorf("could not extract component tarballs: %w", err)
	}

	pkg, err := parseZarfPackage(tmpDir)
	if err != nil {
		return fmt.Errorf("could not parse package metadata: %w", err)
	}

	charts, err := findChartTarballs(componentsDir)
	if err != nil {
		return fmt.Errorf("could not discover charts: %w", err)
	}
	if len(charts) == 0 {
		slog.Debug("Package contains no charts to mirror", "package", packagePath)
		return nil
	}

	for _, chart := range charts {
		subPrefix := chartSubPrefix(pkg, chart.ChartName)
		target := ociBase + "/" + subPrefix
		if err := pushChart(ctx, pusher, chart.TgzPath, target); err != nil {
			return fmt.Errorf("could not push chart `%s`: %w", chart.TgzPath, err)
		}
	}
	return nil
}

// parseZarfPackage reads `zarf.yaml` from an extracted Zarf package
// directory and unmarshals it into Zarf's own typed schema. Used to
// look up the source-URL declarations for each chart so the mirrored
// chart's OCI path can preserve upstream provenance.
func parseZarfPackage(extractDir string) (*v1alpha1.ZarfPackage, error) {
	data, err := os.ReadFile(filepath.Join(extractDir, "zarf.yaml"))
	if err != nil {
		return nil, fmt.Errorf("could not read zarf.yaml: %w", err)
	}
	var pkg v1alpha1.ZarfPackage
	if err := yaml.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("could not unmarshal zarf.yaml: %w", err)
	}
	return &pkg, nil
}

// chartSubPrefix derives the per-chart sub-path appended to the
// `ironbark/charts/` mirror prefix.
//
// For charts declared with a `url:` (`oci://` or `https://`), the scheme
// is stripped and the remaining `host/path` is preserved verbatim so the
// canary lands at e.g. `ghcr.io/cowboysysop/charts`.
//
// For charts without a URL (`localPath`, `gitPath`, or any future
// non-URL source type), the Zarf package name is used as the namespace
// instead, so two packages shipping a chart named `my-chart` do not
// collide.
//
// The chart name itself and the version tag are appended by Helm when
// pushing, so callers must NOT include them in the returned sub-prefix.
func chartSubPrefix(pkg *v1alpha1.ZarfPackage, chartName string) string {
	if pkg != nil {
		for _, component := range pkg.Components {
			for _, chart := range component.Charts {
				if chart.Name != chartName {
					continue
				}
				if sub := subPrefixFromURL(chart.URL); sub != "" {
					return sub
				}
				return pkg.Metadata.Name
			}
		}
	}
	// Fallback for the case where the chart could not be matched in the
	// parsed package (defensive: should not happen for well-formed
	// packages, but we still want to push the chart somewhere sensible).
	if pkg != nil && pkg.Metadata.Name != "" {
		return pkg.Metadata.Name
	}
	return "unknown"
}

// subPrefixFromURL strips the scheme from a chart URL and returns the
// `host/path` portion suitable for use as an OCI path component. The
// trailing chart-name segment of the URL (if any) is dropped because
// Helm's push will re-append `<chart-name>:<chart-version>` itself.
//
// Returns an empty string for any URL we cannot meaningfully interpret
// (empty, scheme-only, unparseable), so the caller can fall back to a
// non-URL strategy.
func subPrefixFromURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	// Drop the scheme. Both `oci://` and `https://` flow through this
	// branch; bare URLs (no scheme) are intentionally treated as
	// invalid because we cannot tell where the host ends.
	idx := strings.Index(rawURL, "://")
	if idx < 0 {
		return ""
	}
	hostPath := rawURL[idx+3:]
	hostPath = strings.TrimSuffix(hostPath, "/")
	if hostPath == "" {
		return ""
	}
	// Drop the last path segment (the chart name) so the result is the
	// parent "repository" path. Helm re-appends the chart name during
	// push, and we never want to double it up.
	if i := strings.LastIndex(hostPath, "/"); i > 0 {
		return hostPath[:i]
	}
	// `host`-only URL (no path). Use the host itself as the sub-prefix.
	return hostPath
}

// extractComponentTarballs extracts every `components/<name>.tar`
// archive into its containing directory and then removes the original
// `.tar` file. After this step, the layout used by `findChartTarballs`
// (`<component>/charts/<chart>.tgz`) is present on disk.
//
// No-op if `componentsDir` does not exist (a package with zero
// components is valid).
func extractComponentTarballs(componentsDir string) error {
	if _, err := os.Stat(componentsDir); os.IsNotExist(err) {
		return nil
	}

	entries, err := os.ReadDir(componentsDir)
	if err != nil {
		return fmt.Errorf("could not read components dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tar") {
			continue
		}
		tarPath := filepath.Join(componentsDir, entry.Name())
		if err := extractPlainTarball(tarPath, componentsDir); err != nil {
			return fmt.Errorf("could not extract component tarball `%s`: %w", tarPath, err)
		}
		if err := os.Remove(tarPath); err != nil {
			return fmt.Errorf("could not remove component tarball `%s`: %w", tarPath, err)
		}
	}
	return nil
}

// findChartTarballs locates every Helm chart staged under
// `componentsDir/<component>/charts/`. Each entry is returned as a
// `discoveredChart` (`.tgz` path + declared chart name).
//
// If Zarf has staged a chart as an exploded directory rather than a
// `.tgz`, this helper packages it on the fly via `chartutil.Save` and
// returns the path to the resulting archive (in the same parent dir, so
// it is cleaned up with the rest of the extraction tempdir).
//
// The chart name is read from each chart's `Chart.yaml` (via
// `loader.LoadFile`) rather than parsed from the filename, because the
// filename convention `<name>-<version>.tgz` is a Helm convention but
// not strictly required, whereas `Chart.yaml` is authoritative.
//
// Callers must run `extractComponentTarballs` first; this helper
// expects each component to already be expanded on disk.
func findChartTarballs(componentsDir string) ([]discoveredChart, error) {
	if _, err := os.Stat(componentsDir); os.IsNotExist(err) {
		// A package with no components has nothing to mirror.
		return nil, nil
	}

	components, err := os.ReadDir(componentsDir)
	if err != nil {
		return nil, fmt.Errorf("could not read components dir: %w", err)
	}

	var charts []discoveredChart
	for _, component := range components {
		if !component.IsDir() {
			continue
		}
		chartsDir := filepath.Join(componentsDir, component.Name(), "charts")
		if _, err := os.Stat(chartsDir); os.IsNotExist(err) {
			continue
		}

		entries, err := os.ReadDir(chartsDir)
		if err != nil {
			return nil, fmt.Errorf("could not read charts dir `%s`: %w", chartsDir, err)
		}
		for _, entry := range entries {
			entryPath := filepath.Join(chartsDir, entry.Name())
			var tgzPath string
			if entry.IsDir() {
				// Exploded chart directory: package it into a `.tgz`
				// alongside the source dir.
				p, err := packageChartDir(entryPath, chartsDir)
				if err != nil {
					return nil, fmt.Errorf("could not package chart dir `%s`: %w", entryPath, err)
				}
				tgzPath = p
			} else if strings.HasSuffix(entry.Name(), ".tgz") {
				tgzPath = entryPath
			} else {
				continue
			}

			chrt, err := loader.LoadFile(tgzPath)
			if err != nil {
				return nil, fmt.Errorf("could not load chart `%s`: %w", tgzPath, err)
			}
			charts = append(charts, discoveredChart{
				TgzPath:   tgzPath,
				ChartName: chrt.Name(),
			})
		}
	}
	return charts, nil
}

// packageChartDir packages an exploded Helm chart directory (`chartDir`)
// into a `.tgz` archive inside `destDir` and returns the path to the
// resulting archive. This is needed because the Helm push action can
// only push archives, not directories.
func packageChartDir(chartDir string, destDir string) (string, error) {
	chrt, err := loader.LoadDir(chartDir)
	if err != nil {
		return "", fmt.Errorf("could not load chart from `%s`: %w", chartDir, err)
	}
	tgzPath, err := chartutil.Save(chrt, destDir)
	if err != nil {
		return "", fmt.Errorf("could not save chart `%s` as tgz: %w", chrt.Name(), err)
	}
	return tgzPath, nil
}

// pushChart uploads a single chart `.tgz` to the OCI target. Helm's
// push action takes the parent path (`ociTarget`) and appends
// `<chart-name>:<chart-version>` to it itself, so `pushChart` does not
// need to format the trailing ref.
func pushChart(_ context.Context, pusher *action.Push, chartTgz string, ociTarget string) error {
	// Load just for metadata so we can log a meaningful "what was pushed".
	chrt, err := loader.LoadFile(chartTgz)
	if err != nil {
		return fmt.Errorf("could not load chart `%s`: %w", chartTgz, err)
	}
	chartRef := fmt.Sprintf("%s/%s:%s", ociTarget, chrt.Name(), chrt.Metadata.Version)

	slog.Info("Mirroring helm chart to in-cluster registry",
		"chart", chrt.Name(),
		"version", chrt.Metadata.Version,
		"ref", chartRef,
	)

	if _, err := pusher.Run(chartTgz, ociTarget); err != nil {
		return fmt.Errorf("could not push chart to `%s`: %w", ociTarget, err)
	}
	return nil
}

// extractZstdTarball decompresses a `.tar.zst` archive at `archivePath`
// into `destDir`. Behaviour mirrors `tar -I zstd -xf <archive> -C <dest>`
// with directory-traversal protection (entries with `..` in their paths
// are rejected).
func extractZstdTarball(archivePath string, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("could not open archive `%s`: %w", archivePath, err)
	}
	defer f.Close()

	zr, err := zstd.NewReader(f)
	if err != nil {
		return fmt.Errorf("could not create zstd reader: %w", err)
	}
	defer zr.Close()

	return extractTarStream(zr, destDir)
}

// extractPlainTarball extracts an uncompressed `.tar` archive at
// `archivePath` into `destDir`. Behaviour mirrors `tar -xf <archive> -C
// <dest>` with the same directory-traversal protection as
// `extractZstdTarball`. Used to expand the per-component `.tar` files
// nested inside a Zarf package.
func extractPlainTarball(archivePath string, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("could not open archive `%s`: %w", archivePath, err)
	}
	defer f.Close()
	return extractTarStream(f, destDir)
}

// extractTarStream walks a tar stream and materialises each entry under
// `destDir`. Rejects any entry whose path escapes `destDir` (defence in
// depth against malicious or malformed archives). Shared by both
// `extractZstdTarball` and `extractPlainTarball`.
func extractTarStream(r io.Reader, destDir string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("could not read tar header: %w", err)
		}
		// Reject any entry that would write outside `destDir`.
		cleanName := filepath.Clean(hdr.Name)
		if strings.HasPrefix(cleanName, "..") || filepath.IsAbs(cleanName) {
			return fmt.Errorf("refusing to extract entry outside destination: `%s`", hdr.Name)
		}
		targetPath := filepath.Join(destDir, cleanName)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.FileMode(hdr.Mode)|0o700); err != nil {
				return fmt.Errorf("could not create dir `%s`: %w", targetPath, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fmt.Errorf("could not create parent dir for `%s`: %w", targetPath, err)
			}
			if err := writeTarFile(tr, targetPath, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		default:
			// Skip symlinks, devices, etc. Zarf packages do not use them
			// for chart payloads, so we silently ignore them rather than
			// erroring out (which would be hostile to forward compatibility).
			slog.Debug("Skipping non-regular tar entry", "name", hdr.Name, "typeflag", hdr.Typeflag)
		}
	}
}

// writeTarFile writes the body of a tar entry to `targetPath` with the
// supplied mode. Split out for readability and so the file handle is
// always closed even on partial writes.
func writeTarFile(tr *tar.Reader, targetPath string, mode os.FileMode) error {
	out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("could not create file `%s`: %w", targetPath, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, tr); err != nil {
		return fmt.Errorf("could not write file `%s`: %w", targetPath, err)
	}
	return nil
}

// registryHost normalises a registry endpoint (which Zarf returns as
// either `host:port` or `scheme://host:port`) into the bare `host:port`
// form that Helm's registry client expects for `Login` and that we
// prepend with `oci://` for push refs.
func registryHost(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", errors.New("registry endpoint is empty")
	}
	if !strings.Contains(endpoint, "://") {
		return endpoint, nil
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("could not parse endpoint url: %w", err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("endpoint url has no host: `%s`", endpoint)
	}
	return u.Host, nil
}
