import type {
  CommandData,
  CommandRequest,
  Envelope,
  FileData,
  ListResponse,
  LoginResponse,
  PublishRequest,
  SharedFileData,
  TemplateData,
  TemplateDetail,
  TemplateFileData,
} from "./types";

// ── The server ──────────────────────────────────────────────────────────────
//
// The UI talks to one eznit server at a time. "" means the origin that served
// this page — the API serves its own UI, and the dev server and the product's
// nginx proxy /v1 to it — and any other value is the absolute address of a
// remote server, which must list this page's origin in CORS_ALLOWED_ORIGINS.

const SERVER_KEY = "eznit.server";

// The same-origin token keeps the key the README documents, so a token pasted
// from `eznit login` still works. A remote server's token lives under a key of
// its own: switching servers never sends one server's token to another.
const TOKEN_KEY = "eznit.token";
const EMAIL_KEY = "eznit.email";

// storage wraps localStorage: a private window or a blocked site can make
// every access throw, and the UI must still render (signed out) when it does.
function read(key: string): string | null {
  try {
    return window.localStorage.getItem(key);
  } catch {
    return null;
  }
}

function write(key: string, value: string | null): void {
  try {
    if (value === null) {
      window.localStorage.removeItem(key);
    } else {
      window.localStorage.setItem(key, value);
    }
  } catch {
    // Nothing to do: the session lasts as long as the page does.
  }
}

function scoped(key: string, server: string): string {
  return server === "" ? key : `${key}@${server}`;
}

export function getServer(): string {
  return read(SERVER_KEY) ?? "";
}

export function setServer(server: string): void {
  write(SERVER_KEY, server === "" ? null : server);
}

export function getToken(server: string = getServer()): string | null {
  return read(scoped(TOKEN_KEY, server));
}

export function getEmail(server: string = getServer()): string | null {
  return read(scoped(EMAIL_KEY, server));
}

export function saveSession(token: string, email: string, server: string = getServer()): void {
  write(scoped(TOKEN_KEY, server), token);
  write(scoped(EMAIL_KEY, server), email);
}

export function clearSession(server: string = getServer()): void {
  write(scoped(TOKEN_KEY, server), null);
  write(scoped(EMAIL_KEY, server), null);
}

// normalizeServer turns what a person typed into a server address: a scheme
// when there is none, no trailing slash, and no trailing /v1 (the address of
// the server, not of its API). The page's own origin collapses to "".
export function normalizeServer(raw: string): string {
  let value = raw.trim();
  if (value === "") {
    return "";
  }
  if (!/^[a-z][a-z0-9+.-]*:\/\//i.test(value)) {
    value = `http://${value}`;
  }

  let url: URL;
  try {
    url = new URL(value);
  } catch {
    throw new Error(`"${raw.trim()}" is not an address.`);
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new Error("The address must start with http:// or https://.");
  }

  const path = url.pathname.replace(/\/+$/, "").replace(/\/v1$/, "");
  const normalized = `${url.origin}${path}`;
  return normalized === window.location.origin ? "" : normalized;
}

// serverLabel is the address a person reads: the host, or this page's.
export function serverLabel(server: string = getServer()): string {
  const base = server === "" ? window.location.origin : server;
  try {
    return new URL(base).host;
  } catch {
    return base;
  }
}

// serverURL is the absolute address, for the CLI instructions.
export function serverURL(server: string = getServer()): string {
  return server === "" ? window.location.origin : server;
}

// ── Requests ────────────────────────────────────────────────────────────────

export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

// messageOf turns any thrown value into something a page can show.
export function messageOf(cause: unknown): string {
  if (cause instanceof Error) {
    return cause.message;
  }
  return String(cause);
}

// refused reports whether the API refused the token. It is the one failure a
// page cannot fix by saying so — the request arrived and was judged, so the
// session is over and the login page is where the reader goes (#339).
export function refused(cause: unknown): boolean {
  return cause instanceof ApiError && cause.status === 401;
}

// unreachable is what a failed fetch means. A browser reports a refused
// connection, a DNS failure and a CORS refusal the same way, so the sentence
// names all of them.
function unreachable(server: string): ApiError {
  const where = serverLabel(server);
  const hint =
    server === ""
      ? "Is the API running?"
      : "Check the address, and that the server lists this page's origin in CORS_ALLOWED_ORIGINS.";
  return new ApiError(0, `Could not reach ${where}. ${hint}`);
}

