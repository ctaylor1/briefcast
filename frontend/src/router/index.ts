import { createRouter, createWebHashHistory } from "vue-router";
import AddPodcastView from "../views/AddPodcastView.vue";
import DashboardView from "../views/DashboardView.vue";
import EpisodesView from "../views/EpisodesView.vue";
import PlayerView from "../views/PlayerView.vue";

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
  ],
});

router.afterEach((to) => {
  const title = typeof to.meta.title === "string" ? to.meta.title : "Briefcast";
  document.title = `${title} | Briefcast`;
});

export default router;
