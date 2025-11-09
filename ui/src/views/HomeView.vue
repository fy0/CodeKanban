<script setup lang="ts">
import { useTitle } from '@vueuse/core';
import TheWelcome from '../components/TheWelcome.vue';
import { APP_NAME } from '@/constants/app';

useTitle(`欢迎 - ${APP_NAME}`);
</script>

<template>
  <main>
    <TheWelcome />
  </main>
</template>
