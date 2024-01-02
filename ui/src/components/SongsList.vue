<template>
	<q-list>
		<q-item v-for="s in songs" :key="s.id" :to="'/song/' + s.id">
			<q-item-section avatar><img :src="s.img" /></q-item-section>
			<q-item-section>{{s.artist.name}} - {{s.name}}</q-item-section>
		</q-item>
	</q-list>
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
			songs: [],
			total: 0,
		};
	},
	watch: {
		filter: {
			async handler(f: string) {
				const resp = await this.$axios.get('https://www.karafun.co.uk/347021?type=song_list&filter='+ f + '&offset=0');
				this.songs = resp.data.songs;
				this.total = resp.data.total;
			},
			immediate: true,
		},
	},
});
</script>
