/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

package cmd

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"brightsparklabs.com/ironbark/internal/zarf"
	"github.com/spf13/cobra"
)

// -----------------------------------------------------------------------------
// UPLOAD COMMAND
// -----------------------------------------------------------------------------

// newUploadCmd creates the upload command for uploading and mirroring packages.
func newUploadCmd() *cobra.Command {
	uploadCmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload and mirror Zarf packages to the cluster",
		Long: `Upload and mirror Zarf packages to the cluster's internal OCI registry.

The upload command mirrors packages (images and Helm charts) without deploying them.
This is useful for pre-loading packages into the cluster's registry.`,
	}

	uploadCmd.AddCommand(newUploadPackageCmd())
	uploadCmd.AddCommand(newUploadPackagesCmd())

	return uploadCmd
}

// newUploadPackageCmd creates the upload package subcommand for single package.
func newUploadPackageCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "package <file.tar.zst>",
		Short: "Upload and mirror a single Zarf package",
		Long: `Upload and mirror a single Zarf package to the cluster's internal OCI registry.

Mirrors the package's container images and Helm charts without deploying the package.`,
		Args: cobra.ExactArgs(1),
		RunE: uploadPackageExec,
	}
}

// newUploadPackagesCmd creates the upload packages subcommand for bulk upload.
func newUploadPackagesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "packages <archive.tar.gz|zip>",
		Short: "Upload and mirror multiple Zarf packages from an archive",
		Long: `Upload and mirror multiple Zarf packages from a tar.gz or zip archive.

Extracts .tar.zst packages from the archive and mirrors them to the cluster's
internal OCI registry. Supports both tar.gz and zip archive formats.`,
		Args: cobra.ExactArgs(1),
		RunE: uploadPackagesExec,
	}
}

// -----------------------------------------------------------------------------
// EXECUTION FUNCTIONS
// -----------------------------------------------------------------------------

// uploadPackageExec uploads and mirrors a single Zarf package.
func uploadPackageExec(cmd *cobra.Command, args []string) error {
	packagePath := args[0]

	// Validate file exists and is a .tar.zst file.
	if _, err := os.Stat(packagePath); os.IsNotExist(err) {
		return NewUserError("package file not found: %s", packagePath)
	}

	if !strings.HasSuffix(packagePath, ".tar.zst") {
		return NewUserError("package must be a .tar.zst file: %s", packagePath)
	}

	slog.Info("Uploading single package", "path", packagePath)

	// Create temp directory for the package.
	tmpDir, err := os.MkdirTemp("", "ironbark-upload-*")
	if err != nil {
		return NewUserError("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Copy package to temp directory (zarf commands expect a directory).
	tmpPackagePath := filepath.Join(tmpDir, filepath.Base(packagePath))
	if err := copyFile(packagePath, tmpPackagePath); err != nil {
		return NewUserError("failed to copy package: %v", err)
	}

	// Mirror packages (images and charts).
	ctx := context.Background()
	if err := zarf.MirrorPackages(tmpDir); err != nil {
		return NewUserError("failed to mirror package: %v", err)
	}

	if err := zarf.MirrorPackageCharts(ctx, tmpDir); err != nil {
		return NewUserError("failed to mirror charts: %v", err)
	}

	slog.Info("Package mirrored successfully", "package", filepath.Base(packagePath))
	return nil
}

// uploadPackagesExec uploads and mirrors multiple packages from an archive.
func uploadPackagesExec(cmd *cobra.Command, args []string) error {
	archivePath := args[0]

	// Validate file exists.
	if _, err := os.Stat(archivePath); os.IsNotExist(err) {
		return NewUserError("archive file not found: %s", archivePath)
	}

	// Determine archive type.
	isZip := strings.HasSuffix(archivePath, ".zip")
	isTarGz := strings.HasSuffix(archivePath, ".tar.gz") || strings.HasSuffix(archivePath, ".tgz")

	if !isZip && !isTarGz {
		return NewUserError("archive must be .tar.gz, .tgz, or .zip format: %s", archivePath)
	}

	slog.Info("Uploading packages from archive", "path", archivePath, "format", map[bool]string{true: "zip", false: "tar.gz"}[isZip])

	// Create temp directory for extraction.
	tmpDir, err := os.MkdirTemp("", "ironbark-bulk-*")
	if err != nil {
		return NewUserError("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Extract packages.
	var packageCount int
	if isZip {
		packageCount, err = extractZipPackages(archivePath, tmpDir)
	} else {
		packageCount, err = extractTarGzPackages(archivePath, tmpDir)
	}

	if err != nil {
		return NewUserError("failed to extract archive: %v", err)
	}

	if packageCount == 0 {
		return NewUserError("no Zarf packages (.tar.zst) found in archive")
	}

	slog.Info("Extracted packages from archive", "count", packageCount)

	// Mirror packages (images and charts).
	ctx := context.Background()
	if err := zarf.MirrorPackages(tmpDir); err != nil {
		return NewUserError("failed to mirror packages: %v", err)
	}

	if err := zarf.MirrorPackageCharts(ctx, tmpDir); err != nil {
		return NewUserError("failed to mirror charts: %v", err)
	}

	slog.Info("Bulk packages mirrored successfully", "count", packageCount)
	return nil
}

// -----------------------------------------------------------------------------
// HELPER FUNCTIONS
// -----------------------------------------------------------------------------

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0644)
}

// extractZipPackages extracts .tar.zst files from a zip archive.
func extractZipPackages(archivePath, destDir string) (int, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open zip: %w", err)
	}
	defer reader.Close()

	count := 0
	for _, file := range reader.File {
		if !strings.HasSuffix(file.Name, ".tar.zst") {
			continue
		}

		rc, err := file.Open()
		if err != nil {
			return count, fmt.Errorf("failed to open file in zip: %w", err)
		}

		destPath := filepath.Join(destDir, filepath.Base(file.Name))
		outFile, err := os.Create(destPath)
		if err != nil {
			rc.Close()
			return count, fmt.Errorf("failed to create output file: %w", err)
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return count, fmt.Errorf("failed to extract file: %w", err)
		}

		count++
		slog.Debug("Extracted package from zip", "file", file.Name)
	}

	return count, nil
}

// extractTarGzPackages extracts .tar.zst files from a tar.gz archive.
func extractTarGzPackages(archivePath, destDir string) (int, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return 0, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	count := 0
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, fmt.Errorf("failed to read tar entry: %w", err)
		}

		if header.Typeflag != tar.TypeReg || !strings.HasSuffix(header.Name, ".tar.zst") {
			continue
		}

		destPath := filepath.Join(destDir, filepath.Base(header.Name))
		outFile, err := os.Create(destPath)
		if err != nil {
			return count, fmt.Errorf("failed to create output file: %w", err)
		}

		_, err = io.Copy(outFile, tarReader)
		outFile.Close()

		if err != nil {
			return count, fmt.Errorf("failed to extract file: %w", err)
		}

		count++
		slog.Debug("Extracted package from tar.gz", "file", header.Name)
	}

	return count, nil
}
