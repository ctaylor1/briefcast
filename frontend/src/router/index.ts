import { createRouter, createWebHashHistory } from "vue-router";
import AddPodcastView from "../views/AddPodcastView.vue";
import ActivityView from "../views/ActivityView.vue";
import DashboardView from "../views/DashboardView.vue";
import EpisodeDetailView from "../views/EpisodeDetailView.vue";
import EpisodesView from "../views/EpisodesView.vue";
import LinksView from "../views/LinksView.vue";
import PlayerView from "../views/PlayerView.vue";
import PodcastDetailView from "../views/PodcastDetailView.vue";
import SettingsView from "../views/SettingsView.vue";
import SummariesView from "../views/SummariesView.vue";

const router = createRouter({
  history: createWebHashHistory("/app/"),
  routes: [
    {
      path: "/",
      name: "dashboard",
      component: DashboardView,
      meta: { title: "Library" },
    },
    {
      path: "/podcasts/:id",
      name: "podcast",
      component: PodcastDetailView,
      meta: { title: "Podcast" },
    },
    {
      path: "/episodes",
      name: "episodes",
      component: EpisodesView,
      meta: { title: "Episodes" },
    },
    {
      path: "/episodes/:id",
      name: "episode",
      component: EpisodeDetailView,
      meta: { title: "Episode" },
    },
    {
      path: "/summaries",
      name: "summaries",
      component: SummariesView,
      meta: { title: "Summaries" },
    },
    {
      path: "/links",
      name: "links",
      component: LinksView,
      meta: { title: "Most referenced" },
    },
    {
      path: "/activity",
      name: "activity",
      component: ActivityView,
      meta: { title: "Activity" },
    },
    // Old addresses: Downloads is a tab of Activity, About is part of Settings.
    { path: "/downloads", redirect: "/activity" },
    {
      path: "/add",
      name: "add",
      component: AddPodcastView,
      meta: { title: "Add podcast" },
    },
    {
      path: "/player",
      name: "player",
      component: PlayerView,
      meta: { title: "Now playing" },
    },
    {
      path: "/settings",
      name: "settings",
      component: SettingsView,
      meta: { title: "Settings" },
    },
    { path: "/about", redirect: { path: "/settings", query: { section: "about" } } },
    { path: "/:pathMatch(.*)*", redirect: "/" },
  ],
});

router.afterEach((to) => {
  const title = typeof to.meta.title === "string" ? to.meta.title : "Briefcast";
  document.title = `${title} | Briefcast`;
});

export default router;
