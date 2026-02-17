<script setup lang="ts">
import { RouterView } from "vue-router";
import AppNav from "./components/AppNav.vue";
</script>

<template>
  <AppNav>
    <main id="main-content" class="app-main">
      <div class="page page--wide">
        <RouterView />
      </div>
    </main>
  </AppNav>
</template>
