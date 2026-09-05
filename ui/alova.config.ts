import { defineConfig } from '@alova/wormhole';

export default defineConfig({
  generator: [
    {
      input: 'http://localhost:3005/openapi.json',
      platform: 'swagger',
      output: 'src/api',
      bodyMediaType: 'application/json',
      global: 'Apis',
      handleApi: apiDescriptor => {
        const operationId = apiDescriptor.operationId;
        const tag = apiDescriptor.tags?.[0] || '';
        if (operationId && tag && operationId.toLowerCase().startsWith(tag.toLowerCase())) {
          const withoutPrefix = operationId.substring(tag.length);
          apiDescriptor.operationId =
            withoutPrefix.charAt(0).toLowerCase() + withoutPrefix.slice(1);
        }
        return apiDescriptor;
      },
    },
  ],
});
