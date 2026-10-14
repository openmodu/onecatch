package desktop

import (
	"context"
	domain "github.com/openmodu/onecatch/internal/domain/prompttemplates"
	templates "github.com/openmodu/onecatch/internal/usecase/prompttemplates"
)

func (a *Service) LoadPromptTemplates(ctx context.Context) (domain.Library, error) {
	return a.promptTemplates.Load(ctx)
}

func (a *Service) SavePromptTemplate(ctx context.Context, draft domain.Template, revision string) (domain.Library, error) {
	return a.promptTemplates.Save(ctx, draft, revision)
}

func (a *Service) PromptTemplateArguments(item domain.Template, library []domain.Template) ([]string, error) {
	return templates.Arguments(item, library)
}

func (a *Service) RenderPromptTemplate(item domain.Template, library []domain.Template, values, args map[string]string) (string, error) {
	return templates.Render(item, library, values, args)
}
