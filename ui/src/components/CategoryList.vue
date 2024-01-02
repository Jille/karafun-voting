<template>
	<div class="row q-gutter-md q-pa-md">
		<div class="col-1" v-for="c in categories" :key="c.img">
			<router-link :to="songsLink(c.id)">
				<img :src="c.img" :title="c.name" width="100" height="100" />
			</router-link>
		</div>
	</div>
</template>

<script lang="ts">
import { defineComponent } from 'vue';

export default defineComponent({
  name: 'CategoryList',
	props: {
		list: {
			type: String,
			required: true,
		},
	},
	data() {
		return {
			categories: [],
		};
	},
	methods: {
		songsLink(id: number): string {
			if(this.list == 'styles') {
				return '/songs/st_' + id;
			}
			return '/songs/pl_' + id;
		},
	},
	watch: {
		list: {
			async handler(list: string) {
				const resp = await this.$axios.get('https://www.karafun.co.uk/347021?type='+ list);
				console.log(resp);
				console.log(resp.request.responseURL);
				if(resp.request.responseURL.match(/remote-error/)) {
					return;
				}
				this.categories = resp.data;
			},
			immediate: true,
		},
	},
});
</script>
