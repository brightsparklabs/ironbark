/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package resources

import (
	"embed"
	cp "github.com/otiai10/copy"
	"text/template"
)

//go:embed resources/*
var resourcesFS embed.FS

// Copies resources from the embedded resources FS to the file system.
// `source` is relative to the "resources" directory.
func Copy(source string, target string) error {
	err := cp.Copy("resources/"+source, target, cp.Options{FS: resourcesFS, PermissionControl: cp.AddPermission(0200)})
	return err
}

// LoadTemplate loads a text template from the embedded resources.
// `file` is relative to the embedded "resources" directory.
func LoadTemplate(file string) (*template.Template, error) {
	tmpl, err := template.ParseFS(resourcesFS, "resources/"+file)
	if err != nil {
		return nil, err
	}

	return tmpl, nil
}

// ReadFile reads a file from the embedded resources and returns its
// raw bytes. `file` is relative to the embedded "resources" directory.
// Returns an error wrapping `fs.ErrNotExist` if the file is not
// present in the embed (callers can branch on `errors.Is(err,
// fs.ErrNotExist)` to apply a fallback).
func ReadFile(file string) ([]byte, error) {
	return resourcesFS.ReadFile("resources/" + file)
}

// Exists reports whether the named file is present in the embedded
// resources. `file` is relative to the embedded "resources" directory.
func Exists(file string) bool {
	_, err := resourcesFS.Open("resources/" + file)
	return err == nil
}
