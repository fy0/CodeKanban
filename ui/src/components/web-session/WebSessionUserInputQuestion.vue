<template>
  <div class="user-input-question">
    <div class="user-input-question-header">
      {{ question.header || question.question }}
    </div>
    <div
      v-if="question.header && question.question && question.header !== question.question"
      class="user-input-question-copy"
    >
      {{ question.question }}
    </div>
    <n-checkbox-group
      v-if="question.options.length > 0 && question.multiSelect"
      :value="selection"
      :disabled="disabled"
      class="user-input-options"
      @update:value="handleMultiSelect"
    >
      <div
        v-for="option in question.options"
        :key="`${question.id}:${option.label}`"
        class="user-input-option"
        :class="{
          'is-selected': selection.includes(option.label),
          'is-disabled': disabled,
        }"
      >
        <n-checkbox :value="option.label">
          <span class="user-input-option-label">{{ option.label }}</span>
        </n-checkbox>
        <div v-if="option.description" class="user-input-option-description">
          {{ option.description }}
        </div>
      </div>
    </n-checkbox-group>
    <n-radio-group
      v-else-if="question.options.length > 0"
      :value="selection[0] || null"
      :disabled="disabled"
      class="user-input-options"
      @update:value="handleSingleSelect"
    >
      <div
        v-for="option in question.options"
        :key="`${question.id}:${option.label}`"
        class="user-input-option"
        :class="{
          'is-selected': selection.includes(option.label),
          'is-disabled': disabled,
        }"
      >
        <n-radio :value="option.label">
          <span class="user-input-option-label">{{ option.label }}</span>
        </n-radio>
        <div v-if="option.description" class="user-input-option-description">
          {{ option.description }}
        </div>
      </div>
    </n-radio-group>
    <n-input
      v-if="question.isOther || question.options.length === 0"
      :value="draft"
      :type="question.isSecret ? 'password' : 'text'"
      size="small"
      :disabled="disabled"
      :show-password-on="question.isSecret ? 'mousedown' : undefined"
      :placeholder="placeholder"
      @update:value="emit('update:draft', $event)"
      @keydown="emit('keydown', $event)"
    />
  </div>
</template>

<script setup lang="ts">
import { NCheckbox, NCheckboxGroup, NInput, NRadio, NRadioGroup } from 'naive-ui';

import type { WebSessionUserInputQuestion } from '@/stores/webSession';

defineProps<{
  question: WebSessionUserInputQuestion;
  selection: string[];
  draft: string;
  disabled: boolean;
  placeholder: string;
}>();

const emit = defineEmits<{
  'update:selection': [value: string[]];
  'update:draft': [value: string];
  keydown: [event: KeyboardEvent];
}>();

function handleMultiSelect(value: Array<string | number>) {
  emit(
    'update:selection',
    value.map(item => String(item))
  );
}

function handleSingleSelect(value: string | number | boolean | null) {
  const normalizedValue = String(value ?? '').trim();
  emit('update:selection', normalizedValue ? [normalizedValue] : []);
}
</script>

<style scoped src="./styles/webSessionUserInputQuestion.css"></style>
