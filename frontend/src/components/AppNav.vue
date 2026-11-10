<script setup lang="ts">
import { episodePath, podcastPath } from "../lib/routes";
import {
  Combobox,
  ComboboxInput,
  ComboboxOption,
  ComboboxOptions,
  Dialog,
  DialogPanel,
  DialogTitle,
  TransitionChild,
  TransitionRoot,
} from "@headlessui/vue";
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import { useDebouncedWatch } from "../composables/useDebouncedWatch";
import { useGlobalSearch } from "../composables/useGlobalSearch";
import { useTheme } from "../composables/useTheme";
import { activeNavItem, commandNavItems, librarySubItems, primaryNavItems } from "../lib/navigation";
import type { LocalSearchResult } from "../types/api";
import UiIcon from "./ui/UiIcon.vue";
import UiIconButton from "./ui/UiIconButton.vue";
import briefcastLogo from "../assets/briefcast-logo.svg";

interface CommandRouteItem {
  kind: "route";
  key: string;
  label: string;
  section: string;
  meta: string;
  to: string;
}

interface CommandLocalItem {
  kind: "local";
  key: string;
  label: string;
  section: string;
  meta: string;
  result: LocalSearchResult;
}

type CommandItem = CommandRouteItem | CommandLocalItem;

const route = useRoute();
const router = useRouter();
const { cycleTheme, themeModeLabel, initTheme, themeMode } = useTheme();


const viewportWidth = ref(typeof window === "undefined" ? 1280 : window.innerWidth);
const sidebarExpanded = ref(false);
const commandOpen = ref(false);
const commandQuery = ref("");
const commandTriggerRef = ref<HTMLButtonElement | null>(null);
const commandInputRef = ref<HTMLInputElement | null>(null);
const lastCommandInvokerRef = ref<HTMLElement | null>(null);
const {
  query: globalSearchQuery,
  results: globalSearchResults,
  loading: globalSearchLoading,
  error: globalSearchError,
  run: runGlobalSearch,
  typeLabel: localResultTypeLabel,
  summary: localResultSummary,
} = useGlobalSearch(50);

const isDesktop = computed(() => viewportWidth.value >= 1024);
const showSidebar = computed(() => viewportWidth.value >= 768);
const sidebarExpandedState = computed(() => isDesktop.value || sidebarExpanded.value);

// The page heading for screen readers; each page shows its own visible title.
const topTitle = computed(() => {
  if (typeof route.meta.title === "string" && route.meta.title.trim()) {
    return route.meta.title.trim();
  }
  return "Briefcast";
});

const routeCommandItems = computed<CommandRouteItem[]>(() => {
  const query = commandQuery.value.trim().toLowerCase();
  return commandNavItems
    .filter((item) => {
      const haystack = `${item.label} ${item.meta} ${item.keywords}`.toLowerCase();
      return !query || haystack.includes(query);
    })
    .map((item) => ({
      kind: "route",
      key: `route:${item.to}`,
      label: item.label,
      section: "Go to",
      meta: item.meta,
      to: item.to,
    }));
});

const localCommandItems = computed<CommandLocalItem[]>(() =>
  globalSearchResults.value.map((result, index) => ({
    kind: "local",
    key: `local:${result.type}:${result.episodeId || result.podcastId || index}:${index}`,
    label: localResultLabel(result),
    section: localResultTypeLabel(result),
    meta: localResultSummary(result) || result.podcastTitle || "Library match",
    result,
  })),
);

const commandItems = computed<CommandItem[]>(() => {
  const query = commandQuery.value.trim();
  if (!query) {
    return routeCommandItems.value;
  }
  return [...routeCommandItems.value, ...localCommandItems.value];
});

const themeIcon = computed(() => (themeMode.value === "dark" ? "moon" : themeMode.value === "light" ? "sun" : "monitor"));

const commandHint = computed(() => (isMac() ? "Cmd+K" : "Ctrl+K"));
const bottomTabsStyle = computed(() => ({
  "--app-bottom-tabs-count": String(primaryNavItems.length),
}));

function isMac(): boolean {
  if (typeof navigator === "undefined") {
    return false;
  }
  return /Mac|iPhone|iPad/.test(navigator.platform);
}

