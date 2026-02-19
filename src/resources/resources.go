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

// Loads a template from the resources.
func loadTemplate(file string) (*template.Template, error) {
	tmpl, err := template.ParseFS(resourcesFS, file)
	if err != nil {
		return nil, err
	}

	return tmpl, nil
}
