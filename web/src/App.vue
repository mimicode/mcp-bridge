<template>
  <div class="page">
    <div class="hero card">
      <div>
        <h1>MCP Bridge 管理台</h1>
        <p>支持 MCP 列表检索、启用状态筛选、快捷复制路由与未保存变更提示，保存后仅重载 MCP 配置。</p>
      </div>
      <div class="hero-actions">
        <n-button secondary @click="reloadConfig" :loading="loading">重新读取</n-button>
        <n-button type="primary" @click="openCreateModal">添加 MCP</n-button>
        <n-button type="success" @click="saveConfig" :loading="saving" :disabled="!isDirty && !saving">保存并热更新</n-button>
      </div>
    </div>

    <n-alert
      v-if="isDirty"
      class="dirty-alert"
      type="warning"
      :bordered="false"
      title="存在未保存的配置变更"
    >
      当前列表内容已变化但尚未写入 `config.json`，点击“保存并热更新”后才会应用到后端路由。
    </n-alert>

    <n-alert
      v-if="showNoEnabledAlert"
      class="empty-alert"
      type="info"
      :bordered="false"
      title="当前没有启用的 MCP"
    >
      服务仍然可以正常运行，管理页也可继续编辑配置；只是当前不会暴露任何可用 MCP 路由，访问相关入口会提示不可用。
    </n-alert>

    <div class="summary-grid">
      <div class="card summary">
        <div class="summary-title">配置文件</div>
        <div class="summary-value">{{ configPath || "-" }}</div>
      </div>
      <div class="card summary">
        <div class="summary-title">MCP 总数</div>
        <div class="summary-value">{{ servers.length }}</div>
      </div>
      <div class="card summary">
        <div class="summary-title">启用 MCP</div>
        <div class="summary-value">{{ enabledCount }}</div>
      </div>
      <div class="card summary">
        <div class="summary-title">已就绪路由</div>
        <div class="summary-value">{{ readyCount }}/{{ routes.length }}</div>
      </div>
      <div class="card summary">
        <div class="summary-title">状态</div>
        <div class="summary-status" :class="statusType || 'neutral'">{{ statusMessage }}</div>
      </div>
      <div class="card summary">
        <div class="summary-title">构建版本</div>
        <div class="summary-value">{{ versionInfo.version || "-" }}</div>
        <div class="summary-meta">{{ versionInfo.commit }} · {{ versionInfo.buildTime }}</div>
      </div>
    </div>

    <div class="content-grid">
      <div class="card">
        <div class="section-head">
          <div>
            <h2>已添加 MCP</h2>
            <p>通过表格统一管理所有 MCP，支持搜索、筛选、复制和编辑。</p>
          </div>
          <div class="table-toolbar">
            <n-input
              v-model:value="keyword"
              clearable
              placeholder="搜索名称、命令、路由、描述"
            />
            <n-select
              v-model:value="statusFilter"
              :options="statusOptions"
              style="width: 160px"
            />
            <n-button quaternary @click="clearFilters">清空筛选</n-button>
          </div>
        </div>

        <n-data-table
          :columns="serverColumns"
          :data="filteredRows"
          :bordered="false"
          :single-line="false"
          size="small"
        />
      </div>

      <div class="card">
        <div class="section-head">
          <div>
            <h2>路由状态</h2>
            <p>展示当前后端热更新后的路由连接状态，并可快捷复制 HTTP 入口。</p>
          </div>
        </div>
        <n-data-table
          :columns="routeColumns"
          :data="routes"
          :bordered="false"
          :single-line="false"
          size="small"
        />
      </div>
    </div>

    <mcp-editor-modal
      v-model:show="editorVisible"
      :server="editingServer"
      @save="handleSaveServer"
    />
  </div>
</template>

<script setup>
import { computed, h, ref } from "vue";
import {
  NAlert,
  NButton,
  NDataTable,
  NInput,
  NSelect,
  NTag,
  useDialog,
  useMessage
} from "naive-ui";
import McpEditorModal from "@/components/McpEditorModal.vue";

const message = useMessage();
const dialog = useDialog();

const loading = ref(false);
const saving = ref(false);
const editorVisible = ref(false);
const editingIndex = ref(-1);
const editingServer = ref(null);
const configPath = ref("");
const routes = ref([]);
const servers = ref([]);
const statusMessage = ref("正在加载配置...");
const statusType = ref("");
const loadedSnapshot = ref("");
const snapshotReady = ref(false);
const keyword = ref("");
const statusFilter = ref("all");
const testingKey = ref("");
const versionInfo = ref({
  version: "",
  commit: "",
  buildTime: ""
});