function isActive(target: string): boolean {
  return activeNavItem(route.path)?.to === target;
}

function refreshViewport(): void {
  viewportWidth.value = typeof window === "undefined" ? 1280 : window.innerWidth;
  if (viewportWidth.value >= 1024) {
    sidebarExpanded.value = true;
  }
  if (viewportWidth.value < 768) {
    sidebarExpanded.value = false;
  }
}

function restoreSidebarPreference(): void {
  if (typeof window === "undefined") {
    return;
  }
  const stored = window.localStorage.getItem("briefcast.sidebar.expanded");
  if (stored === "true") {
    sidebarExpanded.value = true;
  }
}

function persistSidebarPreference(value: boolean): void {
  if (typeof window === "undefined") {
    return;
  }
  window.localStorage.setItem("briefcast.sidebar.expanded", String(value));
}

function toggleSidebar(): void {
  sidebarExpanded.value = !sidebarExpanded.value;
}

function resolveCommandInvoker(target?: EventTarget | null): HTMLElement | null {
  const normalize = (element: HTMLElement | null): HTMLElement | null => {
    if (!element) {
      return null;
    }
    if (element === document.body || element === document.documentElement) {
      return null;
    }
    return element;
  };

  if (target instanceof HTMLElement) {
    return normalize(target);
  }
  if (typeof document === "undefined") {
    return null;
  }
  return document.activeElement instanceof HTMLElement ? normalize(document.activeElement) : null;
}

function restoreCommandFocus(): void {
  if (typeof document === "undefined") {
    return;
  }
  const target = lastCommandInvokerRef.value ?? commandTriggerRef.value;
  if (target && document.contains(target)) {
    target.focus();
    lastCommandInvokerRef.value = null;
    return;
  }
  commandTriggerRef.value?.focus();
  lastCommandInvokerRef.value = null;
}

function openCommandPalette(target?: EventTarget | null): void {
  lastCommandInvokerRef.value = resolveCommandInvoker(target);
  commandOpen.value = true;
}

function closeCommandPalette(): void {
  const wasOpen = commandOpen.value;
  commandOpen.value = false;
  if (!wasOpen && commandQuery.value.trim() === "") {
    return;
  }
  commandQuery.value = "";
  globalSearchQuery.value = "";
  void runGlobalSearch();
  if (wasOpen && typeof window !== "undefined") {
    window.setTimeout(() => {
      restoreCommandFocus();
    }, 0);
    window.setTimeout(() => {
      restoreCommandFocus();
    }, 180);
  }
}

function localResultLabel(result: LocalSearchResult): string {
  switch (result.type) {
    case "podcast":
      return result.podcastTitle || "Podcast match";
    case "episode":
      return result.episodeTitle || "Episode match";
    case "chapter":
      return result.chapterTitle || result.episodeTitle || "Chapter match";
    case "transcript":
      return result.episodeTitle || result.podcastTitle || "Transcript match";
    case "summary":
      return result.episodeTitle || result.podcastTitle || "Summary match";
    default:
      return "Search result";
  }
}

function localResultRoute(result: LocalSearchResult): { path: string; query?: Record<string, string> } {
  if (result.type === "podcast" && result.podcastId) {
    return { path: podcastPath(result.podcastId) };
  }

  if (result.episodeId) {
    const tab =
      result.type === "transcript" ? "transcript"
      : result.type === "chapter" ? "chapters"
      : result.type === "summary" ? "summary"
      : undefined;
    return { path: episodePath(result.episodeId), query: tab ? { tab } : undefined };
  }

  const q = ((): string => {
    if (result.type === "episode") {
      return result.episodeTitle || "";
    }
    if (result.type === "chapter") {
      return result.chapterTitle || result.episodeTitle || "";
    }
    if (result.type === "transcript") {
      return result.episodeTitle || result.transcriptSnippet || "";
    }
    if (result.type === "summary") {
      return result.episodeTitle || "";
    }
    return result.podcastTitle || "";
  })();

  if (result.type === "summary" && q) {
    return {
      path: "/summaries",
      query: { q },
    };
  }

  if (q) {
    return {
      path: "/episodes",
      query: { q },
    };
  }

  if (result.podcastId) {
    return {
      path: "/episodes",
      query: { podcastIds: result.podcastId },
    };
  }

  return { path: "/episodes" };
}

