package gen

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
)

//go:embed templates/*.go.tmpl
var tmplFS embed.FS

var tmpls = template.Must(template.ParseFS(tmplFS, "templates/*.go.tmpl"))

func render(name string, data any) string {
	var b bytes.Buffer
	if err := tmpls.ExecuteTemplate(&b, name, data); err != nil {
		panic(fmt.Errorf("render %s: %w", name, err))
	}
	return b.String()
}
