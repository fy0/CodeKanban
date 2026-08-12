import { DEFAULT_PRESET_ID } from '@/constants/themes';

export interface ThemeMaintenanceDialogOptions {
  title: string;
  content: string;
  positiveText: string;
  negativeText: string;
  maskClosable?: boolean;
  closeOnEsc?: boolean;
  onPositiveClick?: () => boolean | void | Promise<boolean | void>;
  onNegativeClick?: () => void;
  onClose?: () => void;
}

type ThemeMaintenanceTranslator = (key: string, ...args: unknown[]) => string;
type ThemeMaintenanceWarningDialog = (options: ThemeMaintenanceDialogOptions) => void;

interface ThemeMaintenanceWarningControllerOptions {
  t: ThemeMaintenanceTranslator;
  warning: ThemeMaintenanceWarningDialog;
}

interface ThemeSelectionControllerOptions {
  getCurrentPresetId: () => string;
  isFollowSystemTheme: () => boolean;
  selectPreset: (presetId: string) => void;
  toggleFollowSystemTheme: (enabled: boolean) => void;
  confirmPresetThemeChange: () => Promise<boolean>;
  confirmFollowSystemEnable: () => Promise<boolean>;
  shouldConfirmPresetThemeChange?: (presetId: string) => boolean;
  shouldConfirmFollowSystemEnable?: () => boolean;
}

function createThemeMaintenanceWarningPromise(
  warning: ThemeMaintenanceWarningDialog,
  options: ThemeMaintenanceDialogOptions
) {
  return new Promise<boolean>(resolve => {
    let settled = false;
    const resolveOnce = (value: boolean) => {
      if (settled) {
        return;
      }
      settled = true;
      resolve(value);
    };

    warning({
      ...options,
      maskClosable: false,
      closeOnEsc: true,
      onPositiveClick: () => {
        resolveOnce(true);
        return true;
      },
      onNegativeClick: () => resolveOnce(false),
      onClose: () => resolveOnce(false),
    });
  });
}

export function createThemeMaintenanceWarningController({
  t,
  warning,
}: ThemeMaintenanceWarningControllerOptions) {
  return {
    confirmPresetThemeChange: () =>
      createThemeMaintenanceWarningPromise(warning, {
        title: t('theme.maintenanceWarningTitle'),
        content: t('theme.maintenanceWarningPreset'),
        positiveText: t('common.confirm'),
        negativeText: t('common.cancel'),
      }),
    confirmFollowSystemEnable: () =>
      createThemeMaintenanceWarningPromise(warning, {
        title: t('theme.maintenanceWarningTitle'),
        content: t('theme.maintenanceWarningFollowSystem'),
        positiveText: t('common.confirm'),
        negativeText: t('common.cancel'),
      }),
  };
}

export function createThemeSelectionController({
  getCurrentPresetId,
  isFollowSystemTheme,
  selectPreset,
  toggleFollowSystemTheme,
  confirmPresetThemeChange,
  confirmFollowSystemEnable,
  shouldConfirmPresetThemeChange = presetId => presetId !== DEFAULT_PRESET_ID,
  shouldConfirmFollowSystemEnable = () => true,
}: ThemeSelectionControllerOptions) {
  async function selectPresetWithConfirmation(presetId: string) {
    if (!presetId) {
      return false;
    }
    if (presetId === getCurrentPresetId() && !isFollowSystemTheme()) {
      return false;
    }
    if (shouldConfirmPresetThemeChange(presetId) && !(await confirmPresetThemeChange())) {
      return false;
    }
    selectPreset(presetId);
    return true;
  }

  async function toggleFollowSystemThemeWithConfirmation(enabled: boolean) {
    if (enabled === isFollowSystemTheme()) {
      return false;
    }
    if (!enabled) {
      toggleFollowSystemTheme(false);
      return true;
    }
    if (shouldConfirmFollowSystemEnable() && !(await confirmFollowSystemEnable())) {
      return false;
    }
    toggleFollowSystemTheme(true);
    return true;
  }

  async function quickToggleLightDark() {
    const currentPresetId = getCurrentPresetId();
    if (currentPresetId === 'dark') {
      return selectPresetWithConfirmation('light');
    }
    if (currentPresetId === 'light') {
      return selectPresetWithConfirmation('dark');
    }
    return false;
  }

  return {
    selectPresetWithConfirmation,
    toggleFollowSystemThemeWithConfirmation,
    quickToggleLightDark,
  };
}
