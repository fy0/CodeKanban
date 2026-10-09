<template>
  <!-- Keep the route subtree stable across themes and use each page's original navigation. -->
  <div class="ck-shell">
    <main id="ck-main" class="ck-shell-content">
      <slot />
    </main>
  </div>
</template>
