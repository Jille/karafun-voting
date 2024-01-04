import { RouteRecordRaw } from 'vue-router';

const routes: RouteRecordRaw[] = [
  {
    path: '/:channel([0-9]{6})/',
    component: () => import('layouts/MainLayout.vue'),
    children: [
			{ name: 'home', path: '', component: () => import('pages/IndexPage.vue'), props: { list: 'theme'} },
			{ name: 'list', path: ':list(theme|styles|top|news)', component: () => import('pages/IndexPage.vue'), props: true },
			{ name: 'songs', path: 'songs/:filter', component: () => import('pages/SongsPage.vue'), props: true },
			{ name: 'song', path: 'song/:id([0-9]+)', component: () => import('pages/SongPage.vue'), props: true },
			{ name: 'search', path: 'search/:search', component: () => import('pages/SearchPage.vue'), props: true },
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
