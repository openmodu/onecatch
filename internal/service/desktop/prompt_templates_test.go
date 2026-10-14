package desktop

import (
	"context"
	"testing"

	domain "github.com/openmodu/onecatch/internal/domain/prompttemplates"
)

func TestPromptTemplateServiceWithoutUI(t *testing.T) {
	service, _ := newLocalTestApp(t, completingEngine{})
	ctx := context.Background()
	library, err := service.LoadPromptTemplates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	draft := domain.Template{ID: "custom-headless", Name: "无界面模板", Body: "{{project}} / {{arg:重点}} / {{selection}}"}
	saved, err := service.SavePromptTemplate(ctx, draft, library.Revision)
	if err != nil {
		t.Fatal(err)
	}
	args, err := service.PromptTemplateArguments(draft, saved.Templates)
	if err != nil || len(args) != 1 || args[0] != "重点" {
		t.Fatalf("arguments: %v %v", args, err)
	}
	text, err := service.RenderPromptTemplate(draft, saved.Templates, map[string]string{"project": "OneCatch", "selection": "材料"}, map[string]string{"重点": "测试"})
	if err != nil || text != "OneCatch / 测试 / 材料" {
		t.Fatalf("render: %q %v", text, err)
	}
	loaded, err := service.LoadPromptTemplates(ctx)
	if err != nil || loaded.Revision != saved.Revision {
		t.Fatalf("reload: %+v %v", loaded, err)
	}
}