const statusOptions = [
  { label: "全部状态", value: "all" },
  { label: "仅启用", value: "enabled" },
  { label: "仅禁用", value: "disabled" }
];

const readyCount = computed(() => routes.value.filter(item => item.ready).length);
const enabledCount = computed(() => servers.value.filter(item => item.enabled).length);
const currentSnapshot = computed(() => stringifyConfig(configFromServers(servers.value)));
const isDirty = computed(() => snapshotReady.value && currentSnapshot.value !== loadedSnapshot.value);
const showNoEnabledAlert = computed(() => snapshotReady.value && enabledCount.value === 0);

const filteredRows = computed(() => {
  const term = keyword.value.trim().toLowerCase();
  return servers.value
    .map((server, index) => ({
      ...server,
      rowKey: `${server.name || "unnamed"}-${index}`,
      index
    }))
    .filter(row => {
      if (statusFilter.value === "enabled" && !row.enabled) {
        return false;
      }
      if (statusFilter.value === "disabled" && row.enabled) {
        return false;
      }
      if (!term) {
        return true;
      }
      const haystack = [
        row.name,
        row.command,
        row.path || autoPathPreview(row.name),
        row.description
      ]
        .join(" ")
        .toLowerCase();
      return haystack.includes(term);
    });
});

const serverColumns = [
  {
    title: "名称",
    key: "name",
    minWidth: 180,
    render(row) {
      return row.name || "-";
    }
  },
  {
    title: "启用",
    key: "enabled",
    width: 90,
    render(row) {
      return h(
        NTag,
        { type: row.enabled ? "success" : "warning", bordered: false },
        { default: () => (row.enabled ? "是" : "否") }
      );
    }
  },
  {
    title: "命令",
    key: "command",
    minWidth: 140,
    ellipsis: {
      tooltip: true
    }
  },
  {
    title: "路由",
    key: "path",
    minWidth: 160,
    render(row) {
      return row.path || autoPathPreview(row.name);
    }
  },
  {
    title: "参数",
    key: "args",
    width: 90,
    render(row) {
      return h(
        NTag,
        { bordered: false, type: "info" },
        { default: () => `${Array.isArray(row.args) ? row.args.length : 0} 项` }
      );
    }
  },
  {
    title: "环境变量",
    key: "env",
    width: 100,
    render(row) {
      return h(
        NTag,
        { bordered: false, type: "default" },
        { default: () => `${row.env ? Object.keys(row.env).length : 0} 项` }
      );
    }
  },
  {
    title: "描述",
    key: "description",
    minWidth: 220,
    ellipsis: {
      tooltip: true
    },
    render(row) {
      return row.description || "-";
    }
  },
  {
    title: "操作",
    key: "actions",
    width: 320,
    render: row =>
      h("div", { class: "action-cell" }, [
        h(
          NButton,
          {
            size: "small",
            secondary: true,
            loading: testingKey.value === row.rowKey,
            onClick: () => testServer(row.index)
          },
          { default: () => "测试" }
        ),
        h(
          NButton,
          { size: "small", secondary: true, onClick: () => copyText(row.path || autoPathPreview(row.name), "路由已复制") },
          { default: () => "复制路由" }
        ),
        h(
          NButton,
          { size: "small", type: "primary", onClick: () => openEditModal(row.index) },
          { default: () => "编辑" }
        ),
        h(
          NButton,
          { size: "small", secondary: true, onClick: () => duplicateServer(row.index) },
          { default: () => "复制" }
        ),
        h(
          NButton,
          {
            size: "small",
            type: "error",
            ghost: true,
            onClick: () => removeServer(row.index)
          },
          { default: () => "删除" }
        )
      ])
  }
];

