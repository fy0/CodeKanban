<template>
  <section
    class="async-input-card"
    :class="{ 'is-collapsed': collapsed, 'is-submitted': submitted }"
    :aria-label="t('webSession.asyncInputTitle')"
  >
    <template v-if="submitted">
      <div class="async-input-heading">
        <strong>{{ t('webSession.asyncInputTitle') }}</strong>
        <span class="async-input-status">{{ t('webSession.asyncInputSubmitted') }}</span>
      </div>
      <div v-for="answer in displayedAnswers" :key="answer.id" class="async-input-answer">
        <strong>{{ answer.label }}</strong>
        <span>{{ answer.values.join('\n') }}</span>
      </div>
    </template>
    <div v-else-if="collapsed" class="async-input-heading">
      <span>{{ t('webSession.asyncInputTitle') }}</span>
      <button class="async-input-later" type="button" @click="collapsed = false">
        {{ t('webSession.asyncInputResume') }}
      </button>
    </div>
    <form v-else-if="currentQuestion" @submit.prevent="submit">
      <div class="async-input-header">
        <h3>{{ currentQuestion.question }}</h3>
        <div class="async-input-navigation">
          <template v-if="questions.length > 1">
            <button
              type="button"
              class="async-input-icon"
              :aria-label="t('webSession.asyncInputPrevious')"
              :disabled="questionIndex === 0 || submitting"
              @click="changeQuestion(questionIndex - 1)"
            >
              ‹
            </button>
            <span
              class="async-input-progress"
              :aria-label="
                t('webSession.asyncInputProgress', {
                  current: questionIndex + 1,
                  total: questions.length,
                })
              "
              >{{ questionIndex + 1 }} / {{ questions.length }}</span
            >
            <button
              type="button"
              class="async-input-icon"
              :aria-label="t('webSession.asyncInputNext')"
              :disabled="questionIndex === questions.length - 1 || submitting"
              @click="changeQuestion(questionIndex + 1)"
            >
              ›
            </button>
          </template>
          <button
            type="button"
            class="async-input-icon async-input-close"
            :aria-label="t('webSession.asyncInputLater')"
            :disabled="submitting"
            @click="collapsed = true"
          >
            ×
          </button>
        </div>
      </div>
      <div class="async-input-options" role="group" :aria-label="currentQuestion.question">
        <button
          v-for="(option, index) in currentQuestion.options"
          :key="`${currentQuestion.id}:${index}`"
          class="async-input-option"
          :class="{ 'is-selected': selections[currentQuestion.id]?.includes(option.label) }"
          type="button"
          :aria-pressed="selections[currentQuestion.id]?.includes(option.label) ?? false"
          :disabled="disabled || submitting"
          @click="selectAnswer(currentQuestion.id, [option.label])"
        >
          <span class="async-input-number">{{ index + 1 }}</span>
          <span class="async-input-option-label">{{ option.label }}</span>
          <span class="async-input-arrow" aria-hidden="true">→</span>
        </button>
        <div
          class="async-input-custom"
          :class="{ 'is-selected': drafts[currentQuestion.id]?.trim() }"
        >
          <span class="async-input-number">{{ currentQuestion.options.length + 1 }}</span>
          <n-input
            :key="currentQuestion.id"
            :value="drafts[currentQuestion.id] ?? ''"
            type="textarea"
            :autosize="{ minRows: 1, maxRows: 5 }"
            :bordered="false"
            :disabled="disabled || submitting"
            :placeholder="t('webSession.asyncInputPlaceholder')"
            :input-props="{ 'aria-label': t('webSession.asyncInputPlaceholder') }"
            @update:value="writeAnswer(currentQuestion.id, $event)"
          />
        </div>
      </div>
      <p v-if="error" class="async-input-error" role="alert">{{ error }}</p>
      <div class="async-input-footer">
        <span class="async-input-hint">{{ t('webSession.asyncInputWorkingHint') }}</span>
        <button
          class="async-input-later"
          type="button"
          :disabled="submitting"
          @click="collapsed = true"
        >
          {{ t('webSession.asyncInputLater') }}
        </button>
        <button
          class="async-input-submit"
          type="submit"
          :disabled="
            disabled || submitting || (questionIndex === questions.length - 1 && !canSubmit)
          "
        >
          {{
            submitting
              ? t('webSession.asyncInputSubmitting')
              : questionIndex < questions.length - 1
                ? t('webSession.asyncInputNext')
                : t('webSession.userInputSubmit')
          }}
        </button>
      </div>
    </form>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { NInput } from 'naive-ui';

