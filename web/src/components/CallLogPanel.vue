<template>
  <div class="floating-panel">
    <div v-if="minimized" class="collapsed-card">
      <div class="collapsed-info">
        <n-tag :type="receiving ? (connected ? 'success' : 'warning') : 'default'" :bordered="false" size="small">
          {{ receiving ? (connected ? "接收中" : "连接中") : "已暂停" }}
        </n-tag>
        <span class="collapsed-title">实时调用日志</span>
        <span class="collapsed-count">{{ logs.length }}</span>
      </div>
      <div class="collapsed-actions">
        <n-button size="tiny" quaternary @click="toggleReceiving">
          {{ receiving ? "停止" : "继续" }}
        </n-button>
        <n-button size="tiny" type="primary" @click="minimized = false">展开</n-button>
      </div>
    </div>

    <div
      v-else
      ref="panelRef"
      class="card"
      :style="{ width: `${panelWidth}px` }"
    >
      <div class="resize-handle" title="拖拽调整宽度" @mousedown="startResize" />
      <div class="section-head">
        <div>
          <h2>实时调用日志</h2>
          <p>仅保留最近 2000 条，后端不保存历史。</p>
        </div>
        <div class="section-actions">
          <n-tag :type="receiving ? (connected ? 'success' : 'warning') : 'default'" :bordered="false">
            {{ receiving ? (connected ? "接收中" : "连接中") : "已暂停" }}
          </n-tag>
          <span class="count-text">共 {{ logs.length }} 条</span>
        </div>
      </div>

      <div class="toolbar">
        <n-button size="small" secondary @click="toggleReceiving">
          {{ receiving ? "停止接收" : "继续接收" }}
        </n-button>
        <n-button size="small" quaternary @click="minimized = true">最小化</n-button>
        <n-button size="small" quaternary @click="clearLogs" :disabled="logs.length === 0">清空</n-button>
      </div>

      <n-alert class="hint" type="info" :bordered="false">
        默认自动连接并显示实时调用日志；点击“停止接收”后，后端新产生的日志会直接丢弃。
      </n-alert>

      <n-data-table
        v-if="logs.length > 0"
        :columns="columns"
        :data="logs"
        :bordered="false"
        :single-line="false"
        size="small"
        :max-height="420"
        :scroll-x="1360"
      />

      <n-empty v-else class="empty" description="当前还没有收到调用记录">
        <template #default>
          <div class="empty-title">等待实时调用日志</div>
          <div class="empty-desc">当有 Agent 调用 MCP 路由时，最新输入输出会实时出现在这里。</div>
        </template>
      </n-empty>
    </div>

    <n-modal
      :show="detailVisible"
      preset="card"
      title="调用记录详情"
      style="width: min(960px, calc(100vw - 32px))"
      :mask-closable="false"
      @update:show="handleDetailVisible"
    >
      <div v-if="selectedLog" class="detail-wrap">
        <div class="detail-grid">
          <div class="detail-item">
            <div class="detail-label">时间</div>
            <div class="detail-value">{{ selectedLog.time || "-" }}</div>
          </div>
          <div class="detail-item">
            <div class="detail-label">路由</div>
            <div class="detail-value">{{ selectedLog.routeName || "-" }} · {{ selectedLog.routePath || "-" }}</div>
          </div>
          <div class="detail-item">
            <div class="detail-label">请求</div>
            <div class="detail-value">{{ selectedLog.httpMethod || "-" }} / {{ selectedLog.rpcMethod || "-" }}</div>
          </div>
          <div class="detail-item">
            <div class="detail-label">状态</div>
            <div class="detail-value">{{ selectedLog.status || "-" }} · {{ selectedLog.durationMs || 0 }} ms</div>
          </div>
        </div>

        <div class="detail-block">
          <div class="detail-label">输入</div>
          <n-input
            :value="formatBodyText(selectedLog.requestBody)"
            type="textarea"
            readonly
            :autosize="{ minRows: 10, maxRows: 18 }"
          />
        </div>

        <div class="detail-block">
          <div class="detail-label">输出</div>
          <n-input
            :value="formatBodyText(selectedLog.responseBody)"
            type="textarea"
            readonly
            :autosize="{ minRows: 10, maxRows: 18 }"
          />
        </div>
      </div>

      <template #footer>
        <div class="footer">
          <n-button @click="handleDetailVisible(false)">关闭</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<script setup>
