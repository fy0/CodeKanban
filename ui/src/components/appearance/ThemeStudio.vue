<script setup lang="ts">
import { computed, ref, watch, useId } from 'vue';
import { useLocale } from '@/composables/useLocale';
import {
  useUnifiedThemeChoices,
  type UnifiedThemeChoice,
} from '@/composables/useUnifiedThemeChoices';
import { isSupportedThemePreset } from '@/constants/themes';
import {
  createThemeMaintenanceWarningController,
  createThemeSelectionController,
} from '@/utils/themeMaintenanceWarning';
import { NAlert, NButton, NCollapse, NCollapseItem, NModal, useDialog } from 'naive-ui';
import { useAppearanceStore, MAX_APPEARANCE_BYTES } from '@/stores/appearance';
import { useSettingsStore } from '@/stores/settings';
import {
  COLOR_KEYS,
  contrastRatio,
  resolveTheme,
  validateManifest,
  type AppearanceState,
  type ColorKey,
  type LayoutTokens,
  type Mode,
} from '@/themes/core';
import { isDarkHex } from '@/utils/color';
import { getRegisteredThemes } from '@/themes/registry';
import AppearanceSlot from './AppearanceSlot.vue';

const titleId = useId();
const appearance = useAppearanceStore();
const settings = useSettingsStore();
const dialog = useDialog();
const { t, locale } = useLocale();
const { choices, selectedKey, select } = useUnifiedThemeChoices();
const en = computed(() => locale.value.startsWith('en'));
const text = (zh: string, english: string) => (en.value ? english : zh);
const selectedLabel = computed(
  () =>
    choices.value.find(choice => choice.key === selectedKey.value)?.label ||
    appearance.selected.name
);
const maintenance = createThemeMaintenanceWarningController({
  t: key => t(key),
  warning: options => {
    dialog.warning(options);
  },
});
const selectionController = createThemeSelectionController({
  getCurrentPresetId: () =>
    appearance.isClassic && !settings.customTheme ? settings.currentPresetId : '',
  isFollowSystemTheme: () => settings.followSystemTheme,
  selectPreset: id => settings.selectPreset(id),
  toggleFollowSystemTheme: enabled => settings.toggleFollowSystemTheme(enabled),
  confirmPresetThemeChange: maintenance.confirmPresetThemeChange,
  confirmFollowSystemEnable: maintenance.confirmFollowSystemEnable,
  shouldConfirmPresetThemeChange: id => !isSupportedThemePreset(id),
  shouldConfirmFollowSystemEnable: () => false,
});
const currentMode = computed<Mode>(() =>
  appearance.isClassic
    ? settings.followSystemTheme
      ? 'system'
      : isDarkHex(settings.activeTheme.bodyColor)
        ? 'dark'
        : 'light'
    : appearance.mode
);
const colors = computed(() => appearance.resolved.palette);
const layout = computed(() => appearance.resolved.layout);
const contrast = computed(() => contrastRatio(colors.value.text, colors.value.surface));
const mutedContrast = computed(() => contrastRatio(colors.value.muted, colors.value.surface));
const error = ref('');
const fileInput = ref<HTMLInputElement | null>(null);
const importOpen = ref(false);
const importText = ref('');
const replaceExisting = ref(false);
const preview = ref<AppearanceState | null>(null);
const previewVariants = computed(() => {
  const draft = preview.value;
  if (!draft) return [];
  return (['light', 'dark'] as const).map(scheme => ({
    scheme,
    ...resolveTheme(
      draft.themeId,
      [...getRegisteredThemes(), ...draft.customThemes],
      scheme,
      draft.overrides[draft.themeId]
    ),
  }));
});
const isBundle = ref(false);
const importError = ref('');
const colorNames: Record<ColorKey, [string, string]> = {
  canvas: ['画布', 'Canvas'],
  surface: ['面板', 'Surface'],
  raised: ['浮层', 'Raised surface'],
  sunken: ['嵌入区域', 'Sunken surface'],
  text: ['主要文字', 'Primary text'],
  muted: ['次级文字', 'Secondary text'],
  subtle: ['辅助文字', 'Muted text'],
  border: ['边框', 'Border'],
  accent: ['强调色', 'Accent'],
  success: ['成功', 'Success'],
  warning: ['警告／待审批', 'Warning / approval'],
  error: ['错误', 'Error'],
  info: ['信息', 'Information'],
  plan: ['计划', 'Plan'],
  working: ['执行中', 'Working'],
  addition: ['新增代码', 'Added lines'],
  deletion: ['删除代码', 'Removed lines'],
  terminalBg: ['终端背景', 'Terminal background'],
  terminalFg: ['终端文字', 'Terminal foreground'],
};
const colorGroups = [COLOR_KEYS.slice(0, 9), COLOR_KEYS.slice(9, 17), COLOR_KEYS.slice(17)];
function safe(action: () => void) {
  error.value = '';
  try {
    action();
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : String(cause);
  }
}
function changeMode(mode: Mode) {
  safe(() => {
    if (appearance.isClassic) {
      if (mode === 'system') settings.toggleFollowSystemTheme(true);
      else settings.selectPreset(mode);
    }
    appearance.setMode(mode);
  });
}
function setColor(key: ColorKey, event: Event) {
  safe(() =>
    appearance.updateColor(appearance.scheme, key, (event.target as HTMLInputElement).value)
  );
}
function changeLayout(key: keyof LayoutTokens, event: Event, numeric = false) {
  const value = (event.target as HTMLInputElement).value;
  safe(() => appearance.updateLayout({ [key]: numeric ? Number(value) : value }));
}
function resetCurrentCustomization() {
  safe(() => {
    appearance.resetOverrides();
    if (appearance.isClassic && settings.customTheme) {
      settings.selectPreset(settings.currentPresetId);
    }
  });
}
async function chooseTheme(key: string) {
  if (!key.startsWith('preset:')) {
    safe(() => select(key));
    return;
  }
  error.value = '';
  try {
    if (selectedKey.value !== key)
      await selectionController.selectPresetWithConfirmation(key.slice(7));
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : String(cause);
  }
}
function cardStyle(choice: UnifiedThemeChoice) {
  return {
    '--preview-bg': choice.preview.canvas,
    '--preview-surface': choice.preview.surface,
    '--preview-border': choice.preview.border,
    '--preview-accent': choice.preview.accent,
    '--preview-ink': choice.preview.text,
  };
}
function download(name: string, value: unknown) {
  const url = URL.createObjectURL(
    new Blob([JSON.stringify(value, null, 2) + '\n'], { type: 'application/json' })
  );
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = name;
  anchor.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
function exportTheme() {
  safe(() => {
    let manifest = appearance.exportTheme();
    if (appearance.isClassic) {
      // Older preset themes also export their actual selected colors, not the Classic base.
      const old = settings.activeTheme;
      const dark = isDarkHex(old.bodyColor);
      const base = resolveTheme(
        'classic',
        appearance.themes,
        dark ? 'dark' : 'light',
        appearance.preferences.overrides.classic
      ).palette;
      const palette = {
        ...base,
        canvas: old.bodyColor,
        surface: old.surfaceColor,
        raised: old.surfaceRaisedColor || old.surfaceColor,
        sunken: old.surfaceSunkenColor || old.bodyColor,
        text: old.textColor || base.text,
        muted: old.secondaryTextColor || base.muted,
        subtle: old.mutedTextColor || base.subtle,
        border: old.borderColor || base.border,
        accent: old.primaryColor,
        success: old.successColor || base.success,
        warning: old.warningColor || base.warning,
        error: old.errorColor || base.error,
        info: old.infoColor || base.info,
        plan: old.planColor || base.plan,
        working: old.workingColor || base.working,
        addition: old.changeAdditionColor || base.addition,
        deletion: old.changeDeletionColor || base.deletion,
        terminalBg: settings.activeTerminalTheme.background,
        terminalFg: settings.activeTerminalTheme.foreground,
      };
      const name = selectedLabel.value;
      manifest = validateManifest({
        ...manifest,
        name: `${name} · Custom`.slice(0, 80),
        nameEn: `${name} Custom`.slice(0, 80),
        defaultMode: settings.followSystemTheme ? 'system' : dark ? 'dark' : 'light',
        colors: { light: palette, dark: palette },
      });
    }
    download(`${selectedKey.value.replace(':', '-')}-theme.json`, manifest);
  });
}
function exportPreferences() {
  safe(() => download('codekanban-appearance.json', appearance.snapshot()));
}
async function readFile(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file) return;
  importError.value = '';
  try {
    if (file.size > MAX_APPEARANCE_BYTES) throw new Error('File exceeds 2 MiB');
    importText.value = await file.text();
    inspectImport();
  } catch (cause) {
    importError.value = cause instanceof Error ? cause.message : String(cause);
  } finally {
    input.value = '';
  }
}
function inspectImport() {
  preview.value = null;
  importError.value = '';
  try {
    if (new TextEncoder().encode(importText.value).length > MAX_APPEARANCE_BYTES)
      throw new Error('File exceeds 2 MiB');
    const raw = JSON.parse(importText.value) as { version?: unknown };
    isBundle.value = !!raw && raw.version === 1;
    preview.value = isBundle.value
      ? appearance.parseBundle(importText.value)
      : appearance.previewImport(importText.value, replaceExisting.value);
  } catch (cause) {
    importError.value = cause instanceof Error ? cause.message : String(cause);
  }
}
function confirmImport() {
  try {
    // Revalidate against the current registry/state; another browser tab may have changed it.
    if (isBundle.value) appearance.commit(appearance.parseBundle(importText.value));
    else appearance.importTheme(importText.value, replaceExisting.value);
    importOpen.value = false;
    importText.value = '';
    preview.value = null;
  } catch (cause) {
    importError.value = cause instanceof Error ? cause.message : String(cause);
  }
}
function removeTheme(id: string) {
  dialog.warning({
    title: text('移除自定义主题', 'Remove custom theme'),
    content: text(
      '仅移除这份本地主题及它的自定义项，不影响项目或会话。',
      'Only this local theme and its overrides will be removed. Projects and sessions are unaffected.'
    ),
    positiveText: text('移除', 'Remove'),
    negativeText: text('取消', 'Cancel'),
    onPositiveClick: () => safe(() => appearance.removeTheme(id)),
  });
}
watch(
  [importText, replaceExisting],
  () => {
    preview.value = null;
    importError.value = '';
  },
  { flush: 'sync' }
);
</script>

