export function autoPathPreview(name) {
  const slug =
    String(name || "")
      .trim()
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-+|-+$/g, "") || "server";
  return `/mcp/${slug}`;
}

export function resolveServerPath(server) {
  return server?.path || autoPathPreview(server?.name);
}

export function stringifyConfig(config) {
  return `${JSON.stringify(config, null, 2)}\n`;
}

export function configFromServers(list) {
  const mcpServers = {};
  for (const server of list || []) {
    const name = String(server?.name || "").trim();
    if (!name) {
      continue;
    }

    const item = {
      enabled: server?.enabled !== false,
      command: String(server?.command || "").trim(),
      args: Array.isArray(server?.args) ? server.args.filter(Boolean) : [],
      description: server?.description || "",
      timeout: Number.isFinite(server?.timeout) ? server.timeout : 60000
    };

    if (server?.path) {
      item.path = String(server.path).trim();
    }
    if (server?.env && Object.keys(server.env).length > 0) {
      item.env = server.env;
    }

    mcpServers[name] = item;
  }

  return { mcpServers };
}

export function parseConfigContent(content) {
  const parsed = JSON.parse(content || "{}");
  const source = parsed.mcpServers || {};
  return Object.entries(source).map(([name, value]) => ({
    name,
    enabled: value.enabled !== false,
    command: value.command || "",
    path: value.path || "",
    description: value.description || "",
    timeout: typeof value.timeout === "number" ? value.timeout : 60000,
    args: Array.isArray(value.args) ? value.args : [],
    env: value.env && typeof value.env === "object" ? value.env : {}
  }));
}

export function buildAgentConfigPayload(list, origin) {
  const mcpServers = {};
  for (const server of list || []) {
    const name = String(server?.name || "").trim();
    if (!name || server?.enabled === false) {
      continue;
    }
    mcpServers[name] = {
      transport: "streamable-http",
      url: `${origin}${resolveServerPath(server)}`
    };
  }
  return { mcpServers };
}
