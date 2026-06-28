<template>
  <n-modal
    :show="show"
    preset="card"
    title="预览 Agent 配置"
    style="width: min(880px, 92vw)"
    :mask-closable="false"
    @update:show="handleShowUpdate"
  >
    <div class="meta">
      <div class="meta-title">{{ title }}</div>
      <div class="meta-desc">确认内容无误后再复制到 Agent 配置里。</div>
    </div>

    <n-input
      :value="content"
      type="textarea"
      readonly
      :autosize="{ minRows: 14, maxRows: 24 }"
    />

    <template #footer>
      <div class="footer">
        <n-button @click="handleShowUpdate(false)">取消</n-button>
        <n-button type="primary" @click="$emit('copy')">复制配置</n-button>
      </div>
    </template>
  </n-modal>
</template>

<script setup>
import { NButton, NInput, NModal } from "naive-ui";

defineProps({
  show: {
    type: Boolean,
    required: true
  },
  title: {
    type: String,
    default: ""
  },
  content: {
    type: String,
    default: ""
  }
});

const emit = defineEmits(["update:show", "copy"]);

function handleShowUpdate(value) {
  emit("update:show", value);
}
</script>

<style scoped>
.meta {
  margin-bottom: 12px;
}

.meta-title {
  font-size: 15px;
  font-weight: 600;
  margin-bottom: 4px;
}

.meta-desc {
  color: var(--muted-text);
  line-height: 1.6;
}

.footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
