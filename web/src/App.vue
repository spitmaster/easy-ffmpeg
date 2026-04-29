<script setup lang="ts">
import { RouterView } from 'vue-router'
import TopBar from './components/layout/TopBar.vue'
import TabNav from './components/layout/TabNav.vue'
</script>

<template>
  <div class="flex h-full flex-col">
    <TopBar />
    <TabNav />
    <main class="flex-1 overflow-auto bg-bg-base">
      <RouterView />
    </main>
  </div>
</template>
