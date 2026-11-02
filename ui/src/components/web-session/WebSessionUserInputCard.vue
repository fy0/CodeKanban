<template>
  <form
    class="question-card"
    :class="{ 'is-stale': stale }"
    :aria-label="t('webSession.userInputTitle')"
    @submit.prevent="advanceOrSubmit"
  >
    <div v-if="currentQuestion" class="question-card-header">
      <h3>{{ currentQuestion.question || currentQuestion.header }}</h3>
      <div v-if="questions.length > 1" class="question-card-navigation">
        <button
          type="button"
          class="question-card-icon"
          :aria-label="t('webSession.asyncInputPrevious')"
          :disabled="questionIndex === 0 || submitting"
          @click="changeQuestion(questionIndex - 1)"
        >
          ‹
        </button>
        <span
          class="question-card-progress"
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
          class="question-card-icon"
          :aria-label="t('webSession.asyncInputNext')"
          :disabled="questionIndex === questions.length - 1 || submitting"
          @click="changeQuestion(questionIndex + 1)"
        >
          ›
        </button>
      </div>
    </div>
    <p v-else class="question-card-note">{{ prompt || t('webSession.userInputPromptFallback') }}</p>
    <p v-if="stale" class="question-card-note">
      {{ recoveryMessage || t('webSession.recoveredRuntimeHint') }}
    </p>
    <WebSessionUserInputQuestion
      v-if="currentQuestion"
      :key="currentQuestion.id"
      :question="currentQuestion"
      :selection="selections[currentQuestion.id] ?? []"
      :draft="drafts[currentQuestion.id] ?? ''"
      :disabled="disabled || submitting"
      :placeholder="
        t(
          currentQuestion.options.length > 0
            ? 'webSession.userInputOtherPlaceholder'
            : 'webSession.userInputAnswerPlaceholder'
        )
      "
      hide-title
      @update:selection="emit('update:selection', currentQuestion.id, $event)"
      @update:draft="emit('update:draft', currentQuestion.id, $event)"
      @keydown="handleInputKeydown"
    />
    <div class="question-card-footer">
      <span class="question-card-hint">{{ t('webSession.userInputWaitingHint') }}</span>
      <button
        class="question-card-later"
        type="button"
        :disabled="disabled || submitting"
        @click="emit('stop')"
      >
        {{ t('webSession.stop') }}
      </button>
      <button class="question-card-submit" type="submit" :disabled="disabled || submitting">
        {{
          submitting
            ? t('webSession.asyncInputSubmitting')
            : questionIndex < questions.length - 1
              ? t('webSession.asyncInputNext')
              : t('webSession.userInputSubmit')
        }}
      </button>
    </div>
    <p v-if="slow" class="question-card-note" aria-live="polite">
      {{ t('webSession.userInputSubmitSlow') }}
    </p>
  </form>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';

import type { WebSessionUserInputQuestion as Question } from '@/stores/webSession';
import WebSessionUserInputQuestion from './WebSessionUserInputQuestion.vue';

const props = defineProps<{
  requestKey: string;
  questions: Question[];
  selections: Record<string, string[]>;
  drafts: Record<string, string>;
  prompt?: string;
  disabled: boolean;
  submitting: boolean;
  stale?: boolean;
  recoveryMessage?: string;
  slow?: boolean;
}>();
const emit = defineEmits<{
  'update:selection': [id: string, value: string[]];
  'update:draft': [id: string, value: string];
  submit: [];
  stop: [];
}>();
const { t } = useI18n();
const questionIndex = ref(0);
const currentQuestion = computed(() => props.questions[questionIndex.value]);

watch(
  () => props.requestKey,
  () => {
    questionIndex.value = 0;
  }
);

function changeQuestion(index: number) {
  if (!props.submitting)
    questionIndex.value = Math.max(0, Math.min(index, props.questions.length - 1));
}

function advanceOrSubmit() {
  if (props.disabled || props.submitting) return;
  if (questionIndex.value < props.questions.length - 1) {
    changeQuestion(questionIndex.value + 1);
    return;
  }
  const missingIndex = props.questions.findIndex(
    question => !props.selections[question.id]?.length && !props.drafts[question.id]?.trim()
  );
  if (missingIndex >= 0) {
    questionIndex.value = missingIndex;
    return;
  }
  emit('submit');
}

function handleInputKeydown(event: KeyboardEvent) {
  if (
    event.key !== 'Enter' ||
    event.shiftKey ||
    event.ctrlKey ||
    event.altKey ||
    event.metaKey ||
    event.isComposing ||
    event.keyCode === 229
  )
    return;
  event.preventDefault();
  event.stopPropagation();
  advanceOrSubmit();
}
</script>

<style scoped src="./styles/webSessionQuestionCard.css"></style>
