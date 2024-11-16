<template>
	<div class="row q-gutter-md justify-center">
		<div class="col-2 thumbnail" v-for="c in categories" :key="c.img">
			<router-link :to="{name:'songs', params: {filter: (list=='styles' ? 'st' : 'pl') + '_' + c.id}}">
				<q-img :src="c.img" :title="c.name" fit="scale-down" />
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
	watch: {
		list: {
			async handler(list: string) {
				const resp = await this.$axios.get('https://www.karafun.co.uk/' + this.$route.params.channel + '/?type='+ list);
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
