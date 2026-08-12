<template>
  <div class="login-page">
    <div class="login-page__glow login-page__glow--a"></div>
    <div class="login-page__glow login-page__glow--b"></div>
    <n-card class="login-card" :bordered="false">
      <template #header>
        <div class="login-card__header">
          <div class="login-card__eyebrow">{{ t('auth.loginEyebrow') }}</div>
          <h1 class="login-card__title">{{ t('auth.loginTitle') }}</h1>
          <p class="login-card__subtitle">{{ t('auth.loginSubtitle') }}</p>
        </div>
      </template>

      <n-form @submit.prevent="handleSubmit">
        <n-form-item :label="t('auth.passwordLabel')">
          <n-input
            v-model:value="password"
            type="password"
            show-password-on="click"
            :placeholder="t('auth.passwordPlaceholder')"
            :disabled="submitting"
            @keydown.enter.prevent="handleSubmit"
          />
        </n-form-item>

        <n-space vertical size="small">
          <n-button
            block
            type="primary"
            size="large"
            :loading="submitting"
            :disabled="!password.trim()"
            @click="handleSubmit"
          >
            {{ t('auth.loginAction') }}
          </n-button>
          <div class="login-card__hint">{{ t('auth.loginHashHint') }}</div>
        </n-space>
      </n-form>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useMessage } from 'naive-ui';
import { useLocale } from '@/composables/useLocale';
import { useAuthStore } from '@/stores/auth';

const router = useRouter();
const route = useRoute();
const message = useMessage();
const authStore = useAuthStore();
const { t } = useLocale();

const password = ref('');
const submitting = ref(false);

const redirectTarget = computed(() => {
  const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/';
  return redirect.startsWith('/') ? redirect : '/';
});

async function handleSubmit() {
  if (submitting.value || !password.value.trim()) {
    return;
  }

  submitting.value = true;
  try {
    await authStore.loginWithPassword(password.value);
    password.value = '';
    await router.replace(redirectTarget.value);
  } catch (error) {
    const detail = error instanceof Error ? error.message : t('auth.loginFailed');
    message.error(detail);
  } finally {
    submitting.value = false;
  }
}
</script>

<style scoped>
.login-page {
  position: relative;
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 32px 20px;
  overflow: hidden;
  color: var(--app-text-primary);
  background:
    radial-gradient(
      circle at top left,
      color-mix(in srgb, var(--app-accent) 14%, transparent),
      transparent 34%
    ),
    radial-gradient(
      circle at bottom right,
      color-mix(in srgb, var(--app-accent) 10%, transparent),
      transparent 30%
    ),
    linear-gradient(
      145deg,
      color-mix(in srgb, var(--app-canvas) 82%, var(--app-surface-raised)) 0%,
      var(--app-canvas) 100%
    );
}

.login-page__glow {
  position: absolute;
  width: 320px;
  height: 320px;
  border-radius: 999px;
  filter: blur(56px);
  opacity: 0.34;
  pointer-events: none;
}

.login-page__glow--a {
  top: -72px;
  left: -64px;
  background: color-mix(in srgb, var(--app-accent) 24%, transparent);
}

.login-page__glow--b {
  right: -48px;
  bottom: -88px;
  background: color-mix(in srgb, var(--app-accent) 18%, transparent);
}

.login-card {
  position: relative;
  z-index: 1;
  width: min(100%, 440px);
  border-radius: 24px;
  border: 1px solid var(--app-border);
  background: color-mix(in srgb, var(--app-surface-raised) 92%, var(--app-canvas));
  box-shadow: 0 24px 72px var(--app-shadow);
}

.login-card__header {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.login-card__eyebrow {
  font-size: 12px;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--app-link);
}

.login-card__title {
  margin: 0;
  font-size: clamp(26px, 5vw, 34px);
  line-height: 1.05;
  color: var(--app-text-primary);
}

.login-card__subtitle {
  margin: 0;
  color: var(--app-text-secondary);
  line-height: 1.6;
}

.login-card__hint {
  font-size: 12px;
  line-height: 1.6;
  color: var(--app-text-muted);
}

.login-card :deep(.n-form-item-label) {
  color: var(--app-text-secondary);
}

.login-card :deep(.n-input) {
  --n-color: var(--app-surface-sunken) !important;
  --n-color-disabled: var(--app-surface-hover) !important;
  --n-text-color: var(--app-text-primary) !important;
  --n-text-color-disabled: var(--app-text-muted) !important;
  --n-placeholder-color: var(--app-text-muted) !important;
  --n-placeholder-color-disabled: var(--app-text-muted) !important;
  --n-border: 1px solid var(--app-input-border-color) !important;
  --n-border-hover: 1px solid var(--app-input-border-hover-color) !important;
  --n-border-focus: 1px solid var(--app-focus-ring) !important;
  --n-box-shadow-focus: 0 0 0 2px color-mix(in srgb, var(--app-focus-ring) 24%, transparent) !important;
  --n-caret-color: var(--app-accent) !important;
}

.login-card :deep(.n-button.n-button--primary-type) {
  --n-color: var(--app-accent) !important;
  --n-color-hover: var(--app-accent-hover) !important;
  --n-color-pressed: var(--app-accent-pressed) !important;
  --n-color-focus: var(--app-accent-hover) !important;
  --n-text-color: var(--app-accent-contrast) !important;
  --n-text-color-hover: var(--app-accent-contrast) !important;
  --n-text-color-pressed: var(--app-accent-contrast) !important;
  --n-text-color-focus: var(--app-accent-contrast) !important;
  --n-border: 1px solid var(--app-accent) !important;
  --n-border-hover: 1px solid var(--app-accent-hover) !important;
  --n-border-pressed: 1px solid var(--app-accent-pressed) !important;
  --n-border-focus: 1px solid var(--app-focus-ring) !important;
  --n-box-shadow-focus: 0 0 0 2px color-mix(in srgb, var(--app-focus-ring) 24%, transparent) !important;
}
</style>