const routeColumns = [
  {
    title: "名称",
    key: "name",
    minWidth: 160
  },
  {
    title: "状态",
    key: "ready",
    width: 100,
    render(row) {
      return h(
        NTag,
        { type: row.ready ? "success" : "error", bordered: false },
        { default: () => (row.ready ? "已就绪" : "未就绪") }
      );
    }
  },
  {
    title: "路径",
    key: "path",
    minWidth: 160
  },
  {
    title: "后端",
    key: "backendName",
    minWidth: 180,
    render(row) {
      const name = row.backendName || "-";
      const version = row.backendVersion || "";
      return `${name} ${version}`.trim();
    }
  },
  {
    title: "错误",
    key: "lastError",
    minWidth: 200,
    ellipsis: {
      tooltip: true
    },
    render(row) {
      return row.lastError || "-";
    }
  },
  {
    title: "操作",
    key: "routeActions",
    width: 140,
    render(row) {
      return h(
        NButton,
        {
          size: "small",
          secondary: true,
          onClick: () => copyRouteEndpoint(row.path)
        },
        { default: () => "复制入口" }
      );
    }
  }
];

function setStatus(messageText, type = "") {
  statusMessage.value = messageText;
  statusType.value = type;
}

function autoPathPreview(name) {
  const slug = String(name || "")
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "") || "server";
  return `/mcp/${slug}`;
}

function stringifyConfig(config) {
  return `${JSON.stringify(config, null, 2)}\n`;
}

function configFromServers(list) {
  const mcpServers = {};
  for (const server of list) {
    const name = String(server.name || "").trim();
    if (!name) {
      continue;
    }

    const item = {
      enabled: server.enabled !== false,
      command: String(server.command || "").trim(),
      args: Array.isArray(server.args) ? server.args.filter(Boolean) : [],
      description: server.description || "",
      timeout: Number.isFinite(server.timeout) ? server.timeout : 60000
    };

    if (server.path) {
      item.path = String(server.path).trim();
    }
    if (server.env && Object.keys(server.env).length > 0) {
      item.env = server.env;
    }

    mcpServers[name] = item;
  }

  return { mcpServers };
}

async function reloadConfig() {
  loading.value = true;
  snapshotReady.value = false;
  setStatus("正在加载配置...");
  try {
    const response = await fetch("/_admin/api/config");
    const data = await response.json();
    if (!response.ok) {
      throw new Error(data.error || "读取配置失败");
    }

    configPath.value = data.configPath || "";
    routes.value = data.routes || [];
    servers.value = parseConfigContent(data.content || "");
    versionInfo.value = data.version || versionInfo.value;
    loadedSnapshot.value = stringifyConfig(configFromServers(servers.value));
    snapshotReady.value = true;
    setStatus("配置已加载", "ok");
  } catch (error) {
    setStatus(error.message || "读取配置失败", "error");
    message.error(error.message || "读取配置失败");
  } finally {
    loading.value = false;
  }
}

