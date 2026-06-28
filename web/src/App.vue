<template>
  <div class="page" :class="`theme-${resolvedTheme}`">
    <div class="hero card">
      <div class="hero-content">
        <div>
          <h1>MCP Bridge 管理台</h1>
          <p>支持 MCP 列表检索、启用状态筛选、路由状态查看、实时调用日志和 Agent 配置预览复制。</p>
        </div>
        <div class="hero-controls">
          <div class="theme-control">
            <span class="theme-label">主题</span>
            <n-select
              :value="themePreference"
              :options="themeOptions"
              size="small"
              style="width: 160px"
              @update:value="$emit('update:themePreference', $event)"
            />
          </div>
        </div>
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

    <mcp-server-table
      v-model:keyword="keyword"
      v-model:status-filter="statusFilter"
      :servers="servers"
      :routes="routes"
      :loading="loading"
      :saving="saving"
      :is-dirty="isDirty"
      :testing-key="testingKey"
      @reload="reloadConfig"
      @save="saveConfig"
      @open-create="openCreateModal"
      @edit="openEditModal"
      @test="testServer"
      @duplicate-create="openDuplicateCreateModal"
      @remove="removeServer"
      @copy-endpoint="handleCopyEndpoint"
      @preview-agent-config="openAgentConfigPreview"
      @preview-all-agent-config="openAllAgentConfigPreview"
      @clear-filters="clearFilters"
    />

    <call-log-panel />

    <mcp-editor-modal
      v-model:show="editorVisible"
      :server="editingServer"
      @save="handleSaveServer"
    />

    <agent-config-preview-modal
      v-model:show="previewVisible"
      :title="previewTitle"
      :content="previewContent"
      @copy="copyPreviewContent"
    />
  </div>
</template>

<script setup>
import { computed, ref } from "vue";
import { NAlert, NSelect, useDialog, useMessage } from "naive-ui";
import AgentConfigPreviewModal from "@/components/AgentConfigPreviewModal.vue";
import CallLogPanel from "@/components/CallLogPanel.vue";
import McpEditorModal from "@/components/McpEditorModal.vue";
import McpServerTable from "@/components/McpServerTable.vue";
import {
  buildAgentConfigPayload,
  configFromServers,
  parseConfigContent,
  resolveServerPath,
  stringifyConfig
} from "@/lib/admin";

defineProps({
  themePreference: {
    type: String,
    default: "light"
  },
  resolvedTheme: {
    type: String,
    default: "light"
  }
});

defineEmits(["update:themePreference"]);

const message = useMessage();
const dialog = useDialog();

const loading = ref(false);
const saving = ref(false);
const editorVisible = ref(false);
const previewVisible = ref(false);
const previewTitle = ref("");
const previewContent = ref("");
const previewSuccessMessage = ref("配置已复制");
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
const themeOptions = [
  { label: "亮色", value: "light" },
  { label: "暗黑", value: "dark" },
  { label: "跟随系统", value: "system" }
];

const readyCount = computed(() => routes.value.filter(item => item.ready).length);
const enabledCount = computed(() => servers.value.filter(item => item.enabled).length);
const currentSnapshot = computed(() => stringifyConfig(configFromServers(servers.value)));
const isDirty = computed(() => snapshotReady.value && currentSnapshot.value !== loadedSnapshot.value);
const showNoEnabledAlert = computed(() => snapshotReady.value && enabledCount.value === 0);

function setStatus(messageText, type = "") {
  statusMessage.value = messageText;
  statusType.value = type;
}

function buildPreviewContent(list) {
  const origin = window.location.origin || "";
  return stringifyConfig(buildAgentConfigPayload(list, origin));
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

function openDuplicateCreateModal(index) {
  const source = JSON.parse(JSON.stringify(servers.value[index]));
  source.name = source.name ? `${source.name}-copy` : "";
  source.path = "";
  editingIndex.value = -1;
  editingServer.value = source;
  editorVisible.value = true;
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
  } catch {
    message.error("复制失败，请检查浏览器权限");
  }
}

function handleCopyEndpoint(row) {
  const origin = window.location.origin || "";
  copyText(`${origin}${row.resolvedPath || resolveServerPath(row)}`, "HTTP 入口已复制");
}

function openAgentConfigPreview(row) {
  previewTitle.value = `预览 ${row.name || "当前 MCP"} 的 Agent 配置`;
  previewContent.value = buildPreviewContent([row]);
  previewSuccessMessage.value = "Agent 配置已复制";
  previewVisible.value = true;
}

function openAllAgentConfigPreview() {
  previewTitle.value = "预览全部启用 MCP 的 Agent 配置";
  previewContent.value = buildPreviewContent(servers.value);
  previewSuccessMessage.value = "全部 Agent 配置已复制";
  previewVisible.value = true;
}

async function copyPreviewContent() {
  await copyText(previewContent.value, previewSuccessMessage.value);
  previewVisible.value = false;
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
  background: var(--page-bg);
  color: var(--text-color);
  transition: background-color 0.2s ease, color 0.2s ease;
}

.page.theme-light {
  --page-bg: #f3f6fb;
  --panel-bg: #ffffff;
  --border-color: #dbe4f0;
  --text-color: #0f172a;
  --muted-text: #64748b;
  --shadow-color: rgba(15, 23, 42, 0.08);
  --status-ok: #059669;
  --status-warn: #d97706;
  --status-error: #dc2626;
  --status-neutral: #64748b;
}

.page.theme-dark {
  --page-bg: #0b1220;
  --panel-bg: #111827;
  --border-color: #243041;
  --text-color: #e5e7eb;
  --muted-text: #94a3b8;
  --shadow-color: rgba(0, 0, 0, 0.18);
  --status-ok: #34d399;
  --status-warn: #fbbf24;
  --status-error: #f87171;
  --status-neutral: #cbd5e1;
}

.card {
  background: var(--panel-bg);
  border: 1px solid var(--border-color);
  border-radius: 16px;
  padding: 20px;
  box-shadow: 0 10px 30px var(--shadow-color);
}

.hero {
  display: flex;
  margin-bottom: 16px;
}

.hero-content {
  width: 100%;
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
}

.hero h1 {
  margin: 0 0 8px;
}

.hero p {
  margin: 0;
  color: var(--muted-text);
  line-height: 1.7;
}

.hero-controls {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.theme-control {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.theme-label {
  color: var(--muted-text);
  font-size: 13px;
}

.dirty-alert,
.empty-alert {
  margin-bottom: 16px;
}

.summary-grid {
  display: grid;
  gap: 16px;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  margin-bottom: 16px;
}

.summary {
  min-height: 112px;
}

.summary-title {
  color: var(--muted-text);
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
  color: var(--muted-text);
  font-size: 12px;
  line-height: 1.5;
  word-break: break-word;
}

.summary-status {
  font-size: 14px;
  line-height: 1.6;
}

.summary-status.ok {
  color: var(--status-ok);
}

.summary-status.warn {
  color: var(--status-warn);
}

.summary-status.error {
  color: var(--status-error);
}

.summary-status.neutral {
  color: var(--status-neutral);
}

@media (max-width: 1200px) {
  .summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 768px) {
  .page {
    padding: 16px;
  }

  .hero {
    display: block;
  }

  .hero-content {
    flex-direction: column;
  }

  .hero-controls {
    width: 100%;
    justify-content: flex-start;
  }

  .summary-grid {
    grid-template-columns: 1fr;
  }
}
</style>
