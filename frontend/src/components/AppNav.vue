<script setup lang="ts">
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
import type { LocalSearchResult } from "../types/api";
import UiDropdown from "./ui/UiDropdown.vue";
import briefcastLogo from "../assets/briefcast-logo.svg";

type NavIcon = "podcasts" | "episodes" | "downloads" | "add" | "player" | "settings";

interface NavItem {
  to: string;
  label: string;
  icon: NavIcon;
  section: string;
  meta: string;
  keywords: string;
}

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

const navItems: NavItem[] = [
  {
    to: "/",
    label: "Podcasts",
    icon: "podcasts",
    section: "Library",
    meta: "Subscribed feeds",
    keywords: "dashboard home feeds subscriptions library",
  },
  {
    to: "/episodes",
    label: "Episodes",
    icon: "episodes",
    section: "Library",
    meta: "Chapters, transcripts, filters",
    keywords: "episodes search chapters transcript filters",
  },
  {
    to: "/downloads",
    label: "Downloads",
    icon: "downloads",
    section: "Library",
    meta: "Queue status and progress",
    keywords: "downloads queue status progress pause cancel",
  },
  {
    to: "/add",
    label: "Add Podcast",
    icon: "add",
    section: "Library",
    meta: "Add by URL, OPML, discovery",
    keywords: "add import discover rss feed url opml",
  },
  {
    to: "/player",
    label: "Player",
    icon: "player",
    section: "Playback",
    meta: "Now playing queue",
    keywords: "audio player playback listen now playing",
  },
  {
    to: "/settings",
    label: "Settings",
    icon: "settings",
    section: "System",
    meta: "Retention and defaults",
    keywords: "settings preferences retention defaults",
  },
];

const navSections = computed(() => {
  const grouped = new Map<string, NavItem[]>();
  for (const item of navItems) {
    if (!grouped.has(item.section)) {
      grouped.set(item.section, []);
    }
    grouped.get(item.section)?.push(item);
  }
  return Array.from(grouped.entries()).map(([title, items]) => ({ title, items }));
});

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

const topTitle = computed(() => {
  if (typeof route.meta.title === "string" && route.meta.title.trim()) {
    return route.meta.title.trim();
  }
  return "Briefcast";
});

const topDescription = computed(() => {
  const active = navItems.find((item) => isActive(item.to));
  return active?.meta ?? "Podcast workflows without clutter.";
});

const breadcrumbs = computed(() => {
  const active = navItems.find((item) => isActive(item.to));
  if (!active || active.to === "/") {
    return ["Home"];
  }
  return ["Home", active.section, active.label];
});

const routeCommandItems = computed<CommandRouteItem[]>(() => {
  const query = commandQuery.value.trim().toLowerCase();
  return navItems
    .filter((item) => {
      const haystack = `${item.label} ${item.section} ${item.keywords}`.toLowerCase();
      return !query || haystack.includes(query);
    })
    .map((item) => ({
      kind: "route",
      key: `route:${item.to}`,
      label: item.label,
      section: item.section,
      meta: item.meta,
      to: item.to,
    }));
});

