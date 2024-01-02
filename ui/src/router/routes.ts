import { RouteRecordRaw } from 'vue-router';

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    component: () => import('layouts/MainLayout.vue'),
    children: [
			{ path: '', component: () => import('pages/IndexPage.vue'), props: { list: 'theme'} },
			{ path: ':list(theme|styles|top|news)', component: () => import('pages/IndexPage.vue'), props: true },
			{ path: 'songs/:filter', component: () => import('pages/SongsPage.vue'), props: true },
			{ path: 'song/:id([0-9]+)', component: () => import('pages/SongPage.vue'), props: true },
			{ name: 'search', path: 'search/:search', component: () => import('pages/SearchPage.vue'), props: true },
			{ path: 'queue', component: () => import('pages/QueuePage.vue') },
		],
  },

  // Always leave this as last one,
  // but you can also remove it
  {
    path: '/:catchAll(.*)*',
    component: () => import('pages/ErrorNotFound.vue'),
  },
];

export default routes;
