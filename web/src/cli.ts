// The CLI finds its server through two variables, CLI_API_HOST (scheme and
// host) and CLI_API_PORT, and joins them as host + ":" + port
// (internal/cli/main.go). These helpers write the lines that point a terminal
// at the server this page talks to.

export type Shell = "posix" | "fish" | "powershell";

export interface CliTarget {
  host: string;
  port: string;
  // isDefault is true for http://localhost:4000, which the CLI uses when
  // neither variable is set: no setup line is needed then.
  isDefault: boolean;
}

export function cliTarget(address: string): CliTarget {
  try {
    const url = new URL(address);
    const port = url.port !== "" ? url.port : url.protocol === "https:" ? "443" : "80";
    const host = `${url.protocol}//${url.hostname}`;
    return { host, port, isDefault: host === "http://localhost" && port === "4000" };
  } catch {
    return { host: "http://localhost", port: "4000", isDefault: true };
  }
}

// The UI may be served on another port than the API: the product's nginx on
// :5173 and the Vite dev server both proxy /v1 to :4000, which is the address
// the CLI must use. A same-origin page on 5173 is pointed at 4000.
export function apiAddressFor(serverURL: string): string {
  try {
    const url = new URL(serverURL);
    if ((url.hostname === "localhost" || url.hostname === "127.0.0.1") && url.port === "5173") {
      url.port = "4000";
    }
    return `${url.protocol}//${url.host}${url.pathname.replace(/\/+$/, "")}`;
  } catch {
    return serverURL;
  }
}

export function envLines(target: CliTarget, shell: Shell): string {
  switch (shell) {
    case "fish":
      return `set -gx CLI_API_HOST ${target.host}\nset -gx CLI_API_PORT ${target.port}`;
    case "powershell":
      return `$env:CLI_API_HOST = "${target.host}"\n$env:CLI_API_PORT = "${target.port}"`;
    default:
      return `export CLI_API_HOST=${target.host}\nexport CLI_API_PORT=${target.port}`;
  }
}
