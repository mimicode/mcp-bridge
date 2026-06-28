<template>
  <n-modal
    :show="show"
    preset="card"
    :title="title"
    style="width: min(960px, 92vw)"
    :mask-closable="false"
    @update:show="handleShowUpdate"
  >
    <div class="toolbar">
      <n-radio-group v-model:value="mode" name="editor-mode">
        <n-radio-button value="form">表单添加</n-radio-button>
        <n-radio-button value="json">JSON 添加</n-radio-button>
      </n-radio-group>
      <n-button quaternary @click="resetState">重置</n-button>
    </div>

    <div v-if="mode === 'form'" class="form-wrap">
      <n-form label-placement="top">
        <div class="form-grid">
          <n-form-item label="名称">
            <n-input v-model:value="form.name" placeholder="例如 mcp-server-fetch" />
          </n-form-item>

          <n-form-item label="命令">
            <n-input v-model:value="form.command" placeholder="例如 npx / uvx / cmd" />
          </n-form-item>

          <n-form-item label="路由">
            <n-input v-model:value="form.path" placeholder="留空自动生成，例如 /mcp/fetch" />
          </n-form-item>

          <n-form-item label="超时(ms)">
            <n-input-number v-model:value="form.timeout" :min="0" style="width: 100%" />
          </n-form-item>

          <n-form-item class="span-2" label="描述">
            <n-input v-model:value="form.description" placeholder="说明这个 MCP 的用途" />
          </n-form-item>

          <n-form-item class="span-2" label="参数列表">
            <n-dynamic-input
              v-model:value="form.args"
              :on-create="createArgItem"
              #="{ value }"
            >
              <n-input v-model:value="value.value" placeholder="输入一个参数" />
            </n-dynamic-input>
          </n-form-item>

          <n-form-item class="span-2" label="环境变量">
            <n-dynamic-input
              v-model:value="form.env"
              :on-create="createEnvItem"
              #="{ value }"
            >
              <div class="env-row">
                <n-input v-model:value="value.key" placeholder="KEY" />
                <n-input v-model:value="value.value" placeholder="VALUE" />
              </div>
            </n-dynamic-input>
          </n-form-item>

          <n-form-item label="启用">
            <n-switch v-model:value="form.enabled" />
          </n-form-item>
        </div>
      </n-form>
    </div>

    <div v-else class="json-wrap">
      <div class="json-toolbar">
        <n-text depth="3">支持粘贴单个 MCP 条目，或包含 `mcpServers` 的完整 JSON。</n-text>
        <n-button quaternary @click="formatJson">格式化</n-button>
      </div>
      <n-input
        v-model:value="jsonText"
        type="textarea"
        :autosize="{ minRows: 18, maxRows: 28 }"
        placeholder="粘贴 JSON"
      />
    </div>

    <template #footer>
      <div class="footer">
        <n-button @click="handleShowUpdate(false)">取消</n-button>
        <n-button type="primary" @click="submit">确定</n-button>
      </div>
    </template>
  </n-modal>
</template>

<script setup>
import { computed, reactive, ref, watch } from "vue";
import {
  NButton,
  NDynamicInput,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NModal,
  NRadioButton,
  NRadioGroup,
  NSwitch,
  NText
} from "naive-ui";

const props = defineProps({
  show: {
    type: Boolean,
    required: true
  },
  server: {
    type: Object,
    default: null
  }
});

const emit = defineEmits(["update:show", "save"]);

const mode = ref("form");
const jsonText = ref("");
const form = reactive(createEmptyServer());

const title = computed(() => (props.server ? "编辑 MCP" : "新增 MCP"));

watch(
  () => [props.show, props.server],
  () => {
    if (!props.show) {
      return;
    }
    resetState();
  },
  { immediate: true }
);

function createEmptyServer() {
  return {
    name: "",
    enabled: true,
    command: "",
    path: "",
    description: "",
    timeout: 60000,
    args: [],
    env: []
  };
}