// send is fetch with the server's address in front, the session token on it,
// and a network failure turned into a sentence.
async function send(path: string, init: RequestInit = {}, server = getServer()): Promise<Response> {
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  const token = getToken(server);
  if (token !== null) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  try {
    return await fetch(`${server}/v1${path}`, { ...init, headers });
  } catch (cause: unknown) {
    if (cause instanceof DOMException && cause.name === "AbortError") {
      throw cause;
    }
    throw unreachable(server);
  }
}

// readEnvelope reads the envelope a response wears, or an empty one when the
// body is not JSON at all — a proxy or a crash can answer with something else,
// and a parse error must not hide the status code.
async function readEnvelope<T>(response: Response): Promise<Envelope<T>> {
  try {
    return (await response.json()) as Envelope<T>;
  } catch {
    return {};
  }
}

async function failure(response: Response): Promise<ApiError> {
  const envelope = await readEnvelope(response);
  const reason =
    envelope.error !== undefined && envelope.error !== ""
      ? capitalize(envelope.error)
      : `The request failed (${response.status}).`;
  return new ApiError(response.status, reason);
}

function capitalize(text: string): string {
  return text.charAt(0).toUpperCase() + text.slice(1);
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (typeof init.body === "string") {
    headers.set("Content-Type", "application/json");
  }

  const response = await send(path, { ...init, headers });
  if (!response.ok) {
    throw await failure(response);
  }
  const envelope = await readEnvelope<T>(response);
  return envelope.data as T;
}

function json(method: string, body: unknown): RequestInit {
  return { method, body: JSON.stringify(body) };
}

// ── Health ──────────────────────────────────────────────────────────────────

export interface Health {
  ok: boolean;
  ms: number;
  error?: string;
}

// checkHealth asks a server whether it is up. It never throws: a status pill
// and a "test connection" button both want an answer, not an exception.
export async function checkHealth(server: string = getServer()): Promise<Health> {
  const started = performance.now();
  try {
    const response = await fetch(`${server}/v1/healthcheck`, {
      headers: { Accept: "application/json" },
      signal: AbortSignal.timeout(5000),
    });
    const ms = Math.round(performance.now() - started);
    if (!response.ok) {
      return { ok: false, ms, error: `The server answered ${response.status}.` };
    }
    return { ok: true, ms };
  } catch {
    return {
      ok: false,
      ms: Math.round(performance.now() - started),
      error: unreachable(server).message,
    };
  }
}

// ── Accounts ────────────────────────────────────────────────────────────────

export function login(email: string, password: string): Promise<LoginResponse> {
  return request<LoginResponse>("/login", json("POST", { email, password }));
}

// signup creates the account. The API asks for the password twice — it refuses
// a mismatch itself — and takes no name: the users table has none.
export function signup(email: string, password: string, confirmPassword: string): Promise<void> {
  return request<void>(
    "/signup",
    json("POST", { email, password, confirm_password: confirmPassword }),
  );
}

// logout ends the session on the server too. A failure is not worth showing:
// the token is dropped locally either way.
export async function logout(): Promise<void> {
  try {
    await send("/logout");
  } catch {
    // ignored on purpose
  }
}

// ── Templates ───────────────────────────────────────────────────────────────

export function listTemplates(kind = ""): Promise<ListResponse<TemplateData>> {
  const query = kind === "" ? "" : `?kind=${encodeURIComponent(kind)}`;
  return request<ListResponse<TemplateData>>(`/templates${query}`);
}

export function listMyTemplates(): Promise<ListResponse<TemplateData>> {
  return request<ListResponse<TemplateData>>("/templates/mine");
}

export function getTemplate(id: string): Promise<TemplateDetail> {
  return request<TemplateDetail>(`/templates/${encodeURIComponent(id)}`);
}

export function publishTemplate(body: PublishRequest): Promise<TemplateDetail> {
  return request<TemplateDetail>("/templates", json("POST", body));
}

export function updateTemplate(id: string, body: PublishRequest): Promise<TemplateDetail> {
  return request<TemplateDetail>(`/templates/${encodeURIComponent(id)}`, json("PUT", body));
}

