package keeldesktop

import (
	"testing"

	templates "github.com/openmodu/onecatch/internal/domain/prompttemplates"
)

func TestTemplateEditorUsesServiceAndKeepsDraftOnConflict(t *testing.T) {
	a, _ := skillTestApp(t)
	a.initLibrary()
	draft := templates.Template{ID: "custom-editor", Name: "编辑器模板", Body: "{{selection}}"}
	a.library.TemplateEditing = &draft
	a.saveTemplate()
	if a.errorText != "" || a.library.TemplateEditing != nil || a.library.TemplateID != draft.ID {
		t.Fatalf("save did not update editor: %s %+v", a.errorText, a.library.TemplateEditing)
	}
	loaded, err := a.backend.service.LoadPromptTemplates(a.ctx)
	if err != nil {
		t.Fatal(err)
	}
	other := draft
	other.Name = "其他窗口修改"
	if _, err := a.backend.service.SavePromptTemplate(a.ctx, other, loaded.Revision); err != nil {
		t.Fatal(err)
	}
	draft.Name = "尚未保存的草稿"
	a.library.TemplateEditing = &draft
	a.saveTemplate()
	if a.errorText == "" || a.library.TemplateEditing == nil || a.library.TemplateEditing.Name != draft.Name {
		t.Fatal("conflict discarded draft or failed silently")
	}
}
