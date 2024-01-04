<template>
  <q-page>
		<q-tabs class="q-mb-md" active-color="primary">
			<q-route-tab :to="{name: 'list', params: {list: 'theme'}}" exact label="Playlists" />
			<q-route-tab :to="{name: 'list', params: {list: 'styles'}}" exact label="Genres" />
			<q-route-tab :to="{name: 'list', params: {list: 'top'}}" exact label="Top" />
			<q-route-tab :to="{name: 'list', params: {list: 'news'}}" exact label="New releases" />
		</q-tabs>
    <CategoryList :list="list" />
  </q-page>
</template>

<script lang="ts">
import { defineComponent } from 'vue';
import CategoryList from '../components/CategoryList.vue';

export default defineComponent({
  name: 'IndexPage',
	components: {CategoryList},
	props: {
		list: {
			type: String,
			required: true,
		},
	},
});
</script>
