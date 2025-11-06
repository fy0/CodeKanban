import { defineConfig } from '@alova/wormhole';

// For more config detailed visit:
// https://alova.js.org/tutorial/getting-started/extension-integration

export default defineConfig({
  generator: [
    {
      /**
       * file input. support:
       * 1. openapi json file url
       * 2. local file
       */
      input: 'http://localhost:3005/openapi.json',

      /**
       * input file platform. Currently only swagger is supported.
       * When this parameter is specified, the input field only needs to specify the document address without specifying the openapi file
       */
      platform: 'swagger',

      /**
       * output path of interface file and type file.
       * Multiple generators cannot have the same address, otherwise the generated code will overwrite each other.
       */
      output: 'src/api',

      /**
       * the mediaType of the generated response data. default is `application/json`
       */
      // responseMediaType: 'application/json',

      /**
       * the bodyMediaType of the generated request body data. default is `application/json`
       */
      bodyMediaType: 'application/json',

      /**
       * the generated api version. options are `2` or `3`, default is `auto`.
       */
      // version: 'auto',

      /**
       * type of generated code. The options are `auto/ts/typescript/module/commonjs`
       */
      // type: 'auto',

      /**
       * exported global api name, you can access the generated api globally through this name, default is `Apis`.
       * it is required when multiple generators are configured, and it cannot be repeated
       */
      global: 'Apis',

      /**
       * filter or convert the generated api information, return an apiDescriptor, if this function is not specified, the apiDescriptor object is not converted
       */
      handleApi: apiDescriptor => {
        // 去除重复的前缀，例如将 attachments2Delete 改为 delete
        if (apiDescriptor.operationId) {
          const tag = apiDescriptor.tags?.[0] || '';
          const operationId = apiDescriptor.operationId;

          // 如果 operationId 以 tag 开头，则去除这个前缀
          if (tag && operationId.toLowerCase().startsWith(tag.toLowerCase())) {
            // 去除前缀并将首字母小写
            const withoutPrefix = operationId.substring(tag.length);
            apiDescriptor.operationId = withoutPrefix.charAt(0).toLowerCase() + withoutPrefix.slice(1);
          }
        }

        // NOTE: 原始数据有问题，已经从后端修复，留档一个版本后删除
        // if (apiDescriptor.responses?.properties) {
        //   // console.log('info', JSON.stringify(apiDescriptor, null, 2));
        //   fixPropertiesEmptyObjects(apiDescriptor.responses.properties);
        // }

        return apiDescriptor;
      }
    }
  ]

  /**
   * extension only
   * whether to automatically update the interface, enabled by default, check every 5 minutes, closed when set to `false`
   */
  // autoUpdate: true
});
