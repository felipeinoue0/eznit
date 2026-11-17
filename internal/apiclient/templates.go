package apiclient

import (
	"net/http"
	"net/url"
	"sort"

	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/pkg/types"
)

// Templates: the catalogue and one template's contents. The DTOs are the
// domain package's rather than a second copy of them, so a field the server
// grows is a field this client decodes without another definition to keep in
// step — the same choice the CLI makes.

// ListTemplates returns the catalogue: every public template, plus the
// caller's own private ones when the client carries a session.
//
// The kind filter is a query parameter on the catalogue route; the "mine"
// route takes none, so a caller that wants both applies FilterTemplates to the
// result.
func (c *Client) ListTemplates(mine bool, kind string) ([]templates.TemplateData, error) {
	path := "/v1/templates"
	if mine {
		path += "/mine"
	} else if kind != "" {
		path += "?" + url.Values{"kind": {kind}}.Encode()
	}

	var envelope types.Envelope[templates.ListResponse]

	if err := c.do(http.MethodGet, path, nil, &envelope); err != nil {
		return nil, err
	}

	if envelope.Data.Data == nil {
		return []templates.TemplateData{}, nil
	}

	return envelope.Data.Data, nil
}

// GetTemplate returns one template with its files, variables and commands in
// the same answer: a text file carries its content and a binary file carries
// base64, so the detail is the whole template.
func (c *Client) GetTemplate(id string) (*templates.TemplateDetail, error) {
	var envelope types.Envelope[templates.TemplateDetail]

	if err := c.do(http.MethodGet, "/v1/templates/"+url.PathEscape(id), nil, &envelope); err != nil {
		return nil, err
	}

	return &envelope.Data, nil
}

// FilterTemplates keeps the templates of one kind; an empty kind keeps them
// all. The "mine" route cannot filter on the server, so a caller asking for
// both filters here.
func FilterTemplates(items []templates.TemplateData, kind string) []templates.TemplateData {
	if kind == "" {
		return items
	}

	kept := make([]templates.TemplateData, 0, len(items))

	for _, item := range items {
		if item.Kind == kind {
			kept = append(kept, item)
		}
	}

	return kept
}

// PublishTemplate stores a new template for the session's account and returns
// it as the server stored it — with its id, the slug it derived and version 1.
func (c *Client) PublishTemplate(request templates.PublishRequest) (*templates.TemplateDetail, error) {
	var envelope types.Envelope[templates.TemplateDetail]

	if err := c.do(http.MethodPost, "/v1/templates", jsonBody(request), &envelope); err != nil {
		return nil, err
	}

	return &envelope.Data, nil
}

// UpdateTemplate replaces one of the caller's templates with request — the API
// has no partial update — and returns the stored result, one version higher.
func (c *Client) UpdateTemplate(id string, request templates.PublishRequest) (*templates.TemplateDetail, error) {
	var envelope types.Envelope[templates.TemplateDetail]

	if err := c.do(http.MethodPut, "/v1/templates/"+url.PathEscape(id), jsonBody(request), &envelope); err != nil {
		return nil, err
	}

	return &envelope.Data, nil
}

// SetTemplateVisibility makes one of the caller's templates public or private.
// It reads the template and writes it back with only the flag changed, which
// is what the web page does too.
func (c *Client) SetTemplateVisibility(id string, public bool) (*templates.TemplateDetail, error) {
	current, err := c.GetTemplate(id)
	if err != nil {
		return nil, err
	}

	request := ToPublishRequest(current)
	request.IsPublic = public

	return c.UpdateTemplate(id, request)
}

// ToPublishRequest turns a template's detail back into the body that would
// publish it: the files as they are, the variables in their declared order and
// the commands by reference.
func ToPublishRequest(detail *templates.TemplateDetail) templates.PublishRequest {
	request := templates.PublishRequest{
		Name:        detail.Name,
		Slug:        detail.Slug,
		Description: detail.Description,
		Kind:        detail.Kind,
		IsPublic:    detail.IsPublic,
		Files:       make([]templates.PublishFile, 0, len(detail.Files)),
		Variables:   make([]templates.PublishVariable, 0, len(detail.Variables)),
		Commands:    make([]templates.PublishCommand, 0, len(detail.Commands)),
	}

	for _, file := range detail.Files {
		request.Files = append(request.Files, templates.PublishFile{
			Path:          file.Path,
			Kind:          file.Kind,
			Content:       file.Content,
			ContentBase64: file.ContentBase64,
			IsExecutable:  file.IsExecutable,
		})
	}

	variables := append([]templates.TemplateVariableData(nil), detail.Variables...)
	sort.SliceStable(variables, func(i, j int) bool { return variables[i].Position < variables[j].Position })

	for _, variable := range variables {
		request.Variables = append(request.Variables, templates.PublishVariable{
			Key:          variable.Key,
			Prompt:       variable.Prompt,
			DefaultValue: variable.DefaultValue,
			Required:     variable.Required,
		})
	}

	for _, command := range detail.Commands {
		request.Commands = append(request.Commands, templates.PublishCommand{
			CommandID:  command.CommandID,
			TargetPath: command.TargetPath,
		})
	}

	return request
}