export function deleteTemplate(id: string): Promise<void> {
  return request<void>(`/templates/${encodeURIComponent(id)}`, { method: "DELETE" });
}

// toPublishRequest turns a template's detail back into the body that would
// publish it, so a change to one field (the visibility) can be sent as the
// whole-template replacement the API expects.
export function toPublishRequest(template: TemplateDetail): PublishRequest {
  return {
    name: template.name,
    slug: template.slug,
    description: template.description,
    kind: template.kind,
    is_public: template.is_public,
    files: template.files.map((file) =>
      file.kind === "binary"
        ? {
            path: file.path,
            kind: "binary",
            content_base64: file.content_base64 ?? "",
            is_executable: file.is_executable,
          }
        : {
            path: file.path,
            kind: "text",
            content: file.content ?? "",
            is_executable: file.is_executable,
          },
    ),
    variables: [...template.variables]
      .sort((a, b) => a.position - b.position)
      .map((variable) => ({
        key: variable.key,
        prompt: variable.prompt,
        default_value: variable.default_value,
        required: variable.required,
      })),
    commands: (template.commands ?? []).map((command) => ({
      command_id: command.command_id,
      target_path: command.target_path,
    })),
  };
}

// setTemplateVisibility flips one template between public and private. The
// API has no partial update, so it reads the template and writes it back.
export async function setTemplateVisibility(
  id: string,
  isPublic: boolean,
): Promise<TemplateDetail> {
  const current = await getTemplate(id);
  return updateTemplate(id, { ...toPublishRequest(current), is_public: isPublic });
}

// ── Downloads ───────────────────────────────────────────────────────────────

// saveResponse turns an answer into a download under the given name. It cannot
// be a plain link: the routes may sit behind the guard, so the request carries
// the Authorization header and the bytes arrive here.
async function saveResponse(response: Response, name: string): Promise<void> {
  if (!response.ok) {
    throw await failure(response);
  }
  saveBlob(await response.blob(), name);
}

export function saveBlob(blob: Blob, name: string): void {
  const url = window.URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = name;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();

  // The revoke waits a turn of the event loop: revoking in the same tick as the
  // click can cancel a download that has not started yet.
  window.setTimeout(() => window.URL.revokeObjectURL(url), 0);
}

// archiveNameOf is the name the zip is saved under: the template's slug when it
// has one, its name otherwise, with anything that is not safe in a filename
// collapsed to a dash.
export function archiveNameOf(template: Pick<TemplateData, "slug" | "name">): string {
  const source = template.slug !== "" ? template.slug : template.name;
  const safe = source.replace(/[^a-zA-Z0-9._-]+/g, "-").replace(/^-+|-+$/g, "");
  return safe === "" ? "template" : safe;
}

export async function downloadTemplateArchive(
  template: Pick<TemplateData, "id" | "slug" | "name">,
): Promise<void> {
  const response = await send(`/templates/${encodeURIComponent(template.id)}/archive`);
  await saveResponse(response, `${archiveNameOf(template)}.zip`);
}

export async function downloadTemplateFile(
  templateID: string,
  file: TemplateFileData,
): Promise<void> {
  const response = await send(
    `/templates/${encodeURIComponent(templateID)}/files/${encodeURIComponent(file.id)}/content`,
  );
  const parts = file.path.split("/");
  await saveResponse(response, parts[parts.length - 1] ?? file.path);
}

// ── Files ───────────────────────────────────────────────────────────────────

// The most the API takes in one request: it caps the body with
// http.MaxBytesReader at 32 MiB, so the browser refuses first and says why.
export const MAX_UPLOAD_BYTES = 32 * 1024 * 1024;

// fileNameOf is the name the file was uploaded under. The service splits
// "notes.md" into the name "notes" and the extension "md", so the dot goes
// back in.
//
// The extension is read without a leading dot: a file replaced before #358 was
// stored as ".txt", and the name must not come out as "notes..txt".
export function fileNameOf(file: Pick<FileData, "name" | "ext">): string {
  const ext = extOf(file);
  return ext === "" ? file.name : `${file.name}.${ext}`;
}

export function extOf(file: Pick<FileData, "ext">): string {
  return file.ext.replace(/^\.+/, "");
}

export function listFiles(): Promise<ListResponse<FileData>> {
  return request<ListResponse<FileData>>("/files/me");
}

