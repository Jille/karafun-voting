<template>
  <div class="q-col-gutter-md row items-start">
		<div v-for="s in songs" :key="s.id" class="col-6">
		  <router-link :to="{name: 'song', params: {id: s.id}}">
        <q-img :src="s.img">
          <div class="absolute-bottom text-subtitle1 text-center">
            <div>{{s.artist.name}}</div>
            <div>{{s.name}}</div>
          </div>
        </q-img>
      </router-link>
		</div>
	</div>
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
				const resp = await this.$axios.get('https://www.karafun.co.uk/'+ this.$route.params.channel + '?type=song_list&filter='+ f + '&offset=0');
				this.songs = resp.data.songs;
				this.total = resp.data.total;
			},
			immediate: true,
		},
	},
});
</script>
