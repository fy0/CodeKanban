<template>
  <div class="user-input-question">
    <div v-if="!hideTitle" class="user-input-question-header">
      {{ question.header || question.question }}
    </div>
    <div
      v-if="
        !hideTitle && question.header && question.question && question.header !== question.question
      "
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
        v-for="(option, index) in question.options"
        :key="`${question.id}:${option.label}`"
        class="user-input-option"
        :class="{
          'is-selected': selection.includes(option.label),
          'is-disabled': disabled,
        }"
      >
        <n-checkbox :value="option.label">
          <span class="user-input-number" aria-hidden="true">{{
            selection.includes(option.label) ? '✓' : index + 1
          }}</span>
          <span class="user-input-option-copy">
            <span class="user-input-option-label">{{ option.label }}</span>
            <span v-if="option.description" class="user-input-option-description">{{
              option.description
            }}</span>
          </span>
        </n-checkbox>
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
        v-for="(option, index) in question.options"
        :key="`${question.id}:${option.label}`"
        class="user-input-option"
        :class="{
          'is-selected': selection.includes(option.label),
          'is-disabled': disabled,
        }"
      >
        <n-radio :value="option.label">
          <span class="user-input-number" aria-hidden="true">{{ index + 1 }}</span>
          <span class="user-input-option-copy">
            <span class="user-input-option-label">{{ option.label }}</span>
            <span v-if="option.description" class="user-input-option-description">{{
              option.description
            }}</span>
          </span>
          <span class="user-input-arrow" aria-hidden="true">→</span>
        </n-radio>
      </div>
    </n-radio-group>
    <div v-if="question.isOther || question.options.length === 0" class="user-input-custom">
      <span class="user-input-number" aria-hidden="true">{{ question.options.length + 1 }}</span>
      <n-input
        :value="draft"
        :type="question.isSecret ? 'password' : 'text'"
        :bordered="false"
        :disabled="disabled"
        :show-password-on="question.isSecret ? 'mousedown' : undefined"
        :placeholder="placeholder"
        :input-props="{ 'aria-label': question.question || question.header }"
        @update:value="emit('update:draft', $event)"
        @keydown="emit('keydown', $event)"
      />
    </div>
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
  hideTitle?: boolean;
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
