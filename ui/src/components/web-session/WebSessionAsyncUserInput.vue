<template>
  <section
    ref="cardRef"
    class="question-card"
    tabindex="-1"
    :class="{ 'is-collapsed': collapsed || ignored, 'is-submitted': submitted }"
    :aria-label="t('webSession.asyncInputTitle')"
  >
    <template v-if="submitted">
      <div class="question-card-heading">
        <strong>{{ t('webSession.asyncInputTitle') }}</strong>
        <span class="question-card-status">{{ t('webSession.asyncInputSubmitted') }}</span>
      </div>
      <div v-for="answer in displayedAnswers" :key="answer.id" class="question-card-answer">
        <strong>{{ answer.label }}</strong>
        <span>{{ answer.values.join('\n') }}</span>
      </div>
    </template>
    <div v-else-if="ignored" class="question-card-heading">
      <span>{{ questions[0]?.question || t('webSession.asyncInputTitle') }}</span>
      <span class="question-card-status question-card-ignored">{{
        t('webSession.asyncInputIgnored')
      }}</span>
    </div>
    <div v-else-if="collapsed" class="question-card-heading">
      <span>{{ t('webSession.asyncInputTitle') }}</span>
      <button class="question-card-later" type="button" @click="collapsed = false">
        {{ t('webSession.asyncInputResume') }}
      </button>
    </div>
    <form v-else-if="currentQuestion" @submit.prevent="submit">
      <div class="question-card-header">
        <h3>{{ currentQuestion.question }}</h3>
        <div class="question-card-navigation">
          <template v-if="questions.length > 1">
            <button
              type="button"
              class="question-card-icon"
              :aria-label="t('webSession.asyncInputPrevious')"
              :disabled="questionIndex === 0 || busy"
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
              :disabled="questionIndex === questions.length - 1 || busy"
              @click="changeQuestion(questionIndex + 1)"
            >
              ›
            </button>
          </template>
          <button
            type="button"
            class="question-card-icon question-card-close"
            :aria-label="t('webSession.asyncInputLater')"
            :disabled="busy"
            @click="collapsed = true"
          >
            ×
          </button>
        </div>
      </div>
      <div class="question-card-options" role="group" :aria-label="currentQuestion.question">
        <button
          v-for="(option, index) in currentQuestion.options"
          :key="`${currentQuestion.id}:${index}`"
          class="question-card-option"
          :class="{ 'is-selected': selections[currentQuestion.id]?.includes(option.label) }"
          type="button"
          :aria-pressed="selections[currentQuestion.id]?.includes(option.label) ?? false"
          :disabled="disabled || busy"
          @click.capture="preventClickAfterTextSelection"
          @click="selectAnswer(currentQuestion.id, [option.label])"
        >
          <span class="question-card-number">{{ index + 1 }}</span>
          <span class="question-card-option-label">{{ option.label }}</span>
          <span class="question-card-arrow" aria-hidden="true">→</span>
        </button>
        <div
          class="question-card-custom"
          :class="{ 'is-selected': drafts[currentQuestion.id]?.trim() }"
        >
          <span class="question-card-number">{{ currentQuestion.options.length + 1 }}</span>
          <n-input
            :key="currentQuestion.id"
            :value="drafts[currentQuestion.id] ?? ''"
            type="textarea"
            :autosize="{ minRows: 1, maxRows: 5 }"
            :bordered="false"
            :disabled="disabled || busy"
            :placeholder="t('webSession.asyncInputPlaceholder')"
            :input-props="{ 'aria-label': t('webSession.asyncInputPlaceholder') }"
            @update:value="writeAnswer(currentQuestion.id, $event)"
          />
        </div>
      </div>
      <p v-if="error" class="question-card-error" role="alert">{{ error }}</p>
      <div class="question-card-footer">
        <span class="question-card-hint">{{ t('webSession.asyncInputWorkingHint') }}</span>
        <button
          class="question-card-ignore"
          type="button"
          :disabled="disabled || busy"
          @click="ignore"
        >
          {{ t(ignoring ? 'webSession.asyncInputIgnoring' : 'webSession.asyncInputIgnore') }}
        </button>
        <button
          class="question-card-later"
          type="button"
          :disabled="busy"
          @click="collapsed = true"
        >
          {{ t('webSession.asyncInputLater') }}
        </button>
        <button
          class="question-card-submit"
          type="submit"
          :disabled="disabled || busy || (questionIndex === questions.length - 1 && !canSubmit)"
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
import { computed, nextTick, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { NInput } from 'naive-ui';
import { preventClickAfterTextSelection } from '@/utils/textSelection';

import { useWebSessionStore, type WebSessionBlock } from '@/stores/webSession';

const props = defineProps<{ sessionId: string; item: WebSessionBlock; disabled?: boolean }>();
const { t } = useI18n();
const store = useWebSessionStore();
const cardRef = ref<HTMLElement | null>(null);
const selections = ref<Record<string, string[]>>({});
const drafts = ref<Record<string, string>>({});
const submitting = ref(false);
const ignoring = ref(false);
const busy = computed(() => submitting.value || ignoring.value);
const accepted = ref(false);
const acceptedIgnore = ref(false);
const error = ref('');
const collapsed = ref(false);
const questionIndex = ref(0);
// Free-text questions can arrive without options because the server omits empty lists.
const questions = computed(() =>
  (props.item.detail?.questions ?? []).map(question => ({
    ...question,
    options: question.options ?? [],
  }))
);
const currentQuestion = computed(() => questions.value[questionIndex.value]);
const storageKey = computed(() => JSON.stringify(['async', props.sessionId, props.item.id]));
const submitted = computed(() => accepted.value || props.item.detail?.action === 'submitted');
const ignored = computed(() => acceptedIgnore.value || props.item.detail?.action === 'ignored');
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
    acceptedIgnore.value = false;
    collapsed.value = false;
    questionIndex.value = 0;
    error.value = '';
  },
  { immediate: true }
);

watch(
  [selections, drafts],
  () => {
    if (!submitted.value && !ignored.value) {
      store.setPendingUserInputDraft(storageKey.value, {
        selections: selections.value,
        drafts: drafts.value,
      });
    }
  },
  { deep: true }
);

function changeQuestion(index: number) {
  if (!busy.value) questionIndex.value = Math.max(0, Math.min(index, questions.value.length - 1));
}

function selectAnswer(id: string, value: string[]) {
  selections.value[id] = value;
  drafts.value[id] = '';
}

function writeAnswer(id: string, value: string) {
  drafts.value[id] = value;
  if (value.trim()) selections.value[id] = [];
}

async function reveal() {
  collapsed.value = false;
  await nextTick();
  cardRef.value?.focus({ preventScroll: true });
}

defineExpose({ reveal });

async function submit() {
  if (props.disabled || busy.value || submitted.value || ignored.value) return;
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

async function ignore() {
  if (props.disabled || busy.value || submitted.value || ignored.value) return;
  ignoring.value = true;
  error.value = '';
  try {
    await store.ignoreAsyncUserInput(props.sessionId, props.item.id);
    acceptedIgnore.value = true;
    store.clearPendingUserInputDraft(storageKey.value);
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : String(cause);
  } finally {
    ignoring.value = false;
  }
}
</script>

<style scoped src="./styles/webSessionQuestionCard.css"></style>
