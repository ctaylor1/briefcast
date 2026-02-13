<script setup lang="ts">
import { ref, watch } from "vue";
import { RouterLink, useRoute } from "vue-router";
import UiButton from "./ui/UiButton.vue";

const route = useRoute();
const isMenuOpen = ref(false);

const links = [
  { to: "/", label: "Podcasts" },
  { to: "/episodes", label: "Episodes" },
  { to: "/add", label: "Add Podcast" },
];

const isActive = (target: string) => {
  if (target === "/") {
    return route.path === "/";
  }
  return route.path.startsWith(target);
};

watch(
  () => route.path,
  () => {
    isMenuOpen.value = false;
  },
);
</script>

<template>
  <header class="sticky top-0 z-40 border-b border-slate-200 bg-white/95 backdrop-blur supports-[backdrop-filter]:bg-white/80">
    <div class="app-container flex min-h-[var(--nav-height)] items-center justify-between gap-3 py-[var(--space-2)]">
      <div class="flex items-center gap-3">
        <div class="h-9 w-9 rounded-md bg-gradient-to-br from-cyan-500 to-blue-600"></div>
        <div>
          <p class="fluid-subtle font-semibold uppercase tracking-[0.16em] text-slate-500">Briefcast</p>
          <p class="text-sm font-semibold text-slate-900 sm:text-base">Modern UI</p>
        </div>
      </div>

      <UiButton class="md:hidden" size="sm" variant="outline" @click="isMenuOpen = !isMenuOpen">
        {{ isMenuOpen ? "Close" : "Menu" }}
      </UiButton>

      <nav class="hidden items-center gap-2 md:flex">
        <RouterLink
          v-for="link in links"
          :key="link.to"
          :to="link.to"
          class="rounded-md px-3 py-2 text-sm font-medium transition-colors"
          :class="
            isActive(link.to)
              ? 'bg-slate-900 text-white'
              : 'text-slate-700 hover:bg-slate-100 hover:text-slate-900'
          "
        >
          {{ link.label }}
        </RouterLink>
        <a
          href="/"
          class="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-100"
        >
          Legacy UI
        </a>
      </nav>
    </div>

    <Transition
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="-translate-y-2 opacity-0"
      enter-to-class="translate-y-0 opacity-100"
      leave-active-class="transition duration-150 ease-in"
      leave-from-class="translate-y-0 opacity-100"
      leave-to-class="-translate-y-2 opacity-0"
    >
      <nav v-if="isMenuOpen" class="border-t border-slate-200 bg-white md:hidden">
        <div class="app-container py-[var(--space-3)]">
          <div class="grid grid-cols-1 gap-2">
            <RouterLink
              v-for="link in links"
              :key="link.to"
              :to="link.to"
              class="rounded-lg px-3 py-2.5 text-sm font-medium transition-colors"
              :class="
                isActive(link.to)
                  ? 'bg-slate-900 text-white'
                  : 'text-slate-700 hover:bg-slate-100 hover:text-slate-900'
              "
            >
              {{ link.label }}
            </RouterLink>
            <a
              href="/"
              class="rounded-lg border border-slate-300 px-3 py-2.5 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-100"
            >
              Legacy UI
            </a>
          </div>
        </div>
      </nav>
    </Transition>
  </header>

  <Transition
    enter-active-class="transition duration-200 ease-out"
    enter-from-class="opacity-0"
    enter-to-class="opacity-100"
    leave-active-class="transition duration-150 ease-in"
    leave-from-class="opacity-100"
    leave-to-class="opacity-0"
  >
    <button
      v-if="isMenuOpen"
      type="button"
      class="fixed inset-0 z-30 bg-slate-900/25 md:hidden"
      aria-label="Close menu"
      @click="isMenuOpen = false"
    />
  </Transition>
</template>