function pillClass(item: CommandItem): string {
  if (item.kind !== "local") {
    return "";
  }
  switch (item.result.type) {
    case "podcast":
      return "command-pill command-pill--podcast";
    case "episode":
      return "command-pill command-pill--episode";
    case "chapter":
      return "command-pill command-pill--chapter";
    case "transcript":
      return "command-pill command-pill--transcript";
    case "summary":
      return "command-pill command-pill--summary";
    default:
      return "command-pill";
  }
}

function selectCommand(item: CommandItem): void {
  closeCommandPalette();
  if (item.kind === "route") {
    void router.push(item.to);
    return;
  }
  void router.push(localResultRoute(item.result));
}

function handleWindowKeydown(event: KeyboardEvent): void {
  const wantsPalette = (event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k";
  if (wantsPalette) {
    event.preventDefault();
    if (commandOpen.value) {
      closeCommandPalette();
      return;
    }
    openCommandPalette(event.target);
    return;
  }
  if (event.key === "Escape" && commandOpen.value) {
    closeCommandPalette();
  }
}

watch(sidebarExpanded, (value) => {
  persistSidebarPreference(value);
});

watch(
  () => route.fullPath,
  () => {
    closeCommandPalette();
  },
);

watch(commandOpen, async (open) => {
  if (!open) {
    return;
  }
  await nextTick();
  commandInputRef.value?.focus();
  commandInputRef.value?.select();
});

useDebouncedWatch(
  () => commandQuery.value,
  () => {
    if (!commandOpen.value) {
      return;
    }
    globalSearchQuery.value = commandQuery.value;
    void runGlobalSearch();
  },
  250,
);

onMounted(() => {
  restoreSidebarPreference();
  refreshViewport();
  initTheme();
  window.addEventListener("resize", refreshViewport);
  window.addEventListener("keydown", handleWindowKeydown);
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", refreshViewport);
  window.removeEventListener("keydown", handleWindowKeydown);
});
</script>

<template>
  <a class="skip-link" href="#main-content">Skip to main content</a>

  <div class="app-layout">
    <aside
      v-if="showSidebar"
      class="app-sidebar"
      :data-expanded="sidebarExpandedState ? 'true' : undefined"
    >
      <div class="app-sidebar__brand">
        <img class="app-sidebar__logo" :src="briefcastLogo" alt="Briefcast logo" />
      </div>

      <nav class="app-nav" aria-label="Main">
        <template v-for="item in primaryNavItems" :key="item.to">
          <RouterLink
            :to="item.to"
            class="app-nav__link"
            :aria-current="isActive(item.to) ? 'page' : undefined"
            :title="sidebarExpandedState ? undefined : item.label"
          >
            <UiIcon :name="item.icon" class="app-nav__icon" />
            <span class="app-nav__label">{{ item.label }}</span>
          </RouterLink>
          <template v-if="item.to === '/'">
            <RouterLink
              v-for="sub in librarySubItems"
              :key="sub.to"
              :to="sub.to"
              class="app-nav__link app-nav__link--sub"
              :aria-current="isActive(sub.to) ? 'page' : undefined"
              :title="sidebarExpandedState ? undefined : sub.label"
            >
              <UiIcon :name="sub.icon" class="app-nav__icon" />
              <span class="app-nav__label">{{ sub.label }}</span>
            </RouterLink>
          </template>
        </template>
      </nav>

      <button
        v-if="!isDesktop"
        type="button"
        class="app-sidebar__toggle"
        @click="toggleSidebar"
      >
        <UiIcon name="menu" class="app-nav__icon" />
        <span class="app-nav__label">{{ sidebarExpandedState ? "Collapse" : "Expand" }}</span>
      </button>
    </aside>

    <div class="app-main-wrap">
      <header class="app-topbar">
        <div class="app-topbar__left">
          <h1 class="sr-only">{{ topTitle }}</h1>
          <img v-if="!showSidebar" class="app-topbar__logo" :src="briefcastLogo" alt="Briefcast" />
        </div>
        <div class="app-topbar__actions">
          <button
            ref="commandTriggerRef"
            type="button"
            class="command-kbd app-command-trigger"
            :aria-label="`Search and go to (${commandHint})`"
            @click="openCommandPalette($event.currentTarget)"
          >
            <UiIcon name="search" class="app-nav__icon" />
            <span>Search</span>
            <span class="meta-text">{{ commandHint }}</span>
          </button>
          <RouterLink to="/add" class="ui-button ui-button--ghost ui-button--sm app-topbar__add" title="Add podcast">
            <UiIcon name="plus" />
            <span class="app-topbar__add-label">Add podcast</span>
          </RouterLink>
          <UiIconButton :label="`Theme: ${themeModeLabel()}`" tooltip="Change theme" class="app-topbar__theme" @click="cycleTheme">
            <UiIcon :name="themeIcon" />
          </UiIconButton>
        </div>
      </header>

      <slot />
    </div>
  </div>

  <nav class="app-bottom-tabs" aria-label="Main" :style="bottomTabsStyle">
    <RouterLink
      v-for="item in primaryNavItems"
      :key="item.to"
      :to="item.to"
      class="app-bottom-tabs__link"
      :aria-current="isActive(item.to) ? 'page' : undefined"
    >
      <UiIcon :name="item.icon" class="app-bottom-tabs__icon" :size="20" />
      <span>{{ item.label }}</span>
    </RouterLink>
  </nav>

  <TransitionRoot as="template" :show="commandOpen" @after-leave="restoreCommandFocus">
    <Dialog
      as="div"
      class="ui-layer"
      :initial-focus="commandInputRef"
      @close="closeCommandPalette"
    >
      <TransitionChild
        as="template"
        enter="ui-transition-fade-enter"
        enter-from="ui-transition-fade-enter-from"
        enter-to="ui-transition-fade-enter-to"
        leave="ui-transition-fade-leave"
        leave-from="ui-transition-fade-leave-from"
        leave-to="ui-transition-fade-leave-to"
      >
        <div class="command-overlay" />
      </TransitionChild>

      <div class="command-wrap">
        <TransitionChild
          as="template"
          enter="ui-transition-scale-enter"
          enter-from="ui-transition-scale-enter-from"
          enter-to="ui-transition-scale-enter-to"
          leave="ui-transition-scale-leave"
          leave-from="ui-transition-scale-leave-from"
          leave-to="ui-transition-scale-leave-to"
        >
          <DialogPanel class="command-panel">
            <DialogTitle class="sr-only">Search routes and library</DialogTitle>
            <Combobox :model-value="null" @update:model-value="selectCommand">
              <label class="sr-only" for="command-palette-input">Search routes and library</label>
              <ComboboxInput
                id="command-palette-input"
                ref="commandInputRef"
                :display-value="() => commandQuery"
                class="ui-input command-input"
                autocomplete="off"
                placeholder="Search podcasts, episodes, chapters, transcripts, summaries"
                @input="commandQuery = ($event.target as HTMLInputElement).value"
              />
              <ComboboxOptions as="ul" class="command-list visually-scrollable">
                <li
                  v-if="globalSearchLoading && commandQuery.trim().length > 0"
                  class="command-empty"
                >
                  Searching library...
                </li>
                <li v-if="globalSearchError" class="command-empty">
                  {{ globalSearchError }}
                </li>
                <li
                  v-if="commandItems.length === 0 && !globalSearchLoading && !globalSearchError"
                  class="command-empty"
                >
                  No matches.
                </li>
                <ComboboxOption
                  v-for="item in commandItems"
                  :key="item.key"
                  as="li"
                  :value="item"
                  class="command-item"
                  v-slot="{ active }"
                >
                  <div
                    class="command-item__option"
                    :class="active ? 'command-item__option--active' : undefined"
                  >
                    <span class="command-item__line">
                      <span>{{ item.label }}</span>
                      <span v-if="item.kind === 'local'" :class="pillClass(item)">{{ item.section }}</span>
                      <span v-else class="meta-text">{{ item.section }}</span>
                    </span>
                    <span class="command-item__meta">{{ item.meta }}</span>
                  </div>
                </ComboboxOption>
              </ComboboxOptions>
            </Combobox>
          </DialogPanel>
        </TransitionChild>
      </div>
    </Dialog>
  </TransitionRoot>
</template>