import { useWebSessionStore, type WebSessionBlock } from '@/stores/webSession';

const props = defineProps<{ sessionId: string; item: WebSessionBlock; disabled?: boolean }>();
const { t } = useI18n();
const store = useWebSessionStore();
const selections = ref<Record<string, string[]>>({});
const drafts = ref<Record<string, string>>({});
const submitting = ref(false);
const accepted = ref(false);
const error = ref('');
const collapsed = ref(false);
const questionIndex = ref(0);
const questions = computed(() => props.item.detail?.questions ?? []);
const currentQuestion = computed(() => questions.value[questionIndex.value]);
const storageKey = computed(() => JSON.stringify(['async', props.sessionId, props.item.id]));
const submitted = computed(() => accepted.value || props.item.detail?.action === 'submitted');
const answers = computed(() =>
  Object.fromEntries(
    questions.value.map(question => {
      const draft = drafts.value[question.id]?.trim();
      return [question.id, draft ? [draft] : (selections.value[question.id] ?? [])];
    })
  )
);
const canSubmit = computed(
  () =>
    questions.value.length > 0 &&
    questions.value.every(question => answers.value[question.id]?.some(value => value.trim()))
);
const displayedAnswers = computed(
  () =>
    props.item.detail?.answers ??
    questions.value.map(question => ({
      id: question.id,
      label: question.question,
      values: answers.value[question.id] ?? [],
    }))
);

watch(
  storageKey,
  () => {
    const saved = store.getPendingUserInputDraft(storageKey.value);
    selections.value = Object.fromEntries(
      questions.value.map(question => [
        question.id,
        saved?.selections[question.id] ?? (question.options[0] ? [question.options[0].label] : []),
      ])
    );
    drafts.value = { ...saved?.drafts };
    accepted.value = false;
    collapsed.value = false;
    questionIndex.value = 0;
    error.value = '';
  },
  { immediate: true }
);

watch(
  [selections, drafts],
  () => {
    if (!submitted.value) {
      store.setPendingUserInputDraft(storageKey.value, {
        selections: selections.value,
        drafts: drafts.value,
      });
    }
  },
  { deep: true }
);

function changeQuestion(index: number) {
  if (!submitting.value)
    questionIndex.value = Math.max(0, Math.min(index, questions.value.length - 1));
}

function selectAnswer(id: string, value: string[]) {
  selections.value[id] = value;
  drafts.value[id] = '';
}

function writeAnswer(id: string, value: string) {
  drafts.value[id] = value;
  if (value.trim()) selections.value[id] = [];
}

async function submit() {
  if (props.disabled || submitting.value || submitted.value) return;
  if (questionIndex.value < questions.value.length - 1) {
    changeQuestion(questionIndex.value + 1);
    return;
  }
  if (!canSubmit.value) return;
  submitting.value = true;
  error.value = '';
  try {
    await store.answerAsyncUserInput(props.sessionId, props.item.id, answers.value);
    accepted.value = true;
    store.clearPendingUserInputDraft(storageKey.value);
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : String(cause);
  } finally {
    submitting.value = false;
  }
}
</script>

