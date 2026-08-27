import { globalIgnores } from 'eslint/config';
import { defineConfigWithVueTs, vueTsConfigs } from '@vue/eslint-config-typescript';
import pluginVue from 'eslint-plugin-vue';
import pluginOxlint from 'eslint-plugin-oxlint';
import skipFormatting from '@vue/eslint-config-prettier/skip-formatting';

const generatedFiles = [
  'src/api/apiDefinitions.ts',
  'src/api/createApis.ts',
  'src/api/globals.d.ts',
];

export default defineConfigWithVueTs(
  globalIgnores(['**/dist*/**', '**/coverage/**', ...generatedFiles]),

  {
    name: 'app/files-to-lint',
    files: ['**/*.{ts,mts,tsx,vue}'],
  },

  pluginVue.configs['flat/essential'],
  vueTsConfigs.recommended,

  {
    name: 'app/project-rules',
    linterOptions: {
      reportUnusedDisableDirectives: 'error',
    },
    rules: {
      '@typescript-eslint/no-explicit-any': 'error',
    },
  },

  skipFormatting,

  // Keep this last so ESLint does not repeat rules enabled in Oxlint.
  ...pluginOxlint.buildFromOxlintConfigFile('./.oxlintrc.json')
);
