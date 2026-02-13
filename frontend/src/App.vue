<script setup lang="ts">
import { RouterView } from "vue-router";
import AppNav from "./components/AppNav.vue";
</script>

<template>
  <div class="min-h-screen text-slate-900">
    <AppNav />
    <main class="app-container app-main">
      <RouterView />
    </main>
  </div>
</template>
