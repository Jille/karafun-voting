<template>
	<q-list class="full-width">
		<q-item v-for="s in songs" :key="s.id" :to="{name: 'song', params: {id: s.id}}">
			<q-item-section avatar><img :src="s.img" height="70px" /></q-item-section>
			<q-item-section>
				<div class="text-bold">{{s.name}}</div>
				<div>{{s.artist.name}}</div>
			</q-item-section>
		</q-item>
	</q-list>
	<q-btn label="Load more" @click="loadMore" :loading="loading" v-if="songs.length < total" class="q-mb-md" />
</template>

<script lang="ts">
import { defineComponent } from 'vue';

export default defineComponent({
  name: 'CategoryList',
	props: {
		filter: {
			type: String,
			required: true,
		},
	},
	data() {
		return {
			songs: [] as {id: number; name: string; img: string}[],
			total: 0,
			loading: false,
		};
	},
	methods: {
		async loadMore() {
			this.loading = true;
			const resp = await this.$axios.get('https://www.karafun.co.uk/'+ this.$route.params.channel + '?type=song_list&filter='+ this.filter + '&offset='+this.songs.length);
			this.songs.push(...resp.data.songs);
			this.total = resp.data.total;
			this.loading = false;
		},
	},
	watch: {
		filter: {
			async handler(f: string) {
				const resp = await this.$axios.get('https://www.karafun.co.uk/'+ this.$route.params.channel + '?type=song_list&filter='+ f + '&offset=0');
				this.songs = resp.data.songs;
				this.total = resp.data.total;
			},
			immediate: true,
		},
	},
});
</script>
