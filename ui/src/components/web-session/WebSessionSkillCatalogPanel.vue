<template>
  <div class="skill-catalog-panel">
    <div class="skill-catalog-panel__header">
      <n-input
        v-model:value="search"
        size="small"
        clearable
        :placeholder="t('webSession.skillSearchPlaceholder')"
      />
    </div>

    <div v-if="loading" class="skill-catalog-panel__empty">
      {{ t('common.loading') }}
    </div>
    <div v-else-if="filteredSkills.length === 0" class="skill-catalog-panel__empty">
      {{ emptyLabel }}
    </div>
    <div v-else class="skill-catalog-panel__list">
      <div v-for="skill in filteredSkills" :key="skill.name" class="skill-catalog-panel__item">
        <button
          type="button"
          class="skill-catalog-panel__item-main"
          @click="emit('select-token', skill)"
        >
          <div class="skill-catalog-panel__title-line">
            <span class="skill-catalog-panel__title">{{ skill.displayName }}</span>
            <span class="skill-catalog-panel__source">
              {{ sourceLabel(skill.source) }}
            </span>
          </div>
          <div class="skill-catalog-panel__token">{{ buildCodexSkillToken(skill.name) }}</div>
          <div v-if="skill.description" class="skill-catalog-panel__description">
            {{ skill.description }}
          </div>
        </button>
        <button
          v-if="skill.defaultPrompt"
          type="button"
          class="skill-catalog-panel__template-btn"
          @click="emit('select-template', skill)"
        >
          {{ t('webSession.skillInsertTemplate') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useLocale } from '@/composables/useLocale';
import {
  buildCodexSkillToken,
  filterCodexSkills,
} from '@/components/web-session/webSessionCodexSkills';
import type { CodexSkillSource, CodexSkillSummary } from '@/types/models';

const props = withDefaults(
  defineProps<{
    skills?: CodexSkillSummary[];
    loading?: boolean;
  }>(),
  {
    skills: () => [],
    loading: false,
  }
);

const emit = defineEmits<{
  (event: 'select-token', skill: CodexSkillSummary): void;
  (event: 'select-template', skill: CodexSkillSummary): void;
}>();

const { t } = useLocale();
const search = ref('');

const filteredSkills = computed(() => filterCodexSkills(props.skills, search.value));
const emptyLabel = computed(() =>
  search.value.trim() ? t('webSession.skillSearchEmpty') : t('webSession.skillEmpty')
);

watch(
  () => props.skills,
  nextSkills => {
    if (nextSkills.length > 0) {
      return;
    }
    search.value = '';
  }
);

function sourceLabel(source: CodexSkillSource) {
  switch (source) {
    case 'user':
      return t('webSession.skillSourceUser');
    case 'bundled':
      return t('webSession.skillSourceBundled');
    default:
      return t('webSession.skillSourceSystem');
  }
}
</script>

<style scoped>
.skill-catalog-panel {
  width: min(420px, 82vw);
  max-width: 100%;
  box-sizing: border-box;
  display: grid;
  gap: 10px;
  padding: 12px;
}

.skill-catalog-panel__header {
  position: sticky;
  top: 0;
  z-index: 1;
  background: var(--app-surface-raised, inherit);
}

.skill-catalog-panel__list {
  display: grid;
  gap: 8px;
  max-height: min(56vh, 440px);
  overflow-y: auto;
  overscroll-behavior: contain;
  padding-right: 2px;
}

.skill-catalog-panel__item {
  display: grid;
  gap: 6px;
  padding: 10px;
  border-radius: 14px;
  border: 1px solid color-mix(in srgb, var(--app-border, var(--n-border-color)) 78%, transparent);
  background: color-mix(
    in srgb,
    var(--app-surface-raised, #fff) 96%,
    var(--app-accent, var(--n-primary-color)) 4%
  );
}

.skill-catalog-panel__item-main,
.skill-catalog-panel__template-btn {
  border: none;
  background: transparent;
  text-align: left;
  cursor: pointer;
  padding: 0;
}

.skill-catalog-panel__item-main {
  display: grid;
  gap: 6px;
  color: inherit;
}

.skill-catalog-panel__item-main:hover .skill-catalog-panel__title,
.skill-catalog-panel__template-btn:hover {
  color: var(--app-focus-ring, var(--n-primary-color));
}

.skill-catalog-panel__title-line {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.skill-catalog-panel__title {
  font-size: 13px;
  font-weight: 700;
  color: var(--app-text-primary, var(--n-text-color));
}

.skill-catalog-panel__source {
  flex-shrink: 0;
  padding: 2px 8px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--app-border, var(--n-border-color)) 82%, transparent);
  color: var(--app-text-secondary, var(--n-text-color-2));
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.skill-catalog-panel__token {
  font-size: 12px;
  color: var(--app-focus-ring, var(--n-primary-color));
  word-break: break-all;
}

.skill-catalog-panel__description {
  font-size: 12px;
  line-height: 1.5;
  color: var(--app-text-secondary, var(--n-text-color-2));
}

.skill-catalog-panel__template-btn {
  justify-self: flex-start;
  padding: 4px 0 0;
  font-size: 11px;
  font-weight: 600;
  color: var(--app-text-secondary, var(--n-text-color-2));
}

.skill-catalog-panel__empty {
  padding: 16px 8px;
  text-align: center;
  font-size: 12px;
  line-height: 1.5;
  color: var(--app-text-muted, var(--n-text-color-3));
}
</style>
