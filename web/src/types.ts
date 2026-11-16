// The JSON shapes the API speaks. They mirror internal/domain/templates/dto.go,
// internal/domain/users/dto.go, internal/domain/files/dto.go and
// internal/domain/commands/dto.go: the field names below are the wire
// contract, so a rename on the Go side has to happen here too.

export interface TemplateData {
  id: string;
  name: string;
  slug: string;
  description: string;
  kind: string;
  version: number;
  is_public: boolean;
  // The owner's user id. The catalogue never names the account; the caller's
  // own templates are told apart by `/templates/mine`, not by this field.
  owner: string;
  owner_email: string;
  file_count: number;
  created_at: string;
  updated_at: string;
}

// A file of a template. A text file carries content; a binary file carries
// its bytes base64-encoded, so the whole template round-trips in one request.
export interface TemplateFileData {
  id: string;
  path: string;
  kind?: string;
  size?: number;
  content?: string;
  content_base64?: string;
  is_executable: boolean;
}

export interface TemplateVariableData {
  id: string;
  key: string;
  prompt: string;
  default_value: string;
  required: boolean;
  position: number;
}

// A command the template pulls in from its owner's library.
export interface TemplateCommandData {
  command_id: string;
  target_path: string;
  name: string;
  kind: string;
  size: number;
  content?: string;
  content_base64?: string;
  is_executable: boolean;
}

export interface TemplateDetail extends TemplateData {
  files: TemplateFileData[];
  variables: TemplateVariableData[];
  commands?: TemplateCommandData[];
}

// The body of a publish (POST) or an update (PUT): the same shape, because an
// update replaces the whole template.
export interface PublishFile {
  path: string;
  kind: "text" | "binary";
  content?: string;
  content_base64?: string;
  is_executable: boolean;
}

export interface PublishVariable {
  key: string;
  prompt: string;
  default_value: string;
  required: boolean;
}

export interface PublishCommand {
  command_id: string;
  target_path: string;
}

export interface PublishRequest {
  name: string;
  slug: string;
  description: string;
  kind: string;
  is_public: boolean;
  files: PublishFile[];
  variables: PublishVariable[];
  commands: PublishCommand[];
}

// One entry of the files catalogue. It mirrors internal/domain/files/dto.go:
// the files resource sends the id, the name, the extension and the version —
// the path and the content type stay on the server.
export interface FileData {
  id: string;
  name: string;
  ext: string;
  version: number;
  is_public: boolean;
}

export interface SharedFileData extends FileData {
  owner: string;
  owner_email: string;
}

// One command of the caller's library.
export interface CommandData {
  id: string;
  name: string;
  kind: "text" | "binary";
  size: number;
  content?: string;
  content_base64?: string;
  is_executable: boolean;
  created_at: string;
  updated_at: string;
}

export interface CommandRequest {
  name: string;
  kind: "text" | "binary";
  content?: string;
  content_base64?: string;
  is_executable: boolean;
}

export interface ListResponse<T = TemplateData> {
  data: T[];
  total: number;
}

export interface LoginResponse {
  token: string;
}

// The envelope every API response wears: pkg/types.Envelope.
export interface Envelope<T> {
  status_code?: number;
  data?: T;
  message?: string;
  error?: string;
  timestamp?: string;
}

// The kinds a template may carry. The API enforces the same list with a
// CHECK constraint, so a kind outside this list never reaches the catalogue.
export const KINDS = [
  "opencode",
  "codex",
  "copilot",
  "cursor",
  "windsurf",
  "generic",
  "bash",
  "zsh",
  "fish",
  "powershell",
] as const;

export type Kind = (typeof KINDS)[number];

// What each kind is for, in the words a picker shows.
export const KIND_LABELS: Record<Kind, string> = {
  opencode: "opencode agent",
  codex: "Codex agent",
  copilot: "GitHub Copilot",
  cursor: "Cursor rules",
  windsurf: "Windsurf rules",
  generic: "Generic project",
  bash: "bash shell",
  zsh: "zsh shell",
  fish: "fish shell",
  powershell: "PowerShell",
};
