<template>
  <div class="card">
    <div class="section-head">
      <div>
        <h2>已添加 MCP</h2>
        <p>统一展示 MCP、路由状态、后端信息和 Agent 配置复制能力。</p>
      </div>
      <div class="section-actions">
        <n-button secondary @click="$emit('reload')" :loading="loading">重新读取</n-button>
        <n-button secondary @click="$emit('preview-all-agent-config')" :disabled="enabledCount === 0">
          复制全部 Agent 配置
        </n-button>
        <n-button type="primary" @click="$emit('open-create')">添加 MCP</n-button>
        <n-button type="success" @click="$emit('save')" :loading="saving" :disabled="!isDirty && !saving">
          保存并热更新
        </n-button>
      </div>
    </div>

    <div class="table-toolbar">
      <div class="table-toolbar-filters">
        <n-input
          :value="keyword"
          clearable
          placeholder="搜索名称、命令、路由、描述"
          @update:value="$emit('update:keyword', $event)"
        />
        <n-select
          :value="statusFilter"
          :options="statusOptions"
          style="width: 160px"
          @update:value="$emit('update:statusFilter', $event)"
        />
        <n-button quaternary @click="$emit('clear-filters')">清空筛选</n-button>
      </div>
    </div>

    <n-data-table
      v-if="filteredRows.length > 0"
      :columns="serverColumns"
      :data="filteredRows"
      :bordered="false"
      :single-line="false"
      size="small"
    />

    <n-empty v-else class="table-empty" :description="emptyStateDescription">
      <template #default>
        <div class="empty-title">{{ emptyStateTitle }}</div>
        <div class="empty-desc">{{ emptyStateDescription }}</div>
      </template>
      <template #extra>
        <div class="empty-actions">
          <n-button v-if="servers.length === 0 || enabledCount === 0" type="primary" @click="$emit('open-create')">
            添加 MCP
          </n-button>
          <n-button v-if="servers.length > 0 && filteredRows.length === 0" secondary @click="$emit('clear-filters')">
            清空筛选
          </n-button>
          <n-button v-if="servers.length > 0 && enabledCount > 0" secondary @click="$emit('preview-all-agent-config')">
            复制全部 Agent 配置
          </n-button>
          <n-button v-if="servers.length > 0 && enabledCount === 0" type="success" @click="$emit('save')" :loading="saving">
            保存并热更新
          </n-button>
        </div>
      </template>
    </n-empty>
  </div>
</template>

<script setup>
import { computed, h } from "vue";
import { NButton, NDataTable, NDropdown, NEmpty, NInput, NSelect, NTag } from "naive-ui";
import { resolveServerPath } from "@/lib/admin";

const props = defineProps({
  servers: {
    type: Array,
    required: true
  },
  routes: {
    type: Array,
    required: true
  },
  loading: {
    type: Boolean,
    default: false
  },
  saving: {
    type: Boolean,
    default: false
  },
  isDirty: {
    type: Boolean,
    default: false
  },
  testingKey: {
    type: String,
    default: ""
  },
  keyword: {
    type: String,
    default: ""
  },
  statusFilter: {
    type: String,
    default: "all"
  }
});

const emit = defineEmits([
  "update:keyword",
  "update:statusFilter",
  "reload",
  "save",
  "open-create",
  "edit",
  "test",
  "duplicate-create",
  "remove",
  "copy-endpoint",
  "preview-agent-config",
  "preview-all-agent-config",
  "clear-filters"
]);

const statusOptions = [
  { label: "全部状态", value: "all" },
  { label: "仅启用", value: "enabled" },
  { label: "仅禁用", value: "disabled" }
];

const routeInfoByPath = computed(() => {
  const map = new Map();
  for (const route of props.routes) {
    map.set(route.path, route);
  }
  return map;
});

const enabledCount = computed(() => props.servers.filter(item => item.enabled).length);

const filteredRows = computed(() => {
  const term = props.keyword.trim().toLowerCase();
  return props.servers
    .map((server, index) => {
      const resolvedPath = resolveServerPath(server);
      return {
        ...server,
        resolvedPath,
        routeInfo: routeInfoByPath.value.get(resolvedPath),
        rowKey: `${server.name || "unnamed"}-${index}`,
        index
      };
    })
    .filter(row => {
      if (props.statusFilter === "enabled" && !row.enabled) {
        return false;
      }
      if (props.statusFilter === "disabled" && row.enabled) {
        return false;
      }
      if (!term) {
        return true;
      }
      const haystack = [
        row.name,
        row.command,
        row.resolvedPath,
        row.description,
        row.routeInfo?.backendName,
        row.routeInfo?.lastError
      ]
        .join(" ")
        .toLowerCase();
      return haystack.includes(term);
    });
});

const emptyStateTitle = computed(() => {
  if (props.servers.length === 0) {
    return "还没有配置任何 MCP";
  }
  if (enabledCount.value === 0) {
    return "当前没有启用的 MCP";
  }
  return "没有匹配的 MCP";
});

const emptyStateDescription = computed(() => {
  if (props.servers.length === 0) {
    return "先添加一个 MCP 配置，再保存并热更新。";
  }
  if (enabledCount.value === 0) {
    return "当前所有 MCP 都处于禁用状态，保存后不会暴露任何 MCP 路由。";
  }
  return "当前筛选条件下没有匹配项，可以清空筛选后再查看。";
});

function buildMoreOptions(row) {
  return [
    { label: "复制入口", key: "copy-endpoint", disabled: !row.enabled },
    { label: "复制新增", key: "duplicate-create" },
    { label: "删除", key: "delete" }
  ];
}

