import { defineStore } from 'pinia';
import { ref } from 'vue';

export const SETTINGS_SECTION_IDS = [
  'project-workspace',
  'terminal',
  'session',
  'security',
  'developer',
  'worktree',
  'theme',
  'backup',
] as const;

export type SettingsSectionId = (typeof SETTINGS_SECTION_IDS)[number];

export interface OpenSettingsOptions {
  query?: string;
  section?: SettingsSectionId;
}

const DEFAULT_SECTION_ID: SettingsSectionId = 'project-workspace';

export function isSettingsSectionId(value: string | null | undefined): value is SettingsSectionId {
  if (!value) {
    return false;
  }
  return SETTINGS_SECTION_IDS.includes(value as SettingsSectionId);
}

export function sanitizeSettingsSectionId(
  value: string | null | undefined,
  fallback: SettingsSectionId = DEFAULT_SECTION_ID
): SettingsSectionId {
  if (value === 'project-terminal') {
    return 'project-workspace';
  }
  if (value === 'terminal-actions' || value === 'ai-status') {
    return 'terminal';
  }
  if (value === 'preview') {
    return 'theme';
  }
  return isSettingsSectionId(value) ? value : fallback;
}

export const useSettingsUiStore = defineStore('settingsUi', () => {
  const isOpen = ref(false);
  const activeSection = ref<SettingsSectionId>(DEFAULT_SECTION_ID);
  const searchQuery = ref('');

  function openSettings(options?: OpenSettingsOptions) {
    if (options?.section) {
      activeSection.value = sanitizeSettingsSectionId(options.section);
    }
    searchQuery.value = typeof options?.query === 'string' ? options.query : '';
    isOpen.value = true;
  }

  function closeSettings() {
    isOpen.value = false;
  }

  function setActiveSection(section: SettingsSectionId) {
    activeSection.value = sanitizeSettingsSectionId(section);
  }

  function setSearchQuery(query: string) {
    searchQuery.value = query;
  }

  function resetState() {
    activeSection.value = DEFAULT_SECTION_ID;
    searchQuery.value = '';
  }

  return {
    isOpen,
    activeSection,
    searchQuery,
    openSettings,
    closeSettings,
    setActiveSection,
    setSearchQuery,
    resetState,
  };
});