<style scoped>
.async-input-card {
  min-width: 0;
  padding: 16px 10px 10px;
  border: 1px solid color-mix(in srgb, var(--app-border-color, #d1d5db) 55%, transparent);
  border-radius: 22px;
  background: var(--app-surface-raised, var(--app-surface-color, #fff));
  color: var(--app-text-primary, #172238);
  box-shadow: 0 4px 24px rgb(0 0 0 / 5%);
}
.async-input-header,
.async-input-heading,
.async-input-navigation,
.async-input-footer {
  display: flex;
  align-items: center;
}
.async-input-header {
  gap: 12px;
  padding: 2px 6px 12px;
  align-items: flex-start;
}
.async-input-header h3 {
  flex: 1;
  min-width: 0;
  margin: 0;
  font-size: 13px;
  line-height: 1.6;
  font-weight: 500;
  overflow-wrap: anywhere;
}
.async-input-navigation {
  gap: 3px;
  flex-shrink: 0;
  height: 24px;
}
.async-input-progress {
  color: var(--n-text-color-2, #737373);
  font-size: 10px;
  white-space: nowrap;
}
button {
  font: inherit;
  color: inherit;
  cursor: pointer;
}
button:disabled {
  cursor: default;
}
button:focus-visible {
  outline: 2px solid var(--app-accent-color, #7385a3);
  outline-offset: 2px;
}
.async-input-icon {
  display: grid;
  place-items: center;
  width: 20px;
  height: 24px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  background: transparent;
  font-size: 19px;
  line-height: 1;
}
.async-input-icon:disabled {
  opacity: 0.28;
}
.async-input-icon:hover:enabled {
  background: var(--app-surface-sunken, #f3f3f3);
}
.async-input-close {
  margin-left: 8px;
  font-size: 18px;
}
.async-input-options {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.async-input-option,
.async-input-custom {
  box-sizing: border-box;
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
  min-height: 38px;
  padding: 6px;
  border: 0;
  border-radius: 24px;
  background: transparent;
  text-align: left;
}
.async-input-option.is-selected,
.async-input-option:hover:enabled,
.async-input-custom:focus-within {
  background: var(--app-surface-sunken, #f3f3f3);
}
.async-input-number {
  display: grid;
  place-items: center;
  flex-shrink: 0;
  width: 24px;
  height: 24px;
  border: 1px solid var(--app-border-color, #ddd);
  border-radius: 50%;
  font-size: 11px;
  font-weight: 400;
}
.async-input-option-label {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  line-height: 1.5;
  overflow-wrap: anywhere;
}
.async-input-arrow {
  flex-shrink: 0;
  padding-right: 8px;
  color: var(--n-text-color-2, #737373);
  opacity: 0;
}
.async-input-option.is-selected .async-input-arrow,
.async-input-option:hover .async-input-arrow {
  opacity: 1;
}
.async-input-option:disabled {
  opacity: 0.55;
}
.async-input-custom {
  align-items: flex-start;
  border-radius: 16px;
}
.async-input-custom > .async-input-number {
  margin-top: 3px;
}
.async-input-custom :deep(.n-input) {
  --n-color: transparent !important;
  --n-color-focus: transparent !important;
  --n-box-shadow-focus: none !important;
  flex: 1;
  min-width: 0;
  font-size: 13px;
}
.async-input-custom :deep(.n-input-wrapper) {
  padding-left: 0;
  padding-right: 8px;
}
.async-input-footer {
  gap: 8px;
  padding: 10px 6px 2px;
}
.async-input-hint {
  flex: 1;
  color: var(--n-text-color-2, #888);
  font-size: 11px;
}
.async-input-later,
.async-input-submit {
  flex-shrink: 0;
  padding: 5px 12px;
  border: 1px solid var(--app-border-color, #ddd);
  border-radius: 999px;
  background: transparent;
  font-size: 11px;
  line-height: 1.5;
}
.async-input-later:hover:enabled {
  background: var(--app-surface-sunken, #f3f3f3);
}
.async-input-submit {
  border-color: transparent;
  background: var(--app-text-primary, #172238);
  color: var(--app-surface-raised, #fff);
}
.async-input-submit:disabled {
  opacity: 0.35;
}
.async-input-error {
  margin: 8px 6px 0;
  color: var(--app-error, #dc2626);
  font-size: 12px;
  overflow-wrap: anywhere;
}
.async-input-heading {
  justify-content: space-between;
  gap: 12px;
  padding: 0 6px 6px;
  font-size: 13px;
}
.async-input-status {
  color: var(--app-success, #15803d);
  font-size: 12px;
}
.async-input-answer {
  display: flex;
  flex-direction: column;
  gap: 5px;
  margin: 14px 6px 6px;
  font-size: 13px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.async-input-card.is-collapsed {
  padding-top: 10px;
  border-radius: 16px;
  box-shadow: none;
}
@media (max-width: 480px) {
  .async-input-header {
    flex-wrap: wrap-reverse;
    gap: 6px;
  }
  .async-input-navigation {
    margin-left: auto;
  }
  .async-input-header h3 {
    flex-basis: 100%;
  }
  .async-input-footer {
    flex-wrap: wrap;
  }
  .async-input-hint {
    flex-basis: 100%;
  }
  .async-input-later {
    margin-left: auto;
  }
}
</style>