function parseConfigContent(content) {
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

function openCreateModal() {
  editingIndex.value = -1;
  editingServer.value = null;
  editorVisible.value = true;
}

function openEditModal(index) {
  editingIndex.value = index;
  editingServer.value = JSON.parse(JSON.stringify(servers.value[index]));
  editorVisible.value = true;
}

function duplicateServer(index) {
  const source = JSON.parse(JSON.stringify(servers.value[index]));
  source.name = source.name ? `${source.name}-copy` : "";
  source.path = "";
  servers.value.splice(index + 1, 0, source);
  message.success("已复制 MCP");
}

function handleSaveServer(server) {
  if (!server.name) {
    message.error("名称不能为空");
    return;
  }

  const duplicateIndex = servers.value.findIndex(
    (item, index) => item.name === server.name && index !== editingIndex.value
  );
  if (duplicateIndex >= 0) {
    message.error("MCP 名称不能重复");
    return;
  }

  if (editingIndex.value >= 0) {
    servers.value.splice(editingIndex.value, 1, server);
    message.success("已更新 MCP");
  } else {
    servers.value.push(server);
    message.success("已添加 MCP");
  }
}

function removeServer(index) {
  const current = servers.value[index];
  dialog.warning({
    title: "删除 MCP",
    content: `确认删除 ${current?.name || "该 MCP"} 吗？`,
    positiveText: "删除",
    negativeText: "取消",
    onPositiveClick: () => {
      servers.value.splice(index, 1);
      message.success("已删除 MCP");
    }
  });
}

function clearFilters() {
  keyword.value = "";
  statusFilter.value = "all";
}

async function copyText(text, successMessage) {
  try {
    await navigator.clipboard.writeText(text);
    message.success(successMessage);
  } catch (error) {
    message.error("复制失败，请检查浏览器权限");
  }
}

function copyRouteEndpoint(path) {
  const origin = window.location.origin || "";
  copyText(`${origin}${path}`, "HTTP 入口已复制");
}

async function testServer(index) {
  const server = servers.value[index];
  const rowKey = `${server.name || "unnamed"}-${index}`;
  testingKey.value = rowKey;
  try {
    const response = await fetch("/_admin/api/test", {
      method: "POST",
      headers: {
        "Content-Type": "application/json"
      },
      body: JSON.stringify(server)
    });
    const data = await response.json();
    versionInfo.value = data.version || versionInfo.value;
    if (!response.ok) {
      throw new Error(data.error || "测试失败");
    }

    const backendName = data.info?.backendName || server.name;
    const backendVersion = data.info?.backendVersion || "";
    message.success(`测试成功: ${backendName} ${backendVersion}`.trim());
  } catch (error) {
    message.error(error.message || "测试失败");
  } finally {
    testingKey.value = "";
  }
}

async function saveConfig() {
  saving.value = true;
  setStatus("正在保存并热更新...");
  try {
    const nextContent = stringifyConfig(configFromServers(servers.value));
    const response = await fetch("/_admin/api/config", {
      method: "POST",
      headers: {
        "Content-Type": "application/json"
      },
      body: JSON.stringify({
        content: nextContent
      })
    });

    const data = await response.json();
    if (!response.ok) {
      throw new Error(data.error || "保存失败");
    }

    configPath.value = data.configPath || configPath.value;
    routes.value = data.routes || [];
    servers.value = parseConfigContent(data.content || nextContent);
    versionInfo.value = data.version || versionInfo.value;
    loadedSnapshot.value = stringifyConfig(configFromServers(servers.value));
    snapshotReady.value = true;
    setStatus(data.warning || data.message || "保存成功", data.reloaded ? "ok" : "warn");
    message.success(data.message || "配置已保存");
  } catch (error) {
    setStatus(error.message || "保存失败", "error");
    message.error(error.message || "保存失败");
  } finally {
    saving.value = false;
  }
}

reloadConfig();
</script>

<style scoped>
.page {
  min-height: 100vh;
  padding: 24px;
  background: #0b1220;
  color: #e5e7eb;
}

.card {
  background: #111827;
  border: 1px solid #243041;
  border-radius: 16px;
  padding: 20px;
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.18);
}

.hero {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  margin-bottom: 16px;
}

.hero h1,
.section-head h2 {
  margin: 0 0 8px;
}

.hero p,
.section-head p {
  margin: 0;
  color: #94a3b8;
  line-height: 1.7;
}

.hero-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.dirty-alert {
  margin-bottom: 16px;
}

.empty-alert {
  margin-bottom: 16px;
}

.summary-grid,
.content-grid {
  display: grid;
  gap: 16px;
}

.summary-grid {
  grid-template-columns: repeat(5, minmax(0, 1fr));
  margin-bottom: 16px;
}

.summary {
  min-height: 112px;
}

.summary-title {
  color: #94a3b8;
  font-size: 13px;
  margin-bottom: 10px;
}

.summary-value {
  font-size: 18px;
  line-height: 1.5;
  word-break: break-word;
}

.summary-meta {
  margin-top: 8px;
  color: #94a3b8;
  font-size: 12px;
  line-height: 1.5;
  word-break: break-word;
}

.summary-status {
  font-size: 14px;
  line-height: 1.6;
}

.summary-status.ok {
  color: #34d399;
}

.summary-status.warn {
  color: #fbbf24;
}

.summary-status.error {
  color: #f87171;
}

.summary-status.neutral {
  color: #cbd5e1;
}

.content-grid {
  grid-template-columns: minmax(0, 1.6fr) minmax(0, 1fr);
}

.section-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  margin-bottom: 16px;
}

.table-toolbar {
  display: flex;
  gap: 12px;
  align-items: center;
  width: min(540px, 100%);
}

:deep(.n-data-table) {
  background: transparent;
}

:deep(.n-data-table-th) {
  background: #0f172a;
}

.action-cell {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

@media (max-width: 1200px) {
  .summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .content-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 900px) {
  .section-head {
    flex-direction: column;
  }

  .table-toolbar {
    width: 100%;
    flex-wrap: wrap;
  }
}

@media (max-width: 768px) {
  .page {
    padding: 16px;
  }

  .hero {
    flex-direction: column;
  }

  .hero-actions {
    width: 100%;
    justify-content: flex-start;
  }

  .summary-grid {
    grid-template-columns: 1fr;
  }
}
</style>