import { computed, h, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { NAlert, NButton, NDataTable, NEmpty, NInput, NModal, NTag } from "naive-ui";

const panelWidthStorageKey = "mcp-bridge-call-log-panel-width";
const panelMinimizedStorageKey = "mcp-bridge-call-log-panel-minimized";
const defaultPanelWidth = 980;
const minPreferredPanelWidth = 680;
const desktopPanelLeftInset = 20;
const desktopPanelRightInset = 20;
const minPanelWidth = 320;

const logs = ref([]);
const connected = ref(false);
const receiving = ref(true);
const minimized = ref(true);
const detailVisible = ref(false);
const selectedLog = ref(null);
const panelRef = ref(null);
const preferredPanelWidth = ref(defaultPanelWidth);
const viewportWidth = ref(typeof window !== "undefined" ? window.innerWidth : 1280);

let source;
let resizing = false;
let resizeFixedRight = 0;

function closeStream() {
  if (source) {
    source.close();
    source = null;
  }
  connected.value = false;
}

function clearLogs() {
  logs.value = [];
}

function currentPanelRightEdge() {
  if (panelRef.value?.getBoundingClientRect) {
    return panelRef.value.getBoundingClientRect().right;
  }
  if (typeof window !== "undefined") {
    return window.innerWidth - desktopPanelRightInset;
  }
  return viewportWidth.value - desktopPanelRightInset;
}

function clampPanelWidth(value, maxWidth = currentPanelRightEdge()) {
  const allowedMaxWidth = Math.max(minPanelWidth, maxWidth - desktopPanelLeftInset);
  const minWidth = Math.min(minPreferredPanelWidth, allowedMaxWidth);
  return Math.min(Math.max(value, minWidth), allowedMaxWidth);
}

function readStoredPanelWidth() {
  if (typeof window === "undefined") {
    return defaultPanelWidth;
  }
  const value = Number(window.localStorage.getItem(panelWidthStorageKey));
  if (!Number.isFinite(value)) {
    return defaultPanelWidth;
  }
  return Math.max(value, minPreferredPanelWidth);
}

function readStoredMinimized() {
  if (typeof window === "undefined") {
    return true;
  }
  const value = window.localStorage.getItem(panelMinimizedStorageKey);
  if (value === "false") {
    return false;
  }
  return true;
}

function syncViewportWidth() {
  if (typeof window !== "undefined") {
    viewportWidth.value = window.innerWidth;
  }
}

function toggleReceiving() {
  receiving.value = !receiving.value;
  if (receiving.value) {
    connectStream();
    return;
  }
  closeStream();
}

function formatBodyText(value) {
  const text = String(value || "").trim();
  return text || "-";
}

function previewBodyText(value) {
  const text = formatBodyText(value).replace(/\s+/g, " ");
  if (text.length <= 100) {
    return text;
  }
  return `${text.slice(0, 100)}...`;
}

function openDetail(row) {
  selectedLog.value = row;
  detailVisible.value = true;
}

function handleDetailVisible(value) {
  detailVisible.value = value;
  if (!value) {
    selectedLog.value = null;
  }
}

function pushLog(entry) {
  logs.value = [entry, ...logs.value].slice(0, 2000);
}

function connectStream() {
  if (!receiving.value) {
    return;
  }
  closeStream();

  source = new EventSource("/_admin/api/logs/stream");
  source.onopen = () => {
    connected.value = true;
  };
  source.onmessage = event => {
    try {
      pushLog(JSON.parse(event.data));
    } catch {
      // Ignore malformed log frames so the stream can continue.
    }
  };
  source.onerror = () => {
    connected.value = false;
  };
}

function handleResizeMove(event) {
  if (!resizing) {
    return;
  }
  const fallbackRight = typeof window !== "undefined" ? window.innerWidth - desktopPanelRightInset : 0;
  const rect = panelRef.value?.getBoundingClientRect?.();
  const rightEdge = resizeFixedRight || rect?.right || fallbackRight;
  const measuredWidth = rightEdge - event.clientX;
  const nextPanelWidth = clampPanelWidth(measuredWidth, rightEdge);
  preferredPanelWidth.value = Math.max(nextPanelWidth, minPreferredPanelWidth);
}

function stopResize() {
  resizing = false;
  resizeFixedRight = 0;
  window.removeEventListener("mousemove", handleResizeMove);
  window.removeEventListener("mouseup", stopResize);
}

function startResize(event) {
  if (typeof window === "undefined") {
    return;
  }
  event.preventDefault();
  const rect = panelRef.value?.getBoundingClientRect?.();
  resizing = true;
  resizeFixedRight = rect?.right ?? window.innerWidth - desktopPanelRightInset;
  window.addEventListener("mousemove", handleResizeMove);
  window.addEventListener("mouseup", stopResize);
}

const panelWidth = computed(() => clampPanelWidth(preferredPanelWidth.value, currentPanelRightEdge()));

const columns = computed(() => [
  {
    title: "时间",
    key: "time",
    width: 170,
    fixed: "left"
  },
  {
    title: "路由",
    key: "route",
    minWidth: 160,
    render(row) {
      return h("div", { class: "route-meta" }, [
        h("div", { class: "route-name" }, row.routeName || "-"),
        h("div", { class: "route-path" }, row.routePath || "-")
      ]);
    }
  },
  {
    title: "请求",
    key: "request",
    width: 130,
    render(row) {
      return h("div", { class: "request-meta" }, [
        h(
          NTag,
          { bordered: false, type: "info", size: "small" },
          { default: () => row.httpMethod || "-" }
        ),
        row.rpcMethod
          ? h(
              NTag,
              { bordered: false, size: "small" },
              { default: () => row.rpcMethod }
            )
          : null
      ]);
    }
  },
  {
    title: "状态",
    key: "status",
    width: 110,
    render(row) {
      const type = row.status >= 500 ? "error" : row.status >= 400 ? "warning" : "success";
      return h("div", { class: "status-meta" }, [
        h(
          NTag,
          { bordered: false, type, size: "small" },
          { default: () => String(row.status || "-") }
        ),
        h("span", { class: "duration-text" }, `${row.durationMs || 0} ms`)
      ]);
    }
  },
  {
    title: "输入",
    key: "requestBody",
    width: 220,
    render(row) {
      return h("div", { class: "body-preview" }, previewBodyText(row.requestBody));
    }
  },
  {
    title: "输出",
    key: "responseBody",
    width: 220,
    render(row) {
      return h("div", { class: "body-preview" }, previewBodyText(row.responseBody));
    }
  },
  {
    title: "操作",
    key: "actions",
    width: 88,
    fixed: "right",
    render(row) {
      return h(
        NButton,
        {
          size: "small",
          quaternary: true,
          strong: true,
          onClick: () => openDetail(row)
        },
        { default: () => "详情" }
      );
    }
  }
]);

onMounted(() => {
  preferredPanelWidth.value = readStoredPanelWidth();
  minimized.value = readStoredMinimized();
  syncViewportWidth();
  window.addEventListener("resize", syncViewportWidth);
  connectStream();
});

onBeforeUnmount(() => {
  stopResize();
  closeStream();
  if (typeof window !== "undefined") {
    window.removeEventListener("resize", syncViewportWidth);
  }
});

watch(
  preferredPanelWidth,
  value => {
    if (typeof window !== "undefined") {
      window.localStorage.setItem(panelWidthStorageKey, String(Math.round(value)));
    }
  }
);

watch(
  minimized,
  value => {
    if (typeof window !== "undefined") {
      window.localStorage.setItem(panelMinimizedStorageKey, value ? "true" : "false");
    }
  }
);
</script>

<style scoped>
.card {
  position: relative;
  box-sizing: border-box;
  background: var(--panel-bg);
  border: 1px solid var(--border-color);
  border-radius: 16px;
  padding: 16px;
  box-shadow: 0 10px 30px var(--shadow-color);
  max-width: calc(100vw - 20px);
}

.collapsed-card {
  display: flex;
  box-sizing: border-box;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  min-width: 280px;
  max-width: min(420px, calc(100vw - 32px));
  background: var(--panel-bg);
  border: 1px solid var(--border-color);
  border-radius: 14px;
  box-shadow: 0 10px 30px var(--shadow-color);
}

.floating-panel {
  position: fixed;
  right: 20px;
  bottom: 20px;
  z-index: 40;
}

.resize-handle {
  position: absolute;
  top: 0;
  left: 0;
  width: 12px;
  height: 100%;
  cursor: ew-resize;
}

.resize-handle::before {
  content: "";
  position: absolute;
  top: 50%;
  left: 4px;
  width: 4px;
  height: 48px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--border-color) 70%, var(--text-color) 30%);
  transform: translateY(-50%);
  opacity: 0.7;
}

