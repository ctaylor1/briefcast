<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
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
const commandActiveIndex = ref(0);
const commandInputRef = ref<HTMLInputElement | null>(null);

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

const filteredCommandItems = computed(() => {
  const query = commandQuery.value.trim().toLowerCase();
  if (!query) {
    return navItems;
  }
  return navItems.filter((item) => {
    const haystack = `${item.label} ${item.section} ${item.keywords}`.toLowerCase();
    return haystack.includes(query);
  });
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

function openCommandPalette(): void {
  commandOpen.value = true;
  commandActiveIndex.value = 0;
}

function closeCommandPalette(): void {
  commandOpen.value = false;
}

function moveCommandSelection(direction: 1 | -1): void {
  if (filteredCommandItems.value.length === 0) {
    commandActiveIndex.value = 0;
    return;
  }
  const maxIndex = filteredCommandItems.value.length - 1;
  const next = commandActiveIndex.value + direction;
  if (next > maxIndex) {
    commandActiveIndex.value = 0;
    return;
  }
  if (next < 0) {
    commandActiveIndex.value = maxIndex;
    return;
  }
  commandActiveIndex.value = next;
}

function selectCommand(item: NavItem): void {
  closeCommandPalette();
  commandQuery.value = "";
  void router.push(item.to);
}

function runActiveCommand(): void {
  const item = filteredCommandItems.value[commandActiveIndex.value];
  if (item) {
    selectCommand(item);
  }
}

function handleCommandInputKeydown(event: KeyboardEvent): void {
  if (event.key === "ArrowDown") {
    event.preventDefault();
    moveCommandSelection(1);
    return;
  }
  if (event.key === "ArrowUp") {
    event.preventDefault();
    moveCommandSelection(-1);
    return;
  }
  if (event.key === "Enter") {
    event.preventDefault();
    runActiveCommand();
    return;
  }
  if (event.key === "Escape") {
    event.preventDefault();
    closeCommandPalette();
  }
}

function handleWindowKeydown(event: KeyboardEvent): void {
  const wantsPalette = (event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k";
  if (wantsPalette) {
    event.preventDefault();
    if (commandOpen.value) {
      closeCommandPalette();
      return;
    }
    openCommandPalette();
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

watch(filteredCommandItems, (items) => {
  if (items.length === 0) {
    commandActiveIndex.value = 0;
    return;
  }
  if (commandActiveIndex.value >= items.length) {
    commandActiveIndex.value = 0;
  }
});

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
            type="button"
            class="command-kbd app-command-trigger"
            :aria-label="`Open command palette (${commandHint})`"
            @click="openCommandPalette"
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
              <button type="button" class="app-user-menu__item" @click="openCommandPalette">
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

  <div
    v-if="commandOpen"
    class="command-overlay"
    role="dialog"
    aria-modal="true"
    aria-label="Command palette"
    @click.self="closeCommandPalette"
  >
    <div class="command-panel">
      <label class="sr-only" for="command-palette-input">Search routes</label>
      <input
        id="command-palette-input"
        ref="commandInputRef"
        v-model="commandQuery"
        type="text"
        class="ui-input command-input"
        placeholder="Search routes and actions"
        @keydown="handleCommandInputKeydown"
      />
      <ul class="command-list visually-scrollable">
        <li v-if="filteredCommandItems.length === 0" class="command-empty">No matches.</li>
        <li
          v-for="(item, index) in filteredCommandItems"
          :key="item.to"
          class="command-item"
        >
          <button
            type="button"
            :aria-current="commandActiveIndex === index ? 'true' : undefined"
            @mouseenter="commandActiveIndex = index"
            @click="selectCommand(item)"
          >
            <span class="command-item__line">
              <span>{{ item.label }}</span>
              <span class="meta-text">{{ item.section }}</span>
            </span>
            <span class="command-item__meta">{{ item.meta }}</span>
          </button>
        </li>
      </ul>
    </div>
  </div>
</template>
