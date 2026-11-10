// A memory-history router for component tests.
import { createMemoryHistory, createRouter, type RouteRecordRaw, type Router } from "vue-router";

const Blank = { template: "<div />" };

export async function makeRouter(path = "/", routes: RouteRecordRaw[] = []): Promise<Router> {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [...routes, { path: "/:pathMatch(.*)*", component: Blank }],
  });
  await router.push(path);
  await router.isReady();
  return router;
}