<template>
  <section class="ck-studio" :aria-labelledby="titleId">
    <header class="ck-studio-heading">
      <div>
        <h3 :id="titleId">{{ text('主题', 'Themes') }}</h3>
        <p>
          {{
            text(
              '选择或导入主题，所有主题都可以在这里切换和自定义。',
              'Choose or import a theme. Every theme can be selected and customized here.'
            )
          }}
        </p>
      </div>
    </header>
    <NAlert v-if="appearance.notice" type="warning" class="ck-feedback">{{
      appearance.notice
    }}</NAlert>
    <NAlert v-if="error" type="error" class="ck-feedback">{{ error }}</NAlert>
    <div class="ck-theme-grid" role="group" :aria-label="text('主题', 'Themes')">
      <button
        v-for="choice in choices"
        :key="choice.key"
        type="button"
        class="ck-theme-card"
        :title="choice.description || choice.label"
        :class="{ 'is-selected': selectedKey === choice.key }"
        :aria-pressed="selectedKey === choice.key"
        @click="chooseTheme(choice.key)"
      >
        <div
          class="ck-theme-preview"
          :data-design="choice.design"
          :style="cardStyle(choice)"
          aria-hidden="true"
        >
          <span class="ck-preview-nav"><i /><i /><i /></span>
          <span class="ck-preview-main"
            ><b /><span><i /><i /><i /></span><em /><em
          /></span>
          <span class="ck-preview-sessions"><i /><i /><i /><i /></span>
        </div>
        <span class="ck-theme-caption"
          ><strong>{{ choice.label }}</strong
          ><span>{{ selectedKey === choice.key ? '✓' : '↗' }}</span></span
        >
      </button>
    </div>
    <div class="ck-studio-toolbar">
      <span class="ck-studio-subtle"
        >{{ text('当前', 'Selected') }}: <strong>{{ selectedLabel }}</strong></span
      >
      <div>
        <NButton size="small" @click="importOpen = true">{{
          text('导入 JSON', 'Import JSON')
        }}</NButton
        ><NButton size="small" @click="exportTheme">{{ text('导出主题', 'Export theme') }}</NButton>
      </div>
    </div>
    <div class="ck-setting-group">
      <div class="ck-setting-label">
        <h3>{{ text('明暗模式', 'Color mode') }}</h3>
        <p>
          {{
            text(
              '跟随系统时自动响应操作系统的外观变化。',
              'System mode follows your operating system automatically.'
            )
          }}
        </p>
      </div>
      <div class="ck-segmented" role="group" :aria-label="text('明暗模式', 'Color mode')">
        <button
          v-for="option in ['light', 'dark', 'system'] as const"
          :key="option"
          type="button"
          :aria-pressed="currentMode === option"
          @click="changeMode(option)"
        >
          {{
            option === 'light'
              ? text('亮色', 'Light')
              : option === 'dark'
                ? text('暗色', 'Dark')
                : text('系统', 'System')
          }}
        </button>
      </div>
    </div>
    <NCollapse class="ck-studio-details" :default-expanded-names="['layout']">
      <NCollapseItem name="layout" :title="text('布局与阅读', 'Layout & reading')">
        <div class="ck-setting-group">
          <div class="ck-setting-label">
            <p>
              {{
                text(
                  '这些设置会立即应用；不重建当前会话、文件或终端。',
                  'Changes apply immediately without recreating sessions, files or terminals.'
                )
              }}
            </p>
          </div>
          <NAlert :show-icon="false" class="ck-feedback">{{
            text(
              '各页面保留原有导航。工作区固定为：左侧项目 · 中间主工作区 · 右侧会话；切换主题不会重置分栏宽度或关闭会话。',
              'Pages keep their original navigation. The workspace stays projects left · main area center · sessions right. Switching themes preserves panel widths and sessions.'
            )
          }}</NAlert>
          <div class="ck-form-grid">
            <label
              >{{ text('表面材质', 'Surface material')
              }}<select :value="layout.material" @change="changeLayout('material', $event)">
                <option value="solid">{{ text('实色面板', 'Solid') }}</option>
                <option value="glass">{{ text('通透玻璃', 'Glass') }}</option>
                <option value="paper">{{ text('纸张', 'Paper') }}</option>
              </select></label
            >
            <label
              >{{ text('底纹', 'Texture')
              }}<select :value="layout.texture" @change="changeLayout('texture', $event)">
                <option value="none">{{ text('无底纹', 'None') }}</option>
                <option value="grid">{{ text('工程网格', 'Grid') }}</option>
                <option value="ruled">{{ text('纸张细纹', 'Fine ruled paper') }}</option>
              </select></label
            >
            <label
              >{{ text('标题字体', 'Heading typeface')
              }}<select :value="layout.headingFont" @change="changeLayout('headingFont', $event)">
                <option value="sans">{{ text('无衬线', 'Sans serif') }}</option>
                <option value="mono">{{ text('等宽', 'Monospace') }}</option>
                <option value="serif">{{ text('书刊衬线', 'Editorial serif') }}</option>
              </select></label
            >
            <label
              >{{ text('描边粗细', 'Border weight') }} <output>{{ layout.borderWidth }} px</output
              ><input
                type="range"
                min="1"
                max="2"
                step="0.25"
                :value="layout.borderWidth"
                @input="changeLayout('borderWidth', $event, true)"
            /></label>
            <label
              >{{ text('信息密度', 'Density')
              }}<select :value="layout.density" @change="changeLayout('density', $event)">
                <option value="comfortable">{{ text('舒适', 'Comfortable') }}</option>
                <option value="compact">{{ text('紧凑', 'Compact') }}</option>
              </select></label
            >
            <label
              >{{ text('界面字体', 'UI font')
              }}<select :value="layout.font" @change="changeLayout('font', $event)">
                <option value="system">{{ text('系统无衬线', 'System sans') }}</option>
                <option value="humanist">{{ text('人文无衬线', 'Humanist sans') }}</option>
                <option value="mono">{{ text('等宽', 'Monospace') }}</option>
              </select></label
            >
            <label
              >{{ text('面板样式', 'Panel style')
              }}<select :value="layout.panelStyle" @change="changeLayout('panelStyle', $event)">
                <option value="flat">{{ text('平面', 'Flat') }}</option>
                <option value="outlined">{{ text('描边', 'Outlined') }}</option>
                <option value="elevated">{{ text('轻阴影', 'Elevated') }}</option>
              </select></label
            >
            <label
              >{{ text('字号', 'Font size') }} <output>{{ layout.fontSize }} px</output
              ><input
                type="range"
                min="12"
                max="20"
                step="1"
                :value="layout.fontSize"
                @input="changeLayout('fontSize', $event, true)"
            /></label>
            <label
              >{{ text('行高', 'Line height') }} <output>{{ layout.lineHeight }}</output
              ><input
                type="range"
                min="1.3"
                max="2"
                step="0.05"
                :value="layout.lineHeight"
                @input="changeLayout('lineHeight', $event, true)"
            /></label>
            <label
              >{{ text('圆角', 'Corner radius') }} <output>{{ layout.radius }} px</output
              ><input
                type="range"
                min="0"
                max="24"
                step="1"
                :value="layout.radius"
                @input="changeLayout('radius', $event, true)"
            /></label>
            <label
              >{{ text('间距', 'Spacing') }} <output>{{ layout.spacing }} px</output
              ><input
                type="range"
                min="8"
                max="32"
                step="2"
                :value="layout.spacing"
                @input="changeLayout('spacing', $event, true)"
            /></label>
            <label
              >{{ text('页面／阅读最大宽度', 'Page / reading width') }}
              <output>{{ layout.contentWidth }} px</output
              ><input
                type="range"
                min="640"
                max="1600"
                step="20"
                :value="layout.contentWidth"
                @input="changeLayout('contentWidth', $event, true)"
            /></label>
          </div>
          <label class="ck-toggle-row"
            ><input
              type="checkbox"
              :checked="layout.motion === 'reduced'"
              @change="
                safe(() =>
                  appearance.updateLayout({
                    motion: ($event.target as HTMLInputElement).checked ? 'reduced' : 'full',
                  })
                )
              "
            /><span>{{
              text(
                '减少动画（系统“减少动态效果”始终优先）',
                'Reduce motion (the operating system preference always takes priority)'
              )
            }}</span></label
          >
        </div>
      </NCollapseItem>
      <NCollapseItem name="colors" :title="text('颜色', 'Colors')">
        <div v-if="!appearance.isClassic" class="ck-setting-group">
          <div class="ck-setting-label">
            <p>
              {{
                text(
                  '修改当前明暗方案；另一方案独立保存。控件、浮层、会话、差异和跟随主题的终端共用这些颜色。',
                  'Edit the active variant. Controls, overlays, conversations, diffs and following terminals share these tokens.'
                )
              }}
            </p>
          </div>
          <div v-for="(group, index) in colorGroups" :key="index" class="ck-color-group">
            <h4>
              {{
                index === 0
                  ? text('界面层次', 'Surfaces & text')
                  : index === 1
                    ? text('执行与状态', 'Execution & status')
                    : text('终端', 'Terminal')
              }}
            </h4>
            <div class="ck-color-grid">
              <label v-for="key in group" :key="key" class="ck-color-field"
                ><input
                  type="color"
                  :value="colors[key]"
                  :aria-label="text(...colorNames[key])"
                  @input="setColor(key, $event)"
                /><span
                  >{{ text(...colorNames[key]) }}<code>{{ colors[key] }}</code></span
                ></label
              >
            </div>
          </div>
          <NAlert
            :type="contrast < 4.5 || mutedContrast < 4.5 ? 'warning' : 'success'"
            :show-icon="false"
          >
            {{ text('正文对比度', 'Text contrast') }} {{ contrast.toFixed(2) }}:1 ·
            {{ text('次级文字', 'Secondary text') }} {{ mutedContrast.toFixed(2) }}:1.
            {{
              text(
                '普通文本建议至少 4.5:1；这里仅检查文字与面板底色，不代表整页无障碍认证。',
                'Aim for at least 4.5:1 for normal text. This checks text against the surface only, not whole-page accessibility.'
              )
            }}
          </NAlert>
        </div>
        <div v-else class="ck-setting-group">
          <slot name="legacy-colors" />
        </div>
      </NCollapseItem>
    </NCollapse>
    <div class="ck-studio-footer">
      <div class="ck-studio-actions">
        <NButton @click="resetCurrentCustomization">{{
          text('重置当前自定义项', 'Reset current overrides')
        }}</NButton
        ><NButton @click="exportPreferences">{{
          text('备份全部外观', 'Back up all appearance settings')
        }}</NButton
        ><NButton
          v-if="appearance.preferences.customThemes.some(t => t.id === appearance.themeId)"
          type="error"
          secondary
          @click="removeTheme(appearance.themeId)"
          >{{ text('移除此主题', 'Remove theme') }}</NButton
        >
      </div>
      <p>
        {{
          text(
            'JSON 只接受受校验的颜色与布局数据，不加载远程资源，不运行脚本或任意 CSS。开发者可通过可信编译期插件扩展界面插槽。',
            'JSON contains validated colors and layout data only: no remote resources, scripts or arbitrary CSS. Developers can extend UI slots through trusted build-time plugins.'
          )
        }}
      </p>
      <AppearanceSlot name="settings.footer" />
    </div>
    <NModal
      v-model:show="importOpen"
      preset="card"
      class="ck-import-modal"
      :title="text('导入外观', 'Import appearance')"
      :style="{ width: '640px', maxWidth: 'calc(100vw - 32px)' }"
      :auto-focus="true"
    >
      <div class="ck-import-body">
        <p>
          {{
            text(
              '主题文件最大 64 KiB；外观备份最大 2 MiB。先校验预览，再明确应用。',
              'Theme files: up to 64 KiB. Appearance backups: up to 2 MiB. Validate first, then explicitly apply.'
            )
          }}
        </p>
        <input
          ref="fileInput"
          type="file"
          accept=".json,application/json"
          hidden
          @change="readFile"
        />
        <NButton @click="fileInput?.click()">{{
          text('选择 JSON 文件', 'Choose JSON file')
        }}</NButton>
        <label class="ck-json-label"
          >{{ text('或粘贴 JSON', 'Or paste JSON')
          }}<textarea
            v-model="importText"
            rows="9"
            spellcheck="false"
            placeholder='{ "schemaVersion": 1, "id": "my-theme", ... }'
          />
        </label>
        <label class="ck-toggle-row"
          ><input v-model="replaceExisting" type="checkbox" />{{
            text(
              '明确允许替换同 ID 的已导入主题（不能替换内置主题）',
              'Explicitly allow replacing an imported theme with the same ID (not built-ins)'
            )
          }}</label
        >
        <NAlert v-if="importError" type="error">{{ importError }}</NAlert>
        <div v-if="preview" class="ck-import-previews">
          <section
            v-for="variant in previewVariants"
            :key="variant.scheme"
            :style="{
              background: variant.palette.canvas,
              color: variant.palette.text,
              borderColor: variant.palette.border,
              borderRadius: `${variant.layout.radius}px`,
            }"
          >
            <small :style="{ color: variant.palette.muted }"
              >{{ variant.scheme }} · {{ preview.themeId }}</small
            >
            <div
              :style="{
                background: variant.palette.surface,
                borderColor: variant.palette.border,
                borderRadius: `${variant.layout.radius}px`,
              }"
            >
              <strong>{{ text('你的工作区', 'Your workspace') }}</strong>
              <p :style="{ color: variant.palette.muted }">
                {{ text('会话、文件与下一步。', 'Conversations, files and what comes next.') }}
              </p>
              <span :style="{ color: variant.palette.accent }"
                >{{ text('强调色与链接', 'Accent & links') }} →</span
              >
            </div>
          </section>
        </div>
        <NAlert v-if="preview" :type="isBundle ? 'warning' : 'success'">
          <strong>{{ text('校验通过', 'Validation passed') }} · {{ preview.themeId }}</strong
          ><br />
          {{
            isBundle
              ? text(
                  '这是完整外观备份。应用将替换所有外观偏好和已导入主题，但不修改项目、会话或原始设置。',
                  'This is a complete appearance backup. Applying replaces all appearance preferences and imported themes, but not projects, sessions or original settings.'
                )
              : text(
                  '主题结构、颜色、数值范围及完整继承链均有效。应用后切换到此主题。',
                  'The theme structure, colors, ranges and complete inheritance chain are valid. Applying switches to this theme.'
                )
          }}
        </NAlert>
      </div>
      <template #footer
        ><div class="ck-studio-actions">
          <NButton @click="importOpen = false">{{ text('取消', 'Cancel') }}</NButton
          ><NButton :disabled="!importText.trim()" @click="inspectImport">{{
            text('校验预览', 'Validate & preview')
          }}</NButton
          ><NButton type="primary" :disabled="!preview" @click="confirmImport">{{
            text('确认应用', 'Apply explicitly')
          }}</NButton>
        </div></template
      >
    </NModal>
  </section>
</template>