const localCommandItems = computed<CommandLocalItem[]>(() =>
  globalSearchResults.value.map((result, index) => ({
    kind: "local",
    key: `local:${result.type}:${result.episodeId || result.podcastId || index}:${index}`,
    label: localResultLabel(result),
    section: `${localResultTypeLabel(result)} result`,
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

const commandHint = computed(() => (isMac() ? "Cmd+K" : "Ctrl+K"));
const bottomTabsStyle = computed(() => ({
  "--app-bottom-tabs-count": String(navItems.length),
}));

function isMac(): boolean {
  if (typeof navigator === "undefined") {
    return false;
  }
  return /Mac|iPhone|iPad/.test(navigator.platform);
}

function isActive(target: string): boolean {
  if (target === "/") {
    return route.path === "/";
  }
  return route.path === target || route.path.startsWith(`${target}/`);
}

function iconStrokes(name: NavIcon | "search" | "user" | "panel"): string[] {
  switch (name) {
    case "podcasts":
      return ["M4 6h16", "M4 12h16", "M4 18h10"];
    case "episodes":
      return ["M5 5h14v14H5z", "M10 9l5 3-5 3z"];
    case "downloads":
      return ["M12 4v10", "M8 10l4 4 4-4", "M5 20h14"];
    case "add":
      return ["M12 5v14", "M5 12h14", "M12 21a9 9 0 1 1 0-18a9 9 0 0 1 0 18z"];
    case "player":
      return ["M9 7v10l8-5z", "M5 5h14v14H5z"];
    case "settings":
      return ["M4 7h16", "M4 17h16", "M9 5v4", "M15 15v4"];
    case "search":
      return ["M20 20l-4.2-4.2", "M11 18a7 7 0 1 1 0-14a7 7 0 0 1 0 14z"];
    case "user":
      return ["M18 20a6 6 0 0 0-12 0", "M12 12a4 4 0 1 0 0-8a4 4 0 0 0 0 8"];
    case "panel":
      return ["M4 6h16", "M4 12h10", "M4 18h16"];
    default:
      return [];
  }
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
    return;
  }
  commandTriggerRef.value?.focus();
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
  if (wasOpen) {
    void nextTick(() => {
      restoreCommandFocus();
    });
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
    default:
      return "Search result";
  }
}

function localResultRoute(result: LocalSearchResult): { path: string; query?: Record<string, string> } {
  if (result.type === "podcast" && result.podcastId) {
    return {
      path: "/episodes",
      query: { podcastIds: result.podcastId },
    };
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
    return result.podcastTitle || "";
  })();

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

function goToSettings(): void {
  void router.push("/settings");
}

function goToPlayer(): void {
  void router.push("/player");
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

      <nav class="app-nav" aria-label="Sidebar navigation">
        <section v-for="section in navSections" :key="section.title">
          <p class="app-nav__section-title">{{ section.title }}</p>
          <RouterLink
            v-for="item in section.items"
            :key="item.to"
            :to="item.to"
            class="app-nav__link"
            :aria-current="isActive(item.to) ? 'page' : undefined"
          >
            <svg class="app-nav__icon" viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path
                v-for="stroke in iconStrokes(item.icon)"
                :key="stroke"
                :d="stroke"
                stroke="currentColor"
                stroke-width="1.5"
                stroke-linecap="round"
                stroke-linejoin="round"
              />
            </svg>
            <span class="app-nav__text">
              <span class="app-nav__label">{{ item.label }}</span>
              <span class="app-nav__meta meta-text">{{ item.meta }}</span>
            </span>
          </RouterLink>
        </section>
      </nav>

      <button
        v-if="!isDesktop"
        type="button"
        class="app-sidebar__toggle"
        @click="toggleSidebar"
      >
        <svg class="app-nav__icon" viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <path
            v-for="stroke in iconStrokes('panel')"
            :key="stroke"
            :d="stroke"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        </svg>
        <span class="app-nav__label">{{ sidebarExpandedState ? "Collapse" : "Expand" }}</span>
      </button>
    </aside>

    <div class="app-main-wrap">
      <header class="app-topbar">
        <div class="app-topbar__left">
          <h1 class="app-topbar__title">{{ topTitle }}</h1>
          <p class="breadcrumbs">
            <span v-for="crumb in breadcrumbs" :key="crumb">{{ crumb }}</span>
          </p>
        </div>
        <div class="app-topbar__actions">
          <button
            ref="commandTriggerRef"
            type="button"
            class="command-kbd app-command-trigger"
            :aria-label="`Open command palette (${commandHint})`"
            @click="openCommandPalette($event.currentTarget)"
          >
            <svg class="app-nav__icon" viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path
                v-for="stroke in iconStrokes('search')"
                :key="stroke"
                :d="stroke"
                stroke="currentColor"
                stroke-width="1.5"
                stroke-linecap="round"
                stroke-linejoin="round"
              />
            </svg>
            <span>Search</span>
            <span class="meta-text">{{ commandHint }}</span>
          </button>

          <UiDropdown align="end">
            <template #trigger>
              <button type="button" class="ui-button ui-button--ghost ui-button--sm app-user-trigger">
                <svg class="app-nav__icon" viewBox="0 0 24 24" fill="none" aria-hidden="true">
                  <path
                    v-for="stroke in iconStrokes('user')"
                    :key="stroke"
                    :d="stroke"
                    stroke="currentColor"
                    stroke-width="1.5"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                  />
                </svg>
                <span class="app-user-label">Menu</span>
              </button>
            </template>
            <nav class="app-user-menu" aria-label="User menu">
              <p class="app-user-menu__heading">Quick actions</p>
              <button type="button" class="app-user-menu__item" @click="goToSettings">Open settings</button>
              <button type="button" class="app-user-menu__item" @click="goToPlayer">Open player</button>
              <button
                type="button"
                class="app-user-menu__item"
                @click="openCommandPalette($event.currentTarget)"
              >
                Open command palette
              </button>
              <p class="app-user-menu__meta">{{ topDescription }}</p>
            </nav>
          </UiDropdown>
        </div>
      </header>

      <slot />
    </div>
  </div>

  <nav class="app-bottom-tabs" aria-label="Mobile navigation" :style="bottomTabsStyle">
    <RouterLink
      v-for="item in navItems"
      :key="item.to"
      :to="item.to"
      class="app-bottom-tabs__link"
      :aria-current="isActive(item.to) ? 'page' : undefined"
    >
      <svg class="app-bottom-tabs__icon" viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <path
          v-for="stroke in iconStrokes(item.icon)"
          :key="stroke"
          :d="stroke"
          stroke="currentColor"
          stroke-width="1.5"
          stroke-linecap="round"
          stroke-linejoin="round"
        />
      </svg>
      <span>{{ item.label }}</span>
    </RouterLink>
  </nav>

  <TransitionRoot as="template" :show="commandOpen">
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
                placeholder="Search routes, podcasts, episodes, chapters, transcripts"
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
                      <span class="meta-text">{{ item.section }}</span>
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