.collapsed-info,
.collapsed-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.collapsed-info {
  min-width: 0;
  flex: 1;
}

.collapsed-title {
  font-size: 13px;
  font-weight: 600;
  white-space: nowrap;
}

.collapsed-count {
  color: var(--muted-text);
  font-size: 12px;
}

.section-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  margin-bottom: 16px;
}

.section-head h2 {
  margin: 0 0 8px;
}

.section-head p {
  margin: 0;
  color: var(--muted-text);
  line-height: 1.7;
}

.section-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}

.count-text {
  color: var(--muted-text);
  font-size: 13px;
}

.hint {
  margin-bottom: 12px;
}

:deep(.n-data-table) {
  background: transparent;
}

:deep(.n-data-table-th) {
  background: color-mix(in srgb, var(--panel-bg) 88%, var(--text-color) 12%);
}

.route-meta,
.status-meta,
.request-meta {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.route-name {
  font-weight: 600;
}

.route-path,
.duration-text {
  color: var(--muted-text);
  font-size: 12px;
}

.body-text {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 12px;
  line-height: 1.6;
}

.body-preview {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 12px;
  line-height: 1.6;
  padding: 10px 12px;
  border-radius: 10px;
  background: color-mix(in srgb, var(--panel-bg) 76%, var(--text-color) 24%);
  min-height: 68px;
}

.detail-wrap {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.detail-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.detail-item,
.detail-block {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.detail-label {
  color: var(--muted-text);
  font-size: 13px;
}

.detail-value {
  line-height: 1.6;
  word-break: break-word;
}

.footer {
  display: flex;
  justify-content: flex-end;
}

.empty {
  padding: 24px 0;
}

.empty-title {
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 8px;
}

.empty-desc {
  color: var(--muted-text);
  line-height: 1.6;
}

@media (max-width: 900px) {
  .floating-panel {
    right: 16px;
    bottom: 16px;
    left: 16px;
  }

  .collapsed-card {
    min-width: 0;
    width: 100%;
    flex-wrap: wrap;
  }

  .card {
    width: 100% !important;
  }

  .resize-handle {
    display: none;
  }

  .section-head {
    flex-direction: column;
  }

  .detail-grid {
    grid-template-columns: 1fr;
  }
}
</style>