function createArgItem() {
  return { value: "" };
}

function createEnvItem() {
  return { key: "", value: "" };
}

function normalizeServer(server) {
  const source = server || createEmptyServer();
  return {
    name: source.name || "",
    enabled: source.enabled !== false,
    command: source.command || "",
    path: source.path || "",
    description: source.description || "",
    timeout: Number.isFinite(source.timeout) ? source.timeout : 60000,
    args: Array.isArray(source.args) ? source.args.map(item => ({ value: item })) : [],
    env: source.env
      ? Object.entries(source.env).map(([key, value]) => ({ key, value: String(value ?? "") }))
      : []
  };
}

function fillForm(server) {
  const normalized = normalizeServer(server);
  Object.assign(form, normalized);
}

function serverToObject(server) {
  const env = {};
  for (const item of server.env || []) {
    const key = String(item.key || "").trim();
    if (!key) {
      continue;
    }
    env[key] = item.value ?? "";
  }

  return {
    name: String(server.name || "").trim(),
    enabled: !!server.enabled,
    command: String(server.command || "").trim(),
    path: String(server.path || "").trim(),
    description: server.description || "",
    timeout: Number.isFinite(server.timeout) ? server.timeout : 60000,
    args: (server.args || []).map(item => String(item.value || "").trim()).filter(Boolean),
    env
  };
}

function resetState() {
  mode.value = "form";
  fillForm(props.server);
  jsonText.value = JSON.stringify(serverToObject(form), null, 2);
}

function formatJson() {
  const parsed = parseJsonInput(jsonText.value);
  jsonText.value = JSON.stringify(parsed, null, 2);
}

function parseJsonInput(input) {
  const parsed = JSON.parse(input || "{}");
  if (parsed && typeof parsed === "object" && parsed.mcpServers && typeof parsed.mcpServers === "object") {
    const entries = Object.entries(parsed.mcpServers);
    if (entries.length === 0) {
      throw new Error("mcpServers 不能为空");
    }
    const [name, value] = entries[0];
    return {
      name,
      ...(value || {})
    };
  }

  if (parsed && typeof parsed === "object" && parsed.name && parsed.command) {
    return parsed;
  }

  if (parsed && typeof parsed === "object") {
    const entries = Object.entries(parsed);
    if (entries.length === 1 && typeof entries[0][1] === "object") {
      return {
        name: entries[0][0],
        ...(entries[0][1] || {})
      };
    }
  }

  throw new Error("JSON 格式不正确，请提供单个 MCP 条目");
}

function submit() {
  if (mode.value === "json") {
    const parsed = parseJsonInput(jsonText.value);
    emit("save", normalizeForEmit(parsed));
    emit("update:show", false);
    return;
  }

  emit("save", normalizeForEmit(serverToObject(form)));
  emit("update:show", false);
}

function normalizeForEmit(server) {
  return {
    name: String(server.name || "").trim(),
    enabled: server.enabled !== false,
    command: String(server.command || "").trim(),
    path: String(server.path || "").trim(),
    description: server.description || "",
    timeout: Number.isFinite(server.timeout) ? server.timeout : 60000,
    args: Array.isArray(server.args) ? server.args.filter(Boolean) : [],
    env: server.env && typeof server.env === "object" ? server.env : {}
  };
}

function handleShowUpdate(value) {
  emit("update:show", value);
}
</script>

<style scoped>
.toolbar,
.json-toolbar,
.footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.toolbar {
  margin-bottom: 16px;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.span-2 {
  grid-column: span 2;
}

.env-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  width: 100%;
}

.json-toolbar {
  margin-bottom: 12px;
}

.footer {
  justify-content: flex-end;
}

@media (max-width: 768px) {
  .form-grid {
    grid-template-columns: 1fr;
  }

  .span-2 {
    grid-column: span 1;
  }

  .env-row {
    grid-template-columns: 1fr;
  }
}
</style>