function handleMoreAction(key, row) {
  switch (key) {
    case "copy-endpoint":
      return emit("copy-endpoint", row);
    case "duplicate-create":
      return emit("duplicate-create", row.index);
    case "delete":
      return emit("remove", row.index);
    default:
      return undefined;
  }
}

function selectTextFromEvent(event) {
  const selection = window.getSelection();
  if (!selection) {
    return;
  }
  selection.removeAllRanges();
  const range = document.createRange();
  range.selectNodeContents(event.currentTarget);
  selection.addRange(range);
}

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
    ellipsis: { tooltip: true }
  },
  {
    title: "路由",
    key: "path",
    minWidth: 220,
    render(row) {
      return h("div", { class: "route-cell" }, [
        h(
          "span",
          {
            class: ["route-value", !row.enabled && "route-value-disabled"],
            title: "点击选中路由",
            onClick: event => selectTextFromEvent(event)
          },
          row.resolvedPath
        ),
        h(
          NButton,
          {
            size: "tiny",
            quaternary: true,
            strong: true,
            title: row.enabled ? "复制完整入口" : "已禁用，仍可复制路由",
            onClick: () => emit("copy-endpoint", row)
          },
          { default: () => "复制" }
        )
      ]);
    }
  },
  {
    title: "状态",
    key: "routeStatus",
    width: 110,
    render(row) {
      if (!row.enabled) {
        return h(NTag, { type: "warning", bordered: false }, { default: () => "已禁用" });
      }
      if (row.routeInfo?.ready) {
        return h(NTag, { type: "success", bordered: false }, { default: () => "已就绪" });
      }
      return h(
        NTag,
        { type: row.routeInfo?.lastError ? "error" : "default", bordered: false },
        { default: () => (row.routeInfo?.lastError ? "异常" : "未加载") }
      );
    }
  },
  {
    title: "后端",
    key: "backend",
    minWidth: 180,
    render(row) {
      const name = row.routeInfo?.backendName || "-";
      const version = row.routeInfo?.backendVersion || "";
      return `${name} ${version}`.trim();
    }
  },
  {
    title: "参数",
    key: "args",
    width: 90,
    render(row) {
      return h(NTag, { bordered: false, type: "info" }, { default: () => `${Array.isArray(row.args) ? row.args.length : 0} 项` });
    }
  },
  {
    title: "环境变量",
    key: "env",
    width: 100,
    render(row) {
      return h(NTag, { bordered: false, type: "default" }, { default: () => `${row.env ? Object.keys(row.env).length : 0} 项` });
    }
  },
  {
    title: "错误",
    key: "lastError",
    minWidth: 220,
    ellipsis: { tooltip: true },
    render(row) {
      if (!row.enabled) {
        return "-";
      }
      return row.routeInfo?.lastError || "-";
    }
  },
  {
    title: "操作",
    key: "actions",
    width: 360,
    render: row =>
      h("div", { class: "action-cell" }, [
        h("div", { class: "action-main" }, [
          h(
            NButton,
            {
              size: "small",
              type: "primary",
              onClick: () => emit("edit", row.index)
            },
            { default: () => "编辑" }
          ),
          h(
            NButton,
            {
              size: "small",
              secondary: true,
              strong: true,
              loading: props.testingKey === row.rowKey,
              onClick: () => emit("test", row.index)
            },
            { default: () => "测试" }
          ),
          h(
            NButton,
            {
              size: "small",
              quaternary: true,
              strong: true,
              onClick: () => emit("preview-agent-config", row)
            },
            { default: () => "预览配置" }
          )
        ]),
        h(
          NDropdown,
          {
            trigger: "click",
            options: buildMoreOptions(row),
            onSelect: key => handleMoreAction(key, row)
          },
          {
            default: () =>
              h(
                NButton,
                {
                  size: "small",
                  quaternary: true,
                  strong: true
                },
                { default: () => "更多" }
              )
          }
        )
      ])
  }
];
</script>

<style scoped>
.card {
  background: var(--panel-bg);
  border: 1px solid var(--border-color);
  border-radius: 16px;
  padding: 20px;
  box-shadow: 0 10px 30px var(--shadow-color);
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

.section-actions,
.table-toolbar {
  display: flex;
  gap: 12px;
  align-items: center;
  flex-wrap: wrap;
}

.table-toolbar {
  margin-bottom: 16px;
}

.table-toolbar-filters {
  display: flex;
  gap: 12px;
  align-items: center;
  width: min(540px, 100%);
  flex-wrap: wrap;
}

:deep(.n-data-table) {
  background: transparent;
}

:deep(.n-data-table-th) {
  background: color-mix(in srgb, var(--panel-bg) 88%, var(--text-color) 12%);
}

.action-cell {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: nowrap;
}

.action-main {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.route-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.route-value {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  word-break: break-all;
  cursor: text;
  user-select: text;
  padding: 2px 8px;
  border-radius: 8px;
  background: color-mix(in srgb, var(--panel-bg) 76%, var(--text-color) 24%);
}

.route-value-disabled {
  color: var(--muted-text);
  opacity: 0.75;
}

.table-empty {
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

.empty-actions {
  display: flex;
  gap: 12px;
  justify-content: center;
  flex-wrap: wrap;
}

@media (max-width: 900px) {
  .section-head {
    flex-direction: column;
  }

  .section-actions,
  .table-toolbar,
  .table-toolbar-filters {
    width: 100%;
  }

  .action-cell {
    flex-wrap: wrap;
    justify-content: flex-start;
  }
}
</style>
