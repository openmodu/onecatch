package prompttemplates

import (
	"reflect"
	"strings"
	"testing"

	domain "github.com/openmodu/onecatch/internal/domain/prompttemplates"
)

func TestTemplateExpansionNestedAndLiteralMaterial(t *testing.T) {
	child := domain.Template{ID: "custom-child", Name: "Nested", Body: "{{project}}: {{selection}} / {{arg:重点}}"}
	parent := domain.Template{ID: "custom-parent", Body: "Before {{template:custom-child}} After {{arg:重点}}"}
	library := []domain.Template{child, parent}
	args, err := Arguments(parent, library)
	if err != nil || !reflect.DeepEqual(args, []string{"重点"}) {
		t.Fatalf("arguments: %v %v", args, err)
	}
	out, err := Render(parent, library, map[string]string{"project": "OneCatch", "selection": "{{arg:do-not-evaluate}}"}, map[string]string{"重点": "tests"})
	if err != nil || out != "Before OneCatch: {{arg:do-not-evaluate}} / tests After tests" {
		t.Fatalf("expansion: %q %v", out, err)
	}
}
func TestTemplateExpansionRejectsInvalidReferences(t *testing.T) {
	for _, test := range []struct{ Name, Body, Want string }{{"missing", "{{template:missing}}", "不存在"}, {"unknown", "{{password}}", "不支持"}, {"selection", "{{selection}}", "输入"}, {"argument", "{{arg:重点}}", "填写"}} {
		t.Run(test.Name, func(t *testing.T) {
			p := domain.Template{ID: "custom-one", Body: test.Body}
			_, err := Render(p, []domain.Template{p}, nil, nil)
			if err == nil || !strings.Contains(err.Error(), test.Want) {
				t.Fatalf("expected %s: %v", test.Want, err)
			}
		})
	}
	p := domain.Template{ID: "custom-one", Body: "{{template:custom-two}}"}
	q := domain.Template{ID: "custom-two", Body: "{{template:custom-one}}"}
	if _, err := Arguments(p, []domain.Template{p, q}); err == nil || !strings.Contains(err.Error(), "循环") {
		t.Fatalf("cycle: %v", err)
	}
}
func TestTemplateExpansionBounds(t *testing.T) {
	p := domain.Template{ID: "custom-one", Body: strings.Repeat("{{project}}", 1001)}
	if _, err := Arguments(p, []domain.Template{p}); err == nil {
		t.Fatal("unbounded token count")
	}
	p.Body = "{{selection}}"
	if _, err := Render(p, []domain.Template{p}, map[string]string{"selection": strings.Repeat("x", 128001)}, nil); err == nil {
		t.Fatal("unbounded output")
	}
	p.Body = strings.Repeat("x", 32001)
	if _, err := Arguments(p, []domain.Template{p}); err == nil {
		t.Fatal("unbounded body")
	}
}
func TestTemplateLibraryValidation(t *testing.T) {
	p := domain.Template{ID: "custom-one", Name: "Example", Body: "{{selection}}"}
	if err := validateTemplates([]domain.Template{p}); err != nil {
		t.Fatal(err)
	}
	if err := validateTemplates([]domain.Template{p, p}); err == nil {
		t.Fatal("duplicate id accepted")
	}
	p.Builtin = true
	if err := validateTemplates([]domain.Template{p}); err == nil {
		t.Fatal("custom template masquerading as built in")
	}
}
