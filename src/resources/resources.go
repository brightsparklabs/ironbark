/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package resources

import (
	"embed"
	"text/template"

	cp "github.com/otiai10/copy"
)

//go:embed resources/*
var resourcesFS embed.FS

// Copies resources from the embedded resources FS to the file system.
func Copy(source, target string) error {
	err := cp.Copy(source, target, cp.Options{FS: resourcesFS, PermissionControl: cp.AddPermission(0200)})
	return err
}

// Loads a template from the resources.
func loadTemplate(file string) (*template.Template, error) {
	tmpl, err := template.ParseFS(resourcesFS, file)
	if err != nil {
		return nil, err
	}

	return tmpl, nil
}
