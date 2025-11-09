<script setup lang="ts">
import { useTitle } from '@vueuse/core';
import { APP_NAME } from '@/constants/app';

useTitle(`关于 - ${APP_NAME}`);
</script>

<template>
  <div class="about">
    <h1>This is an about page</h1>
  </div>
</template>

<style>
@media (min-width: 1024px) {
  .about {
    min-height: 100vh;
    display: flex;
    align-items: center;
  }
}
</style>