export function listSharedFiles(): Promise<ListResponse<SharedFileData>> {
  return request<ListResponse<SharedFileData>>("/files/shared");
}

function multipart(file: File): FormData {
  const form = new FormData();
  form.append("file", file);
  return form;
}

export async function uploadFile(file: File): Promise<void> {
  // No Content-Type here: the browser sets it with the multipart boundary.
  const response = await send("/files", { method: "POST", body: multipart(file) });
  if (!response.ok) {
    throw await failure(response);
  }
}

// replaceFile stores new bytes under the same record; the version goes up.
export async function replaceFile(id: string, file: File): Promise<void> {
  const response = await send(`/files/${encodeURIComponent(id)}`, {
    method: "PUT",
    body: multipart(file),
  });
  if (!response.ok) {
    throw await failure(response);
  }
}

export async function shareFile(id: string, isPublic: boolean): Promise<void> {
  await request<void>(
    `/files/${encodeURIComponent(id)}/share`,
    json("POST", { is_public: isPublic }),
  );
}

export async function downloadFile(file: Pick<FileData, "id" | "name" | "ext">): Promise<void> {
  const response = await send(`/files/${encodeURIComponent(file.id)}/content`);
  await saveResponse(response, fileNameOf(file));
}

export function deleteFile(id: string): Promise<void> {
  return request<void>(`/files/${encodeURIComponent(id)}`, { method: "DELETE" });
}

// ── Commands ────────────────────────────────────────────────────────────────

export function listCommands(): Promise<ListResponse<CommandData>> {
  return request<ListResponse<CommandData>>("/commands");
}

export function createCommand(body: CommandRequest): Promise<CommandData> {
  return request<CommandData>("/commands", json("POST", body));
}

export function updateCommand(id: string, body: CommandRequest): Promise<CommandData> {
  return request<CommandData>(`/commands/${encodeURIComponent(id)}`, json("PUT", body));
}

export function deleteCommand(id: string): Promise<void> {
  return request<void>(`/commands/${encodeURIComponent(id)}`, { method: "DELETE" });
}

// ── Bytes ───────────────────────────────────────────────────────────────────

// toBase64 encodes bytes for a JSON body. It works in chunks: spreading a
// large array into String.fromCharCode overflows the call stack.
export function toBase64(bytes: Uint8Array): string {
  let binary = "";
  const chunk = 0x8000;
  for (let i = 0; i < bytes.length; i += chunk) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunk));
  }
  return btoa(binary);
}

export function base64Size(encoded: string): number {
  const padding = encoded.endsWith("==") ? 2 : encoded.endsWith("=") ? 1 : 0;
  return Math.max(0, Math.floor((encoded.length * 3) / 4) - padding);
}

// readLocalFile decides whether a file a person picked is text or binary: it is
// text when it decodes as UTF-8 and holds no NUL byte, which is how a template
// file is told apart on the CLI side too.
export async function readLocalFile(
  file: File,
): Promise<{ kind: "text"; content: string } | { kind: "binary"; base64: string; size: number }> {
  const bytes = new Uint8Array(await file.arrayBuffer());
  if (!bytes.includes(0)) {
    try {
      const content = new TextDecoder("utf-8", { fatal: true }).decode(bytes);
      return { kind: "text", content };
    } catch {
      // not UTF-8: binary below
    }
  }
  return { kind: "binary", base64: toBase64(bytes), size: bytes.length };
}

export function formatBytes(size: number): string {
  if (size < 1024) {
    return `${size} B`;
  }
  const units = ["KB", "MB", "GB"];
  let value = size / 1024;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value.toFixed(value < 10 ? 1 : 0)} ${units[unit]}`;
}

// relativeTime is "3 days ago" for a timestamp the API sent.
export function relativeTime(iso: string): string {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) {
    return "";
  }
  const seconds = Math.round((then - Date.now()) / 1000);
  const steps: [Intl.RelativeTimeFormatUnit, number][] = [
    ["year", 31536000],
    ["month", 2592000],
    ["week", 604800],
    ["day", 86400],
    ["hour", 3600],
    ["minute", 60],
  ];
  const format = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });
  for (const [unit, size] of steps) {
    if (Math.abs(seconds) >= size) {
      return format.format(Math.round(seconds / size), unit);
    }
  }
  return "just now";
}
