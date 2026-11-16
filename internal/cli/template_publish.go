package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/internal/templateapply"
	"github.com/fiwon123/eznit/internal/templatefolder"
	"github.com/fiwon123/eznit/pkg/types"
)

// The limits and the folder walk live in internal/templatefolder, shared with
// the desktop window. The names below keep the command and its tests on the
// names they already use.
const (
	maxTemplateFileBytes        = templatefolder.MaxFileBytes
	maxTemplateTotalBytes       = templatefolder.MaxTextTotalBytes
	maxTemplateBinaryBytes      = templatefolder.MaxBinaryBytes
	maxTemplateBinaryTotalBytes = templatefolder.MaxTotalBytes
	maxTemplateFiles            = templatefolder.MaxFiles
	maxTemplateVariableName     = templatefolder.MaxVariableName
)

// collectTemplateFiles is templatefolder.Collect, and isBinary its test.
var (
	collectTemplateFiles = templatefolder.Collect
	isBinary             = templatefolder.IsBinary
)

// Template Publish Cmd publishes a folder as a template.
type TemplatePublishCmd struct {
	Folder   string   `arg:"" optional:"" help:"the folder to publish" default:"."`
	Public   bool     `help:"publish to the shared catalogue"`
	Commands []string `help:"pull a command into the template as <id>=<path> (repeatable)" optional:""`
}

// Run publishes the folder as a template.
func (cmd *TemplatePublishCmd) Run(g *Globals) error {
	token, err := getToken()
	if err != nil {
		g.logger.Warn("not logged in. ")
		return nil
	}

	root := cmd.Folder
	if root == "" {
		root = "."
	}

	info, err := os.Stat(root)
	if err != nil {
		g.logger.Warn("failed to read the folder. ", slog.String("error", err.Error()))
		return nil
	}

	if !info.IsDir() {
		g.logger.Warn("the path is not a folder. ", slog.String("path", root))
		return nil
	}

	files, skipped, err := collectTemplateFiles(root)
	if err != nil {
		g.logger.Warn("failed to read the folder. ", slog.String("error", err.Error()))
		return nil
	}

	if len(files) == 0 {
		g.logger.Warn("the folder holds no file to publish. ", slog.String("path", root))
		return nil
	}

	for _, name := range skipped {
		fmt.Printf("skipped %s (a symlink, or bigger than a template allows)\n", name)
	}

	fmt.Println()

	name := promptLine("name: ")
	if name == "" {
		g.logger.Warn("a template needs a name. ")
		return nil
	}

	fmt.Printf("kind (%s): ", strings.Join(templates.Kinds, "/"))
	kind := promptLine("")
	if kind == "" {
		kind = templates.KindGeneric
	}

	if !templates.IsKind(kind) {
		g.logger.Warn("that is not a terminal eznit knows. ", slog.String("kind", kind))
		return nil
	}

	description := promptLine("description: ")

	isPublic := cmd.Public
	if !isPublic {
		answer := promptLine("publish to the shared catalogue? (y/N) ")
		isPublic = strings.EqualFold(strings.TrimSpace(answer), "y")
	}

	// The placeholders in the files are the variables the template asks
	// for: the author declares them by writing them.
	keys := templateapply.Undeclared(files, nil)

	variables := make([]templates.PublishVariable, 0, len(keys))

	for _, key := range keys {
		value := promptLine(fmt.Sprintf("default value for {{%s}}: ", key))

		variables = append(variables, templates.PublishVariable{
			Key:    key,
			Prompt: key,
			// A variable without a default has to be answered when the
			// template is applied.
			DefaultValue: value,
			Required:     value == "",
		})
	}

	request := templates.PublishRequest{
		Name:        name,
		Description: description,
		Kind:        kind,
		IsPublic:    isPublic,
	}

	for _, file := range files {
		request.Files = append(request.Files, templates.PublishFile{
			Path:          file.Path,
			Kind:          file.Kind,
			Content:       file.Content,
			ContentBase64: file.ContentBase64,
			IsExecutable:  file.IsExecutable,
		})
	}

	request.Variables = variables

	for _, spec := range cmd.Commands {
		commandID, targetPath, ok := parseCommandSpec(spec)
		if !ok {
			g.logger.Warn("that is not a command reference. ", slog.String("reference", spec))
			return nil
		}

		request.Commands = append(request.Commands, templates.PublishCommand{
			CommandID:  commandID,
			TargetPath: targetPath,
		})
	}

	detail, ok := sendPublishRequest(g.api.baseURL, request, token, g)
	if !ok {
		return nil
	}

	fmt.Println()
	fmt.Printf("published: %s (%s)\n", detail.Name, detail.ID)
	fmt.Printf("apply it with: eznit template apply %s\n", detail.ID)
	fmt.Println()

	return nil
}

// sendPublishRequest sends the template to the API.
func sendPublishRequest(baseURL string, request templates.PublishRequest, token string, g *Globals) (*templates.TemplateDetail, bool) {
	jsonData, err := json.Marshal(request)
	if err != nil {
		g.logger.Error("failed to convert request to json. ", slog.Any("error", err))
		return nil, false
	}

	url := baseURL + "/v1/templates"

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		g.logger.Warn("failed to create the request. ", slog.String("error", err.Error()))
		return nil, false
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		g.logger.Warn("failed to send request. ", slog.String("error", err.Error()))
		return nil, false
	}
	defer resp.Body.Close()

	var envelope types.Envelope[templates.TemplateDetail]

	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		g.logger.Error("failed to decode. ", slog.String("error", err.Error()))
		return nil, false
	}

	if resp.StatusCode != http.StatusOK {
		g.logger.Warn("the template could not be published. ", slog.Any("result", envelope))
		return nil, false
	}

	return &envelope.Data, true
}

// stdinReader reads the answers to the prompts. One reader for the whole
// run: a second reader on the same stdin loses whatever the first one has
// buffered.
var stdinReader *bufio.Reader

// promptReader returns the reader the prompts read from.
func promptReader() *bufio.Reader {
	if stdinReader == nil {
		stdinReader = bufio.NewReader(os.Stdin)
	}

	return stdinReader
}

// promptLine asks a question and returns the answer.
func promptLine(label string) string {
	fmt.Print(label)

	answer, err := promptReader().ReadString('\n')
	if err != nil && answer == "" {
		return ""
	}

	return strings.TrimSpace(answer)
}
