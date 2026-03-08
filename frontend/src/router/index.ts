import { createRouter, createWebHashHistory } from "vue-router";
import AddPodcastView from "../views/AddPodcastView.vue";
import AboutView from "../views/AboutView.vue";
import DashboardView from "../views/DashboardView.vue";
import DownloadsView from "../views/DownloadsView.vue";
import EpisodesView from "../views/EpisodesView.vue";
import PlayerView from "../views/PlayerView.vue";
import SettingsView from "../views/SettingsView.vue";
import SummariesView from "../views/SummariesView.vue";

const router = createRouter({
  history: createWebHashHistory("/app/"),
  routes: [
    {
      path: "/",
      name: "dashboard",
      component: DashboardView,
      meta: { title: "Podcasts" },
    },
    {
      path: "/episodes",
      name: "episodes",
      component: EpisodesView,
      meta: { title: "Episodes" },
    },
    {
      path: "/summaries",
      name: "summaries",
      component: SummariesView,
      meta: { title: "Summaries" },
    },
    {
      path: "/downloads",
      name: "downloads",
      component: DownloadsView,
      meta: { title: "Downloads" },
    },
    {
      path: "/add",
      name: "add",
      component: AddPodcastView,
      meta: { title: "Add Podcast" },
    },
    {
      path: "/player",
      name: "player",
      component: PlayerView,
      meta: { title: "Player" },
    },
    {
      path: "/settings",
      name: "settings",
      component: SettingsView,
      meta: { title: "Settings" },
    },
    {
      path: "/about",
      name: "about",
      component: AboutView,
      meta: { title: "About" },
    },
  ],
});

router.afterEach((to) => {
  const title = typeof to.meta.title === "string" ? to.meta.title : "Briefcast";
  document.title = `${title} | Briefcast`;
});

export default router;
